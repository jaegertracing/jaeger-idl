// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
	"errors"
	"fmt"
	"regexp/syntax"
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
		if err := validateRegexSubject(call.Args[0]); err != nil {
			return err
		}
		pattern, ok := patternText(call.Args[1])
		if !ok {
			return fmt.Errorf("operator %q takes a constant string as its pattern, got %s", call.Op, termName(call.Args[1]))
		}
		return validatePattern(pattern)
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

// validateComparison checks the two operands of a comparison: one names a value on the span and
// the other supplies the constant to compare it against, in either order. Two constants ask
// nothing about the span, and what comparing two references means — a duration against a name,
// say — is a question this version does not answer, so neither is accepted.
func validateComparison(call *Call, quantified []Level) error {
	for _, arg := range call.Args {
		if err := validateOperand(call.Op, arg, quantified); err != nil {
			return err
		}
	}
	if isConstant(call.Args[0]) && isConstant(call.Args[1]) {
		return fmt.Errorf("operator %q compares a reference against a constant, got two constants", call.Op)
	}
	if !isConstant(call.Args[0]) && !isConstant(call.Args[1]) {
		return fmt.Errorf("operator %q compares a reference against a constant, and this version does not define what comparing two references means", call.Op)
	}
	reference, constant := call.Args[0], call.Args[1]
	if isConstant(reference) {
		reference, constant = constant, reference
	}
	return validateTimeConstant(call.Op, reference, constant)
}

// validateTimeConstant refuses a duration or an instant compared against an attribute. The wire
// spells neither (§5.4), so one survives only where a built-in field's declared type rebuilds it.
// An attribute declares nothing, so the constant would come back untyped and ask the backend a
// different question; comparing the attribute against the spelling asks that question directly.
func validateTimeConstant(op Operator, reference, constant Expression) error {
	if _, ok := reference.(*AttributeRef); !ok {
		return nil
	}
	switch constant.(type) {
	case *DurationValue:
		return errNoWireSpelling(op, constant, "duration")
	case *TimestampValue:
		return errNoWireSpelling(op, constant, "timestamp")
	}
	return nil
}

func errNoWireSpelling(op Operator, constant Expression, kind string) error {
	return fmt.Errorf("operator %q compares %s against an attribute, and the wire cannot spell a %s",
		op, termName(constant), kind)
}

// validateOrderedComparison checks an ordered comparison. Its operands have to be comparable in
// one domain — numbers with numbers, durations with durations, instants with instants, text with
// text — because there is no defensible answer to a duration against a bare number. Text is
// ordered lexicographically, which is a real query: `span.name > "m"` asks for the names that
// sort after it.
func validateOrderedComparison(call *Call, quantified []Level) error {
	if err := validateComparison(call, quantified); err != nil {
		return err
	}
	for _, arg := range call.Args {
		if !orderable(arg) {
			return fmt.Errorf("operator %q has no ordering for %s", call.Op, termName(arg))
		}
	}
	left, right := domainOfOperand(call.Args[0]), domainOfOperand(call.Args[1])
	if left != domainUnknown && right != domainUnknown && left != right {
		return fmt.Errorf("operator %q compares %s against %s, which have no common ordering",
			call.Op, termName(call.Args[0]), termName(call.Args[1]))
	}
	return nil
}

// validateOperand checks a value an operator compares. A call is not one: no operator in this
// vocabulary has a result type, so there is nothing to say about what comparing one would mean.
// An operator that takes a call result — a future extraction function, say — arrives with its
// signature declared rather than through this door (§5.3).
func validateOperand(op Operator, arg Expression, _ []Level) error {
	switch term := arg.(type) {
	case *AttributeRef:
		return validateAttributeRef(term)
	case *FieldRef:
		return validateFieldRef(term)
	case *NestedRef:
		return errCollectionOutOfPlace()
	}
	if isConstant(arg) {
		return nil
	}
	return fmt.Errorf("operator %q compares a reference or a constant, got %s", op, termName(arg))
}

