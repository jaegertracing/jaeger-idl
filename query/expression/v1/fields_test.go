// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReference_IsField covers the ways a reference can fail to be a given built-in field:
// a different name, the same name at another level, and an attribute borrowing its spelling.
func TestReference_IsField(t *testing.T) {
	span := &Reference{Level: LevelSpan, Name: FieldDuration}
	assert.True(t, span.IsField(LevelSpan, FieldDuration))
	assert.False(t, span.IsField(LevelSpan, FieldName), "a different field of the same level")

	event := &Reference{Level: LevelEvent, Name: FieldName}
	assert.False(t, event.IsField(LevelSpan, FieldName), "the same name is a different field per level")
	assert.True(t, event.IsField(LevelEvent, FieldName))

	attribute := &Reference{Level: LevelSpan, Name: FieldDuration, Attr: true}
	assert.False(t, attribute.IsField(LevelSpan, FieldDuration), "an attribute is never the field")

	unqualified := &Reference{Name: FieldService}
	assert.False(t, unqualified.IsField(LevelResource, FieldService), "unqualified is an attribute")
}

// TestFields pins the enumeration as the query API's own: every level a reference can name has
// fields, every entry is named, and the derived ones are marked so a backend knows it has to
// compute rather than read them.
func TestFields(t *testing.T) {
	byLevel := map[Level]int{}
	derived := map[string]bool{}
	for _, f := range Fields() {
		require.NotEmpty(t, f.Name, "every field is named")
		require.NotEmpty(t, f.Level, "every field belongs to a level")
		byLevel[f.Level]++
		if f.Derived {
			derived[string(f.Level)+"."+f.Name] = true
		}
	}
	for _, level := range []Level{LevelSpan, LevelResource, LevelInstrumentation, LevelEvent, LevelLink} {
		assert.NotZero(t, byLevel[level], "level %q defines no field", level)
	}
	assert.Equal(t, map[string]bool{
		"span.duration":        true,
		"resource.service":     true,
		"event.timeSinceStart": true,
	}, derived, "the fields computed rather than read")
}

// TestFields_ReturnsACopy pins that a caller cannot reach the table through Fields, which a
// query builder handing the slice to a UI would otherwise do.
func TestFields_ReturnsACopy(t *testing.T) {
	Fields()[0].Name = "mutated"
	_, ok := LookupField(LevelSpan, FieldTraceID)
	assert.True(t, ok)
}

func TestLookupField(t *testing.T) {
	f, ok := LookupField(LevelSpan, FieldDuration)
	require.True(t, ok)
	assert.True(t, f.Derived)

	_, ok = LookupField(LevelResource, FieldDuration)
	assert.False(t, ok, "a span field is not a resource field")

	_, ok = LookupField(LevelSpan, "nonesuch")
	assert.False(t, ok)
}

// TestValidateFilter_RejectsUnknownField pins that naming a field this API does not define is
// refused, and that the message says how to ask for an attribute of that name instead.
func TestValidateFilter_RejectsUnknownField(t *testing.T) {
	err := ValidateFilter(eq(&Reference{Level: LevelSpan, Name: "durtion"}, &Scalar{Value: "2s"}))
	require.ErrorContains(t, err, `unknown built-in field "durtion" at the "span" level`)
	require.ErrorContains(t, err, "set attr to name an attribute instead")

	// The same spelling as an attribute is fine, because an attribute key is arbitrary.
	require.NoError(t, ValidateFilter(
		eq(&Reference{Level: LevelSpan, Name: "durtion", Attr: true}, &Scalar{Value: "2s"})))

	// A field of the wrong level is refused too.
	require.ErrorContains(t,
		ValidateFilter(eq(&Reference{Level: LevelResource, Name: FieldDuration}, &Scalar{Value: "2s"})),
		`unknown built-in field "duration" at the "resource" level`)
}
