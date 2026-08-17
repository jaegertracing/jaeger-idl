// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
	{&CollectionRef{}, "a collection reference"},
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

// TestConstantKinds pins which constants an ordered comparison and a regular expression accept.
// The untyped constant is in both, because it is what a value with no wire hint arrives as and
// neither operator can refuse it on its spelling alone.
func TestConstantKinds(t *testing.T) {
	ordered := map[string]bool{}
	text := map[string]bool{}
	for _, test := range allTerms {
		if !isConstant(test.term) {
			continue
		}
		if isOrderedConstant(test.term) {
			ordered[test.name] = true
		}
		if isTextConstant(test.term) {
			text[test.name] = true
		}
	}
	assert.Equal(t, map[string]bool{
		"an untyped constant":       true,
		"an integer constant":       true,
		"a floating-point constant": true,
		"a duration constant":       true,
		"a timestamp constant":      true,
	}, ordered)
	assert.Equal(t, map[string]bool{
		"an untyped constant": true,
		"a string constant":   true,
	}, text)

	assert.False(t, isConstant(&List{}), "a list is only ever a membership operand")
	assert.False(t, isConstant(nil))
}
