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

// TestFields_DeclaredTypes pins the fields that are not text, since those are the ones whose
// constants are parsed and can be refused (see ResolveConstants). Everything else is a string,
// including the IDs, the status and the span kind, which are validated spellings rather than
// types of their own.
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

// TestFieldTypes_AreAllReadable pins that every declared type has a rule for reading a
// constant as it. Without this, adding a type and forgetting the rule would refuse every
// constant compared against a field of that type.
func TestFieldTypes_AreAllReadable(t *testing.T) {
	require.NotEmpty(t, fieldTypes)
	for _, ft := range fieldTypes {
		t.Run(string(ft), func(t *testing.T) {
			_, err := readConstant(ft, spellingFor(ft))
			require.NoError(t, err)
		})
	}
}

// spellingFor is a value the given field type can be read from, so that walking the vocabulary
// does not turn into a test of each type's parser.
func spellingFor(t FieldType) string {
	switch t {
	case FieldTypeDuration:
		return "2s"
	case FieldTypeTimestamp:
		return "2026-08-16T18:56:20.123456789Z"
	case FieldTypeSpanKind:
		return SpanKinds[0]
	case FieldTypeSpanStatus:
		return SpanStatuses[0]
	default:
		return "anything"
	}
}
