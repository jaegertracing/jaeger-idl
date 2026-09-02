// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allTerms is every term type, as the pointer a tree is built from, paired with the name an
// error message gives it. The tests below walk it, so a term added without a case in termName
// fails here rather than being reported to a caller as an empty term.
var allTerms = []struct {
	term Expression
	name string
}{
	{&AttributeRef{}, "an attribute reference"},
	{&FieldRef{}, "a field reference"},
	{&NestedRef{}, "a collection reference"},
	{&AnyValue{}, "an untyped constant"},
	{&StringValue{}, "a string constant"},
	{&IntValue{}, "an integer constant"},
	{&DoubleValue{}, "a floating-point constant"},
	{&BoolValue{}, "a boolean constant"},
	{&DurationValue{}, "a duration constant"},
	{&TimestampValue{}, "a timestamp constant"},
	{&List{}, "a list"},
	{&Call{}, "a predicate"},
}

// TestLevelsAndValueTypesAreCopies pins that a caller cannot reach the vocabulary through what
// the accessors hand back, since a consumer walking it must not be able to edit it.
func TestLevelsAndValueTypesAreCopies(t *testing.T) {
	Levels()[0] = Level("edited")
	ValueTypes()[0] = ValueType("edited")
	Operators()[0] = Operator("edited")
	assert.Equal(t, LevelSpan, levels[0])
	assert.Equal(t, ValueTypeString, valueTypes[0])
	assert.Equal(t, OpAnd, operators[0])
}

// TestLevelValid walks the vocabulary rather than listing the levels a second time, so a level
// added to levels is covered here without editing this test.
func TestLevelValid(t *testing.T) {
	assert.Equal(t, levels, Levels())
	for _, level := range Levels() {
		assert.True(t, level.Valid(), "%q is one of the defined levels", level)
	}
	assert.False(t, Level("").Valid(), "the empty level is the unqualified search, not a level")
	assert.False(t, Level("Span").Valid(), "the vocabulary is lower case")
	assert.False(t, Level("nowhere").Valid())
}

// TestOperators pins that the accessor reports every declared operator, so a consumer walking it
// to cover each case sees an operator added in a later release.
func TestOperators(t *testing.T) {
	assert.Equal(t, operators, Operators())
	assert.Contains(t, Operators(), OpSome, "the quantifier is an operator like any other")
	assert.NotContains(t, Operators(), Operator(""))
}

// TestValueTypeValid does the same for the constant value types.
func TestValueTypeValid(t *testing.T) {
	assert.Equal(t, valueTypes, ValueTypes())
	for _, valueType := range ValueTypes() {
		assert.True(t, valueType.Valid(), "%q is one of the defined value types", valueType)
	}
	assert.False(t, ValueType("").Valid(), `the empty type means "any type", not a type`)
	assert.False(t, ValueType("String").Valid(), "the vocabulary is lower case")
	assert.False(t, ValueType("duration").Valid(),
		"duration is a field type, not something a constant declares")
}

// TestExpressionTerms_ArePointersOnly pins that a term is a pointer. The embedded marker takes
// a pointer receiver so that a term value does not also satisfy Expression: a tree is built from
// pointers, and a value slipping in would pass every type switch written for one.
func TestExpressionTerms_ArePointersOnly(t *testing.T) {
	for _, test := range allTerms {
		value := reflect.New(reflect.TypeOf(test.term).Elem()).Elem().Interface()
		_, ok := value.(Expression)
		assert.False(t, ok, "%T must not satisfy Expression; only its pointer does", value)
	}
}

// TestExpressionTerms pins which types are filter terms. The marker method is what closes
// the interface to these, so a backend switching on the concrete type covers every case.
func TestExpressionTerms(t *testing.T) {
	for _, test := range allTerms {
		test.term.isExpression()
		assert.Equal(t, test.name, termName(test.term))
	}
	assert.Equal(t, "an empty term", termName(nil))
}

// unknownTerm is a term this package does not define. Only a test inside the package can build one,
// because the unexported marker closes the interface, and it exists so that the switches naming and
// validating a term are answerable for a term added later rather than falling off the end of a case
// list.
type unknownTerm struct {
	expressionTerm
}

func TestUnknownTerm(t *testing.T) {
	term := &unknownTerm{}
	assert.Equal(t, "an unknown term", termName(term))
	assert.False(t, isConstant(term))
	assert.False(t, isMissing(term))

	err := ValidateFilter(&Call{Op: OpEq, Args: []Expression{attr("a"), term}})
	require.ErrorContains(t, err, "got an unknown term")
}

// TestConstantsCarryTheirParsedValue pins the point of a typed constant: a consumer reads the
// value, rather than a string it has to parse again at every layer that touches it.
func TestConstantsCarryTheirParsedValue(t *testing.T) {
	assert.Equal(t, "banana", (&AnyValue{Value: "banana"}).Value)
	assert.Equal(t, "GET", (&StringValue{Value: "GET"}).Value)
	assert.Equal(t, int64(500), (&IntValue{Value: 500}).Value)
	assert.InEpsilon(t, 1.5, (&DoubleValue{Value: 1.5}).Value, 0.0001)
	assert.True(t, (&BoolValue{Value: true}).Value)
	assert.Equal(t, 2*time.Second, (&DurationValue{Value: 2 * time.Second}).Value)
	assert.Equal(t, time.Unix(0, 0).UTC(), (&TimestampValue{Value: time.Unix(0, 0).UTC()}).Value)
}

// TestConstantKinds pins the kind of value each constant holds, and which ones can serve as a
// regular expression. An untyped constant holds nothing known, because it is what a value with no
// wire hint arrives as and no operator can settle its type from its text alone.
func TestConstantKinds(t *testing.T) {
	domains := map[string]domain{}
	patterns := map[string]bool{}
	for _, test := range allTerms {
		if !isConstant(test.term) {
			continue
		}
		domains[test.name] = domainOf(test.term)
		if _, ok := patternText(test.term); ok {
			patterns[test.name] = true
		}
	}
	assert.Equal(t, map[string]domain{
		"an untyped constant":       domainUnknown,
		"a string constant":         domainText,
		"an integer constant":       domainNumber,
		"a floating-point constant": domainNumber,
		"a boolean constant":        domainBool,
		"a duration constant":       domainDuration,
		"a timestamp constant":      domainTimestamp,
	}, domains)
	assert.Equal(t, map[string]bool{
		"an untyped constant": true,
		"a string constant":   true,
	}, patterns)

	assert.False(t, isConstant(&List{}), "a list is only ever a membership operand")
	assert.False(t, isConstant(nil))
}
