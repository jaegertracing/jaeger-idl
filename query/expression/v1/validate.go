// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
	"errors"
	"fmt"
	"slices"
)

// ValidateFilter checks that a filter is well formed: every operator is one this package
// defines and has the number and kind of arguments it takes, every level and value type is a
// defined one, every reference names something, and a reference to a built-in field names one
// this API defines (see Field).
//
// It does not type check — comparing a duration against a word is a valid graph and a separate
// concern (RFC 0005 §6.1) — and it says nothing about which of the valid things a given
// backend can serve, which is what a backend's declared capabilities are for.
func ValidateFilter(filter *Call) error {
	if filter == nil {
		return errors.New("filter is empty")
	}
	return validateCall(filter)
}

func validateCall(call *Call) error {
	if call == nil {
		return errors.New("filter has a missing predicate")
	}
	switch call.Op {
	case OpAnd, OpOr:
		if len(call.Args) < 2 {
			return fmt.Errorf("operator %q takes at least two arguments, got %d", call.Op, len(call.Args))
		}
		return validatePredicateArgs(call)
	case OpNot:
		if err := wantArgs(call, 1); err != nil {
			return err
		}
		return validatePredicateArgs(call)
	case OpExists:
		if err := wantArgs(call, 1); err != nil {
			return err
		}
		ref, ok := call.Args[0].(*Reference)
		if !ok {
			return fmt.Errorf("operator %q takes a reference, got %s", call.Op, termName(call.Args[0]))
		}
		return validateReference(ref)
	case OpSome:
		if err := wantArgs(call, 2); err != nil {
			return err
		}
		return validateSome(call)
	case OpIn, OpNotIn:
		if err := wantArgs(call, 2); err != nil {
			return err
		}
		if err := validateOperand(call.Args[0]); err != nil {
			return err
		}
		list, ok := call.Args[1].(*List)
		if !ok || list == nil {
			return fmt.Errorf("operator %q takes a list as its second argument, got %s", call.Op, termName(call.Args[1]))
		}
		return validateValueType(list.Type)
	case OpEq, OpNe, OpGt, OpLt, OpGte, OpLte, OpRegex:
		if err := wantArgs(call, 2); err != nil {
			return err
		}
		for _, arg := range call.Args {
			if err := validateOperand(arg); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown filter operator %q", call.Op)
	}
}

// wantArgs checks an operator's arity in the case that knows it, beside the check on what kind
// of arguments it takes — which is the part worth reading.
func wantArgs(call *Call, n int) error {
	if len(call.Args) != n {
		return fmt.Errorf("operator %q takes %d argument(s), got %d", call.Op, n, len(call.Args))
	}
	return nil
}

// validatePredicateArgs checks the arguments of a boolean combinator, each of which
// must itself be a predicate rather than a bare reference or constant.
func validatePredicateArgs(call *Call) error {
	for _, arg := range call.Args {
		nested, ok := arg.(*Call)
		if !ok {
			return fmt.Errorf("operator %q takes predicates as arguments, got %s", call.Op, termName(arg))
		}
		if err := validateCall(nested); err != nil {
			return err
		}
	}
	return nil
}

// validateSome checks the existential quantifier: it binds one element of a span's
// events or links, so its first argument names that collection and its second is the
// predicate evaluated against the bound element.
func validateSome(call *Call) error {
	ref, ok := call.Args[0].(*Reference)
	if !ok || ref == nil {
		return fmt.Errorf("operator %q takes a collection reference as its first argument, got %s", call.Op, termName(call.Args[0]))
	}
	if ref.Level != LevelEvent && ref.Level != LevelLink {
		return fmt.Errorf("operator %q quantifies over %q or %q, got level %q", call.Op, LevelEvent, LevelLink, ref.Level)
	}
	if ref.Name != "" {
		return fmt.Errorf("operator %q takes the whole collection, so its first argument must not name %q", call.Op, ref.Name)
	}
	predicate, ok := call.Args[1].(*Call)
	if !ok {
		return fmt.Errorf("operator %q takes a predicate as its second argument, got %s", call.Op, termName(call.Args[1]))
	}
	return validateCall(predicate)
}

// validateOperand checks a value an operator compares. A nested call is allowed
// because a call's result is itself a value — the property that lets a future
// arithmetic or extraction function sit under a comparison.
func validateOperand(arg Expression) error {
	switch term := arg.(type) {
	case *Reference:
		return validateReference(term)
	case *Scalar:
		if term == nil {
			return errors.New("filter has a missing constant")
		}
		return validateValueType(term.Type)
	case *Call:
		return validateCall(term)
	default:
		return fmt.Errorf("%s cannot be compared; only a reference, a constant, or a call result can", termName(arg))
	}
}

func validateReference(ref *Reference) error {
	if ref == nil {
		return errors.New("filter has a missing reference")
	}
	if ref.Level != "" && !slices.Contains(levels, ref.Level) {
		return fmt.Errorf("unknown filter level %q", ref.Level)
	}
	if ref.Name == "" {
		return errors.New("filter reference has no name")
	}
	if !ref.IsAttribute() {
		if _, ok := LookupField(ref.Level, ref.Name); !ok {
			return fmt.Errorf("unknown built-in field %q at the %q level; set attr to name an attribute instead",
				ref.Name, ref.Level)
		}
	}
	return nil
}

func validateValueType(t ValueType) error {
	if t != "" && !slices.Contains(valueTypes, t) {
		return fmt.Errorf("unknown filter value type %q", t)
	}
	return nil
}

// termName names the kind of a term for an error message.
func termName(e Expression) string {
	switch e.(type) {
	case *Reference:
		return "a reference"
	case *Scalar:
		return "a constant"
	case *List:
		return "a list"
	case *Call:
		return "a predicate"
	default:
		return "an empty term"
	}
}
