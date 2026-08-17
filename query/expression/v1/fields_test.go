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
	span := &Reference{Level: LevelSpan, Name: SpanFieldDuration}
	assert.True(t, span.IsField(LevelSpan, SpanFieldDuration))
	assert.False(t, span.IsField(LevelSpan, SpanFieldName), "a different field of the same level")

	event := &Reference{Level: LevelEvent, Name: EventFieldName}
	assert.False(t, event.IsField(LevelSpan, SpanFieldName), "the same name is a different field per level")
	assert.True(t, event.IsField(LevelEvent, EventFieldName))

	attribute := &Reference{Level: LevelSpan, Name: SpanFieldDuration, Attr: true}
	assert.False(t, attribute.IsField(LevelSpan, SpanFieldDuration), "an attribute is never the field")

	unqualified := &Reference{Name: ResourceFieldService}
	assert.False(t, unqualified.IsField(LevelResource, ResourceFieldService), "unqualified is an attribute")
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
	for _, level := range []Level{LevelSpan, LevelResource, LevelScope, LevelEvent, LevelLink} {
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
	_, ok := LookupField(LevelSpan, SpanFieldTraceID)
	assert.True(t, ok)
}

func TestLookupField(t *testing.T) {
	f, ok := LookupField(LevelSpan, SpanFieldDuration)
	require.True(t, ok)
	assert.True(t, f.Derived)

	_, ok = LookupField(LevelResource, SpanFieldDuration)
	assert.False(t, ok, "a span field is not a resource field")

	_, ok = LookupField(LevelSpan, "nonesuch")
	assert.False(t, ok)
}
