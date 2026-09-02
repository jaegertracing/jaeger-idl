// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// allTerms is every term type, as the pointer a tree is built from. Listing them as Expression is
// itself the check that each one is a term, since only a type in this package can satisfy it.
var allTerms = []Expression{
	&AttributeRef{},
	&FieldRef{},
	&NestedRef{},
	&AnyValue{},
	&StringValue{},
	&IntValue{},
	&DoubleValue{},
	&BoolValue{},
	&DurationValue{},
	&TimestampValue{},
	&List{},
	&Call{},
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
	for _, term := range allTerms {
		value := reflect.New(reflect.TypeOf(term).Elem()).Elem().Interface()
		_, ok := value.(Expression)
		assert.False(t, ok, "%T must not satisfy Expression; only its pointer does", value)
	}
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
