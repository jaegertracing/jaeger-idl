// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFields pins the enumeration as the query API's own: every level a reference can name has
// fields, every entry is named and typed, and the derived ones are marked so a backend knows it
// has to compute rather than read them.
func TestFields(t *testing.T) {
	byLevel := map[Level]int{}
	derived := map[string]bool{}
	for _, f := range Fields() {
		require.NotEmpty(t, f.Name, "every field is named")
		require.NotEmpty(t, f.Level, "every field belongs to a level")
		require.Contains(t, fieldTypes, f.Type, "field %q at the %q level declares no known type", f.Name, f.Level)
		byLevel[f.Level]++
		if f.Derived {
			derived[string(f.Level)+"."+f.Name] = true
		}
	}
	for _, level := range levels {
		assert.NotZero(t, byLevel[level], "level %q defines no field", level)
	}
	assert.Equal(t, map[string]bool{
		"span.duration":        true,
		"resource.service":     true,
		"event.timeSinceStart": true,
	}, derived, "the fields computed rather than read")
}

// TestVocabulariesAreCopies pins that a caller cannot edit the closed set of words a field holds
// into something a consumer would then honor. A consumer checking a constant against the set reads
// these same words, so a caller able to append one would change what every filter after it means.
func TestVocabulariesAreCopies(t *testing.T) {
	kinds := SpanKinds()
	kinds[0] = "banana"
	assert.Equal(t, "unspecified", SpanKinds()[0])

	statuses := SpanStatuses()
	statuses[0] = "banana"
	assert.Equal(t, "unset", SpanStatuses()[0])
}

// TestFields_DeclaredTypes pins the fields that are not text, since those are the ones whose
// constants are parsed and can be refused. Everything else is a string, including the IDs, the
// status and the span kind, which are text this API checks rather than types of their own.
func TestFields_DeclaredTypes(t *testing.T) {
	byType := map[FieldType][]string{}
	for _, f := range Fields() {
		byType[f.Type] = append(byType[f.Type], string(f.Level)+"."+f.Name)
	}
	assert.Equal(t, []string{"span.duration", "event.timeSinceStart"}, byType[FieldTypeDuration])
	assert.Equal(t, []string{"span.startTime", "span.endTime", "event.time"}, byType[FieldTypeTimestamp])
	assert.Equal(t, []string{"span.kind"}, byType[FieldTypeSpanKind])
	assert.Equal(t, []string{"span.status"}, byType[FieldTypeSpanStatus])
	for _, name := range []string{"span.traceID", "span.statusMessage", "resource.service"} {
		assert.Contains(t, byType[FieldTypeString], name,
			"an ID is a string: an ID nobody recorded reads the same as one being looked for")
	}
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
	assert.Equal(t, FieldTypeDuration, f.Type)

	_, ok = LookupField(LevelResource, SpanFieldDuration)
	assert.False(t, ok, "a span field is not a resource field")

	_, ok = LookupField(LevelSpan, "nonesuch")
	assert.False(t, ok)
}