// validateSubject checks the operand an operator reads a value from rather than supplies one
// to: the left-hand side of membership and of a regular expression.
func validateSubject(op Operator, arg Expression, _ []Level) error {
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

// validateRegexSubject refuses a subject a pattern has nothing to match against. A string field,
// a word-valued field and an attribute all hold text; a duration or a timestamp does not, and
// nothing in this API says which of its spellings a pattern would be shown.
func validateRegexSubject(subject Expression) error {
	ref, ok := subject.(*FieldRef)
	if !ok || ref == nil {
		return nil
	}
	field, _ := LookupField(ref.Level, ref.Name)
	switch field.Type {
	case FieldTypeDuration, FieldTypeTimestamp:
		return fmt.Errorf("operator %q matches text, and %s.%s holds a %s",
			OpRegex, ref.Level, ref.Name, field.Type)
	}
	return nil
}

// domain is the kind of value a term holds, which is what decides whether two operands can be
// compared at all. Nothing has said what an untyped constant or an attribute holds, so those are
// unknown and compare against anything.
//
// Whether a domain has an order is a separate question, and the answer can differ within one: the
// two word-valued fields hold text but have no useful order (see orderable).
type domain int

const (
	domainUnknown domain = iota
	domainNumber
	domainDuration
	domainTimestamp
	domainText
	domainBool
)

// domainOf reads the kind of value a constant holds.
func domainOf(e Expression) domain {
	switch e.(type) {
	case *IntValue, *DoubleValue:
		return domainNumber
	case *DurationValue:
		return domainDuration
	case *TimestampValue:
		return domainTimestamp
	case *StringValue:
		return domainText
	case *BoolValue:
		return domainBool
	default:
		return domainUnknown
	}
}

// domainOfOperand reads the kind of value either side of a comparison holds. A built-in field
// holds what its declared type says; an attribute holds whatever storage wrote there, which is
// not this API's to know.
func domainOfOperand(e Expression) domain {
	if ref, ok := e.(*FieldRef); ok && ref != nil {
		// A field this API does not define is refused before an ordering is asked about, so the
		// zero Field's empty type is only ever reached by a caller checking one term directly.
		field, _ := LookupField(ref.Level, ref.Name)
		return domainOfFieldType(field.Type)
	}
	if _, ok := e.(*AttributeRef); ok {
		return domainUnknown
	}
	return domainOf(e)
}

// domainOfValueType reads the kind of value a declared wire type names, which is what a list's
// element type says about its elements.
func domainOfValueType(t ValueType) domain {
	switch t {
	case ValueTypeInt, ValueTypeDouble:
		return domainNumber
	case ValueTypeString:
		return domainText
	case ValueTypeBool:
		return domainBool
	default:
		return domainUnknown
	}
}

// domainOfFieldType reads the kind of value a built-in field holds. A field holding one of a
// closed set of words holds text, which is what makes a list of strings the right list for it.
func domainOfFieldType(t FieldType) domain {
	switch t {
	case FieldTypeDuration:
		return domainDuration
	case FieldTypeTimestamp:
		return domainTimestamp
	case FieldTypeString, FieldTypeSpanKind, FieldTypeSpanStatus:
		return domainText
	default:
		return domainUnknown
	}
}

// orderable reports whether an operand has an order to be compared within. Two do not: a boolean,
// and a field holding one of a closed set of words, because the kinds that sort after "server" is
// not a question about span kinds.
func orderable(e Expression) bool {
	if ref, ok := e.(*FieldRef); ok && ref != nil {
		field, _ := LookupField(ref.Level, ref.Name)
		return field.Type != FieldTypeSpanKind && field.Type != FieldTypeSpanStatus
	}
	return domainOf(e) != domainBool
}

// validatePattern checks a regular expression. RFC 0005 §5.3 makes it RE2 syntax, matched anywhere
// in the value and case-sensitively, so a pattern that will not parse is refused here rather than
// by whichever backend received it.
func validatePattern(pattern string) error {
	parsed, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return fmt.Errorf("operator %q takes a pattern in RE2 syntax: %w", OpRegex, err)
	}
	return checkPortable(parsed)
}

// checkPortable refuses the constructs the backends this lowers to do not all have. Elasticsearch,
// for one, reads `^` as a literal caret rather than as an anchor, so a pattern using it would be
// answered differently by each backend instead of being refused by the ones that cannot honor it.
func checkPortable(re *syntax.Regexp) error {
	switch re.Op {
	case syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText, syntax.OpEndText:
		return fmt.Errorf("operator %q matches anywhere in the value, so a pattern cannot anchor itself", OpRegex)
	case syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return fmt.Errorf("operator %q takes a pattern without word boundaries", OpRegex)
	}
	if re.Flags&syntax.NonGreedy != 0 {
		return fmt.Errorf("operator %q asks whether the value matches, so a quantifier cannot be lazy", OpRegex)
	}
	if re.Flags&syntax.FoldCase != 0 {
		return fmt.Errorf("operator %q matches case-sensitively, so a pattern cannot fold case", OpRegex)
	}
	for _, sub := range re.Sub {
		if err := checkPortable(sub); err != nil {
			return err
		}
	}
	return nil
}

// patternText reads the spelling of a constant that can serve as a regular expression. An untyped
// constant can: a pattern is written as a bare string and carries no wire hint.
func patternText(e Expression) (string, bool) {
	switch value := e.(type) {
	case *AnyValue:
		return value.Value, true
	case *StringValue:
		return value.Value, true
	}
	return "", false
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
