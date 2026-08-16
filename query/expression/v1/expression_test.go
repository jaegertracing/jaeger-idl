// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExpressionTerms_ArePointersOnly pins that a term is a pointer. The embedded marker takes
// a pointer receiver so that a Reference value does not also satisfy Expression: a tree is
// built from pointers, and a value slipping in would pass every type switch written for one.
func TestExpressionTerms_ArePointersOnly(t *testing.T) {
	var _ Expression = &Reference{}
	var _ Expression = &Scalar{}
	var _ Expression = &List{}
	var _ Expression = &Call{}

	for _, value := range []any{Reference{}, Scalar{}, List{}, Call{}} {
		_, ok := value.(Expression)
		assert.False(t, ok, "%T must not satisfy Expression; only its pointer does", value)
	}
}

// TestExpressionTerms pins which types are filter terms. The marker method is what closes
// the interface to these four, so a backend switching on the concrete type covers every
// case.
func TestExpressionTerms(t *testing.T) {
	tests := []struct {
		term Expression
		name string
	}{
		{&Reference{}, "a reference"},
		{&Scalar{}, "a constant"},
		{&List{}, "a list"},
		{&Call{}, "a predicate"},
	}
	for _, test := range tests {
		test.term.isExpression()
		assert.Equal(t, test.name, termName(test.term))
	}
	assert.Equal(t, "an empty term", termName(nil))
}

// TestReference_IsAttribute pins that the question is not the Attr bit alone: an unqualified
// reference is an attribute however Attr is set, because no built-in field has an unqualified
// form.
func TestReference_IsAttribute(t *testing.T) {
	assert.True(t, (&Reference{Name: "http.method"}).IsAttribute(), "unqualified, Attr unset")
	assert.True(t, (&Reference{Name: "http.method", Attr: true}).IsAttribute())
	assert.True(t, (&Reference{Name: "http.method", Level: LevelSpan, Attr: true}).IsAttribute())
	assert.False(t, (&Reference{Name: "duration", Level: LevelSpan}).IsAttribute())
	assert.False(t, (&Reference{Level: LevelEvent}).IsAttribute(), "a collection is not an attribute")
}
