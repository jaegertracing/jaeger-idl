// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// ResolveConstants reads every unconstrained constant that is compared against a built-in
// field as that field's declared type, and refuses one whose text will not parse — a
// duration of "banana" is answered at the query boundary rather than passed to a backend to
// interpret. It is stage 3 of RFC 0005 §7 and expects a filter ValidateFilter has accepted.
//
// A constant compared against an *attribute* is left alone, because only storage knows how that
// attribute was written. Resolution rewrites the nodes it changes and returns a new tree rather
// than annotating the one it was given, so nothing it produces can go stale when a query
// interceptor edits a predicate afterwards.
//
// It also puts the reference first in every comparison, so each consumer downstream reads one
// orientation rather than handling both.
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
	op := call.Op
	if len(args) == 2 {
		var err error
		switch {
		case isComparison(op):
			if err = resolveComparison(args); err == nil {
				op, args = referenceFirst(op, args)
			}
		case op == OpIn || op == OpNotIn:
			err = checkMembership(args)
		}
		if err != nil {
			return nil, err
		}
	}
	return &Call{Op: op, Args: args}, nil
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
		field, ok := LookupField(ref.Level, ref.Name)
		if !ok {
			// ValidateFilter refuses a field this API does not define, so there is nothing to
			// resolve against and nothing useful to say about it here.
			continue
		}
		text, ok := textToRead(field.Type, args[other])
		if !ok {
			continue
		}
		value, err := readConstant(field.Type, text)
		if err != nil {
			return fmt.Errorf("cannot compare %s.%s against %q: %w", ref.Level, ref.Name, text, err)
		}
		args[other] = value
	}
	return nil
}

// referenceFirst puts the reference on the left of a comparison. A caller may write the constant
// there instead, and swapping the operands asks the same question as long as an ordered operator
// turns around with them.
func referenceFirst(op Operator, args []Expression) (Operator, []Expression) {
	if !isConstant(args[0]) || isConstant(args[1]) {
		return op, args
	}
	return turnedAround(op), []Expression{args[1], args[0]}
}

func turnedAround(op Operator) Operator {
	switch op {
	case OpGt:
		return OpLt
	case OpLt:
		return OpGt
	case OpGte:
		return OpLte
	case OpLte:
		return OpGte
	default:
		return op
	}
}

// checkMembership reads every element of a list compared against a built-in field as that
// field's type, so a value refused under `gt` is refused under `in` as well. The list is not
// rewritten — it carries its elements as text and there is no typed list node — so this
// only refuses what cannot be read.
//
// A declared element type does not exempt the list. It says how to read the elements, so it has
// to be a type the field could hold, and the elements still have to be readable as it.
func checkMembership(args []Expression) error {
	list, ok := args[1].(*List)
	if !ok || list == nil {
		return nil
	}
	if err := readDeclaredElements(list); err != nil {
		return err
	}
	ref, ok := args[0].(*FieldRef)
	if !ok || ref == nil {
		return nil
	}
	field, ok := LookupField(ref.Level, ref.Name)
	if !ok {
		return nil
	}
	if list.Type != "" && domainOfValueType(list.Type) != domainOfFieldType(field.Type) {
		return fmt.Errorf("cannot compare %s.%s against a list of %s: the field holds %s",
			ref.Level, ref.Name, list.Type, field.Type)
	}
	for _, element := range list.Values {
		if _, err := readConstant(field.Type, element); err != nil {
			return fmt.Errorf("cannot compare %s.%s against %q: %w", ref.Level, ref.Name, element, err)
		}
	}
	return nil
}

// readDeclaredElements reads a list's elements as the type the list declares. That type is what
// the list says its elements are, so it holds wherever the list appears — including beside an
// attribute, where there is no field to check it against.
func readDeclaredElements(list *List) error {
	if list.Type == "" {
		return nil
	}
	for _, element := range list.Values {
		if err := readValue(list.Type, element); err != nil {
			return fmt.Errorf("element %q of a list of %s: %w", element, list.Type, err)
		}
	}
	return nil
}

// readValue reads an element as a declared wire type. A string needs no reading; the others each
// have one form, and anything else is a value the caller cannot have meant.
func readValue(t ValueType, raw string) error {
	var err error
	switch t {
	case ValueTypeInt:
		_, err = strconv.ParseInt(raw, 10, 64)
	case ValueTypeDouble:
		_, err = strconv.ParseFloat(raw, 64)
	case ValueTypeBool:
		_, err = strconv.ParseBool(raw)
	}
	return err
}

// textToRead returns the text of a constant that still has to be read as a field's type: an
// untyped constant always, and a string beside a field holding one of a closed set of words, since
// it has to be one of those words. Every other constant already carries its value, and validation
// has refused the ones the field cannot hold.
func textToRead(t FieldType, operand Expression) (string, bool) {
	if isMissing(operand) {
		// Resolution answers for any tree, including one validation would have refused.
		return "", false
	}
	switch value := operand.(type) {
	case *AnyValue:
		return value.Value, true
	case *StringValue:
		if t == FieldTypeSpanKind || t == FieldTypeSpanStatus {
			return value.Value, true
		}
	}
	return "", false
}

// readConstant reads a constant's text as the type a field holds. The two that measure time
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
	case FieldTypeSpanKind, FieldTypeSpanStatus:
		return readWord(raw, wordsOf(t))
	default:
		return nil, fmt.Errorf("no rule for reading a constant as %q", t)
	}
}

// wordsOf names the closed set a word-valued field holds.
func wordsOf(t FieldType) []string {
	if t == FieldTypeSpanKind {
		return spanKinds
	}
	return spanStatuses
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
