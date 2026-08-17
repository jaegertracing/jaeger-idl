// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ResolveConstants reads every unconstrained constant that is compared against a built-in
// field as that field's declared type, and refuses one whose spelling will not parse — a
// duration of "banana" is answered at the query boundary rather than passed to a backend to
// interpret. It is stage 3 of RFC 0005 §7 and expects a filter ValidateFilter has accepted.
//
// A constant compared against an *attribute* is left alone, because only storage knows how that
// attribute was written. Resolution rewrites the nodes it changes and returns a new tree rather
// than annotating the one it was given, so nothing it produces can go stale when a query
// interceptor edits a predicate afterwards.
func ResolveConstants(filter *Call) (*Call, error) {
	if filter == nil {
		return nil, errors.New("filter is empty")
	}
	return resolveCall(filter)
}

// resolveCall rebuilds a call with its arguments resolved. The arguments it does not rewrite are
// carried over as they are: a term is never modified in place, so sharing one is safe.
func resolveCall(call *Call) (*Call, error) {
	if call == nil {
		return nil, nil
	}
	args := make([]Expression, len(call.Args))
	for i, arg := range call.Args {
		nested, ok := arg.(*Call)
		if !ok {
			args[i] = arg
			continue
		}
		resolved, err := resolveCall(nested)
		if err != nil {
			return nil, err
		}
		args[i] = resolved
	}
	if len(args) == 2 {
		var err error
		switch {
		case isComparison(call.Op):
			err = resolveComparison(args)
		case call.Op == OpIn || call.Op == OpNotIn:
			err = checkMembership(args)
		}
		if err != nil {
			return nil, err
		}
	}
	return &Call{Op: call.Op, Args: args}, nil
}

// resolveComparison rewrites the unconstrained constant sitting opposite a built-in field. A
// regular expression is not one of the comparisons this runs for, because its pattern stays a
// pattern whatever the field holds, and nor is membership, whose List carries its own elements.
func resolveComparison(args []Expression) error {
	for i, arg := range args {
		ref, ok := arg.(*FieldRef)
		if !ok || ref == nil {
			continue
		}
		other := 1 - i
		raw, ok := args[other].(*AnyValue)
		if !ok {
			continue
		}
		field, ok := LookupField(ref.Level, ref.Name)
		if !ok {
			// ValidateFilter refuses a field this API does not define, so there is nothing to
			// resolve against and nothing useful to say about it here.
			continue
		}
		value, err := readConstant(field.Type, raw.Value)
		if err != nil {
			return fmt.Errorf("cannot compare %s.%s against %q: %w", ref.Level, ref.Name, raw.Value, err)
		}
		args[other] = value
	}
	return nil
}

// checkMembership reads every element of a list compared against a built-in field as that
// field's type, so a spelling refused under `gt` is refused under `in` as well. The list is not
// rewritten — it carries its elements as spellings and there is no typed list node — so this
// only refuses what cannot be read.
func checkMembership(args []Expression) error {
	ref, ok := args[0].(*FieldRef)
	if !ok || ref == nil {
		return nil
	}
	list, ok := args[1].(*List)
	if !ok || list == nil || list.Type != "" {
		// A list that declares its own element type says how to read itself.
		return nil
	}
	field, ok := LookupField(ref.Level, ref.Name)
	if !ok {
		return nil
	}
	for _, element := range list.Values {
		if _, err := readConstant(field.Type, element); err != nil {
			return fmt.Errorf("cannot compare %s.%s against %q: %w", ref.Level, ref.Name, element, err)
		}
	}
	return nil
}

// readConstant reads a constant's spelling as the type a field holds. The two that measure time
// have no wire type of their own, which is why this is the only place they are produced.
func readConstant(t FieldType, raw string) (Expression, error) {
	switch t {
	case FieldTypeString:
		return &StringValue{Value: raw}, nil
	case FieldTypeDuration:
		value, err := time.ParseDuration(raw)
		if err != nil {
			return nil, err
		}
		return &DurationValue{Value: value}, nil
	case FieldTypeTimestamp:
		value, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return nil, err
		}
		return &TimestampValue{Value: value}, nil
	case FieldTypeSpanKind:
		return readWord(raw, SpanKinds)
	case FieldTypeSpanStatus:
		return readWord(raw, SpanStatuses)
	default:
		return nil, fmt.Errorf("no rule for reading a constant as %q", t)
	}
}

// readWord reads a constant that has to be one of a closed set of words. The set is small
// enough to name in the error, which is the whole value of refusing here rather than letting a
// backend match nothing.
func readWord(raw string, words []string) (Expression, error) {
	if slices.Contains(words, raw) {
		return &StringValue{Value: raw}, nil
	}
	return nil, fmt.Errorf("not one of %s", strings.Join(words, ", "))
}

// isComparison reports whether an operator compares its two operands by value, which is what
// makes a built-in field's type the type of the constant beside it.
func isComparison(op Operator) bool {
	switch op {
	case OpEq, OpNe, OpGt, OpLt, OpGte, OpLte:
		return true
	default:
		return false
	}
}
