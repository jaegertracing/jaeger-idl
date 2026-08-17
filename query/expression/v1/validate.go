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
// defined one, every reference names something, a reference to a built-in field names one this
// API defines (see Field), and the quantifier's binding rules hold (RFC 0005 §5.5).
//
// What it deliberately leaves alone is a constant's spelling — whether "banana" is a duration
// is answered by ResolveConstants, which knows the field it is compared against — and which of
// the valid things a given backend can serve, which is what a backend's declared capabilities
// are for.
func ValidateFilter(filter *Call) error {
	if filter == nil {
		return errors.New("filter is empty")
	}
	return validateCall(filter, nil)
}

// validateCall checks one call. quantified carries the collection levels of the enclosing
// OpSome calls, which is what lets a nested quantifier over an already-bound level be refused.
func validateCall(call *Call, quantified []Level) error {
	if call == nil {
		return errors.New("filter has a missing predicate")
	}
	switch call.Op {
	case OpAnd, OpOr:
		if len(call.Args) < 2 {
			return fmt.Errorf("operator %q takes at least two arguments, got %d", call.Op, len(call.Args))
		}
		return validatePredicateArgs(call, quantified)
	case OpNot:
		if err := wantArgs(call, 1); err != nil {
			return err
		}
		return validatePredicateArgs(call, quantified)
	case OpExists:
		if err := wantArgs(call, 1); err != nil {
			return err
		}
		return validateReference(call.Op, call.Args[0])
	case OpSome:
		if err := wantArgs(call, 2); err != nil {
			return err
		}
		return validateSome(call, quantified)
	case OpIn, OpNotIn:
		if err := wantArgs(call, 2); err != nil {
			return err
		}
		if err := validateSubject(call.Op, call.Args[0], quantified); err != nil {
			return err
		}
		list, ok := call.Args[1].(*List)
		if !ok || list == nil {
			return fmt.Errorf("operator %q takes a list as its second argument, got %s", call.Op, termName(call.Args[1]))
		}
		if len(list.Values) == 0 {
			// Membership in nothing matches nothing, so the query asks for an empty result in a
			// way that reads like an oversight. Refusing says so.
			return fmt.Errorf("operator %q takes a list with at least one element", call.Op)
		}
		return validateValueType(list.Type)
	case OpRegex:
		if err := wantArgs(call, 2); err != nil {
			return err
		}
		if err := validateSubject(call.Op, call.Args[0], quantified); err != nil {
			return err
		}
		if !isTextConstant(call.Args[1]) {
			return fmt.Errorf("operator %q takes a constant string as its pattern, got %s", call.Op, termName(call.Args[1]))
		}
		return nil
	case OpEq, OpNe:
		if err := wantArgs(call, 2); err != nil {
			return err
		}
		return validateComparison(call, quantified)
	case OpGt, OpLt, OpGte, OpLte:
		if err := wantArgs(call, 2); err != nil {
			return err
		}
		return validateOrderedComparison(call, quantified)
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
func validatePredicateArgs(call *Call, quantified []Level) error {
	for _, arg := range call.Args {
		nested, ok := arg.(*Call)
		if !ok {
			return fmt.Errorf("operator %q takes predicates as arguments, got %s", call.Op, termName(arg))
		}
		if err := validateCall(nested, quantified); err != nil {
			return err
		}
	}
	return nil
}

// validateSome checks the existential quantifier: it binds one element of a span's
// events or links, so its first argument names that collection and its second is the
// predicate evaluated against the bound element.
func validateSome(call *Call, quantified []Level) error {
	ref, ok := call.Args[0].(*NestedRef)
	if !ok || ref == nil {
		return fmt.Errorf("operator %q takes a collection reference as its first argument, got %s", call.Op, termName(call.Args[0]))
	}
	if ref.Level != LevelEvent && ref.Level != LevelLink {
		return fmt.Errorf("operator %q quantifies over %q or %q, got level %q", call.Op, LevelEvent, LevelLink, ref.Level)
	}
	// RFC 0005 §5.5 rule 4: whether an inner quantifier shadows the outer one, and whether its
	// predicate may reach back to the outer element, are questions this version does not answer,
	// so it refuses the query rather than answering one of them by accident.
	if slices.Contains(quantified, ref.Level) {
		return fmt.Errorf("operator %q is already quantifying over %q, and this version does not define what a nested one would bind", call.Op, ref.Level)
	}
	predicate, ok := call.Args[1].(*Call)
	if !ok {
		return fmt.Errorf("operator %q takes a predicate as its second argument, got %s", call.Op, termName(call.Args[1]))
	}
	return validateCall(predicate, append(slices.Clone(quantified), ref.Level))
}

// validateComparison checks the two operands of an equality: each names or supplies a value,
// and they are not both constants, since comparing one constant against another asks nothing
// about the span.
func validateComparison(call *Call, quantified []Level) error {
	for _, arg := range call.Args {
		if err := validateOperand(call.Op, arg, quantified); err != nil {
			return err
		}
	}
	if isConstant(call.Args[0]) && isConstant(call.Args[1]) {
		return fmt.Errorf("operator %q compares a reference against a constant, or two references, got two constants", call.Op)
	}
	return nil
}

// validateOrderedComparison checks an ordered comparison, which is an equality's operands plus
// the one restriction RFC 0005 §5.3 spells out: the values are read as numbers or instants,
// never as text, so a constant that only has a text ordering is a type error rather than a
// comparison a backend may answer its own way.
func validateOrderedComparison(call *Call, quantified []Level) error {
	if err := validateComparison(call, quantified); err != nil {
		return err
	}
	for _, arg := range call.Args {
		if isConstant(arg) && !isOrderedConstant(arg) {
			return fmt.Errorf("operator %q reads its operands as numbers or instants, got %s", call.Op, termName(arg))
		}
	}
	return nil
}

// validateOperand checks a value an operator compares. A nested call is allowed
// because a call's result is itself a value — the property that lets a future
// arithmetic or extraction function sit under a comparison.
func validateOperand(op Operator, arg Expression, quantified []Level) error {
	switch term := arg.(type) {
	case *AttributeRef:
		return validateAttributeRef(term)
	case *FieldRef:
		return validateFieldRef(term)
	case *NestedRef:
		return errCollectionOutOfPlace()
	case *Call:
		return validateCall(term, quantified)
	}
	if isConstant(arg) {
		return nil
	}
	return fmt.Errorf("operator %q cannot compare %s; only a reference, a constant, or a call result can be compared", op, termName(arg))
}

// validateSubject checks the operand an operator reads a value from rather than supplies one
// to: the left-hand side of membership and of a regular expression.
func validateSubject(op Operator, arg Expression, quantified []Level) error {
	if call, ok := arg.(*Call); ok {
		return validateCall(call, quantified)
	}
	return validateReference(op, arg)
}

// validateReference checks an argument that has to name a value on the span.
func validateReference(op Operator, arg Expression) error {
	switch term := arg.(type) {
	case *AttributeRef:
		return validateAttributeRef(term)
	case *FieldRef:
		return validateFieldRef(term)
	case *NestedRef:
		return errCollectionOutOfPlace()
	default:
		return fmt.Errorf("operator %q takes a reference, got %s", op, termName(arg))
	}
}

func validateAttributeRef(ref *AttributeRef) error {
	if ref == nil {
		return errors.New("filter has a missing reference")
	}
	if ref.Level != "" && !slices.Contains(levels, ref.Level) {
		return fmt.Errorf("unknown filter level %q", ref.Level)
	}
	if ref.Key == "" {
		return errors.New("attribute reference has no key")
	}
	return nil
}

func validateFieldRef(ref *FieldRef) error {
	if ref == nil {
		return errors.New("filter has a missing reference")
	}
	// An empty level is the unqualified attribute search, and no built-in field has an
	// unqualified form, so there is nothing for a field reference to mean without one.
	if ref.Level == "" {
		return errors.New("field reference has no level, and a built-in field belongs to one")
	}
	if !slices.Contains(levels, ref.Level) {
		return fmt.Errorf("unknown filter level %q", ref.Level)
	}
	if ref.Name == "" {
		return errors.New("field reference has no name")
	}
	if _, ok := LookupField(ref.Level, ref.Name); !ok {
		return fmt.Errorf("unknown built-in field %q at the %q level; name an attribute to match a tag spelled that way instead",
			ref.Name, ref.Level)
	}
	return nil
}

// errCollectionOutOfPlace refuses a collection reference anywhere but the one place it means
// something. A collection is many values rather than one, so nothing else can read it.
func errCollectionOutOfPlace() error {
	return fmt.Errorf("a collection reference is only the first argument of %q", OpSome)
}

func validateValueType(t ValueType) error {
	if t != "" && !slices.Contains(valueTypes, t) {
		return fmt.Errorf("unknown filter value type %q", t)
	}
	return nil
}

// isConstant reports whether a term is a single constant value. A List is not one: it is only
// ever the right-hand side of membership, never a value an operator reads or compares.
func isConstant(e Expression) bool {
	switch e.(type) {
	case *AnyValue, *StringValue, *IntValue, *DoubleValue, *BoolValue, *DurationValue, *TimestampValue:
		return true
	default:
		return false
	}
}

// isOrderedConstant reports whether an ordering is defined on a constant. An untyped constant
// counts, because it is what an unhinted number or duration arrives as and what a numeric
// operator asks a backend to read numerically.
func isOrderedConstant(e Expression) bool {
	switch e.(type) {
	case *AnyValue, *IntValue, *DoubleValue, *DurationValue, *TimestampValue:
		return true
	default:
		return false
	}
}

// isTextConstant reports whether a constant can be read as text, which is what a regular
// expression needs of its pattern. An untyped constant counts: a pattern is written as a bare
// string and carries no wire hint.
func isTextConstant(e Expression) bool {
	switch e.(type) {
	case *AnyValue, *StringValue:
		return true
	default:
		return false
	}
}

// termName names the kind of a term for an error message.
func termName(e Expression) string {
	switch e.(type) {
	case *AttributeRef:
		return "an attribute reference"
	case *FieldRef:
		return "a field reference"
	case *NestedRef:
		return "a collection reference"
	case *AnyValue:
		return "an untyped constant"
	case *StringValue:
		return "a string constant"
	case *IntValue:
		return "an integer constant"
	case *DoubleValue:
		return "a floating-point constant"
	case *BoolValue:
		return "a boolean constant"
	case *DurationValue:
		return "a duration constant"
	case *TimestampValue:
		return "a timestamp constant"
	case *List:
		return "a list"
	case *Call:
		return "a predicate"
	default:
		return "an empty term"
	}
}
