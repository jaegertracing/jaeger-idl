// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func spanField(name string) *FieldRef {
	return &FieldRef{Name: name, Level: LevelSpan}
}

// TestResolveConstants pins what each field type reads its constant as, and what resolution
// leaves alone: a constant that already has a type, a constant compared against an attribute,
// and a regular expression's pattern.
func TestResolveConstants(t *testing.T) {
	timestamp, err := time.Parse(time.RFC3339Nano, "2026-08-16T18:56:20.123456789Z")
	require.NoError(t, err)

	tests := []struct {
		name     string
		filter   *Call
		expected *Call
	}{
		{
			name:     "a duration field",
			filter:   &Call{Op: OpGt, Args: []Expression{spanField(SpanFieldDuration), &AnyValue{Value: "2s"}}},
			expected: &Call{Op: OpGt, Args: []Expression{spanField(SpanFieldDuration), &DurationValue{Value: 2 * time.Second}}},
		},
		{
			name:     "a timestamp field",
			filter:   &Call{Op: OpGte, Args: []Expression{spanField(SpanFieldStartTime), &AnyValue{Value: "2026-08-16T18:56:20.123456789Z"}}},
			expected: &Call{Op: OpGte, Args: []Expression{spanField(SpanFieldStartTime), &TimestampValue{Value: timestamp}}},
		},
		{
			name:     "a string field",
			filter:   eq(spanField(SpanFieldName), &AnyValue{Value: "GET /api"}),
			expected: eq(spanField(SpanFieldName), &StringValue{Value: "GET /api"}),
		},
		{
			name:     "the constant on the left",
			filter:   eq(&AnyValue{Value: "5s"}, spanField(SpanFieldDuration)),
			expected: eq(&DurationValue{Value: 5 * time.Second}, spanField(SpanFieldDuration)),
		},
		{
			name:     "an event field, which measures from its span's start",
			filter:   &Call{Op: OpLt, Args: []Expression{&FieldRef{Name: EventFieldTimeSinceStart, Level: LevelEvent}, &AnyValue{Value: "50us"}}},
			expected: &Call{Op: OpLt, Args: []Expression{&FieldRef{Name: EventFieldTimeSinceStart, Level: LevelEvent}, &DurationValue{Value: 50 * time.Microsecond}}},
		},
		{
			name:     "an attribute, which declares nothing",
			filter:   &Call{Op: OpGt, Args: []Expression{attr("http.response.size"), &AnyValue{Value: "500"}}},
			expected: &Call{Op: OpGt, Args: []Expression{attr("http.response.size"), &AnyValue{Value: "500"}}},
		},
		{
			name:     "a constant that already has a type",
			filter:   eq(spanField(SpanFieldName), &StringValue{Value: "GET /api"}),
			expected: eq(spanField(SpanFieldName), &StringValue{Value: "GET /api"}),
		},
		{
			name:     "a pattern, which is a pattern whatever the field holds",
			filter:   &Call{Op: OpRegex, Args: []Expression{spanField(SpanFieldDuration), &AnyValue{Value: ".*"}}},
			expected: &Call{Op: OpRegex, Args: []Expression{spanField(SpanFieldDuration), &AnyValue{Value: ".*"}}},
		},
		{
			name: "a membership list, which carries its own spelling",
			filter: &Call{Op: OpIn, Args: []Expression{
				spanField(SpanFieldDuration), &List{Values: []string{"2s", "3s"}},
			}},
			expected: &Call{Op: OpIn, Args: []Expression{
				spanField(SpanFieldDuration), &List{Values: []string{"2s", "3s"}},
			}},
		},
		{
			name:     "two fields, with no constant between them",
			filter:   &Call{Op: OpLt, Args: []Expression{spanField(SpanFieldStartTime), spanField(SpanFieldEndTime)}},
			expected: &Call{Op: OpLt, Args: []Expression{spanField(SpanFieldStartTime), spanField(SpanFieldEndTime)}},
		},
		{
			name: "a predicate buried under a quantifier and a negation",
			filter: &Call{Op: OpSome, Args: []Expression{
				&NestedRef{Level: LevelEvent},
				&Call{Op: OpNot, Args: []Expression{
					&Call{Op: OpGt, Args: []Expression{
						&FieldRef{Name: EventFieldTimeSinceStart, Level: LevelEvent},
						&AnyValue{Value: "1ms"},
					}},
				}},
			}},
			expected: &Call{Op: OpSome, Args: []Expression{
				&NestedRef{Level: LevelEvent},
				&Call{Op: OpNot, Args: []Expression{
					&Call{Op: OpGt, Args: []Expression{
						&FieldRef{Name: EventFieldTimeSinceStart, Level: LevelEvent},
						&DurationValue{Value: time.Millisecond},
					}},
				}},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, ValidateFilter(test.filter), "the fixture is a filter validation accepts")
			resolved, err := ResolveConstants(test.filter)
			require.NoError(t, err)
			assert.Equal(t, test.expected, resolved)
		})
	}
}

// TestResolveConstants_LeavesAnUndefinedFieldAlone pins that resolution has nothing to say
// about a field this API does not define: there is no type to read the constant as, and
// ValidateFilter is what refuses the reference.
func TestResolveConstants_LeavesAnUndefinedFieldAlone(t *testing.T) {
	filter := eq(spanField("durtion"), &AnyValue{Value: "2s"})
	require.Error(t, ValidateFilter(filter))

	resolved, err := ResolveConstants(filter)
	require.NoError(t, err)
	assert.Equal(t, filter, resolved)
}

// TestResolveConstants_RefusesAConstantThatWillNotParse pins the point of resolving against the
// field: the query boundary answers a malformed value, rather than each backend interpreting it
// its own way.
func TestResolveConstants_RefusesAConstantThatWillNotParse(t *testing.T) {
	tests := []struct {
		name        string
		filter      *Call
		expectedErr string
	}{
		{
			name:        "a word where a duration belongs",
			filter:      &Call{Op: OpGt, Args: []Expression{spanField(SpanFieldDuration), &AnyValue{Value: "banana"}}},
			expectedErr: `cannot compare span.duration against "banana": time: invalid duration "banana"`,
		},
		{
			name:        "a bare number, since a duration carries its unit",
			filter:      &Call{Op: OpGt, Args: []Expression{spanField(SpanFieldDuration), &AnyValue{Value: "500"}}},
			expectedErr: `cannot compare span.duration against "500"`,
		},
		{
			name:        "a timestamp that is not RFC 3339",
			filter:      &Call{Op: OpLt, Args: []Expression{spanField(SpanFieldEndTime), &AnyValue{Value: "yesterday"}}},
			expectedErr: `cannot compare span.endTime against "yesterday"`,
		},
		{
			name: "buried in a conjunction",
			filter: &Call{Op: OpAnd, Args: []Expression{
				eq(attr("a"), &AnyValue{Value: "1"}),
				&Call{Op: OpGt, Args: []Expression{spanField(SpanFieldDuration), &AnyValue{Value: "banana"}}},
			}},
			expectedErr: `cannot compare span.duration against "banana"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved, err := ResolveConstants(test.filter)
			require.ErrorContains(t, err, test.expectedErr)
			assert.Nil(t, resolved)
		})
	}
}

// TestResolveConstants_LeavesItsInputAlone pins that resolution rewrites nodes into a new tree
// rather than annotating the one it was given, which is what keeps a query interceptor's later
// edit from leaving anything stale behind.
// TestResolveConstants_ChecksMembershipElements pins that a spelling refused under a comparison
// is refused under membership too. The list is not rewritten, so what this asserts is the
// refusal; a list declaring its own element type says how to read itself and is left alone.
func TestResolveConstants_ChecksMembershipElements(t *testing.T) {
	duration := &FieldRef{Name: SpanFieldDuration, Level: LevelSpan}

	_, err := ResolveConstants(&Call{Op: OpIn, Args: []Expression{
		duration, &List{Values: []string{"2s", "banana"}},
	}})
	require.ErrorContains(t, err, `cannot compare span.duration against "banana"`)

	_, err = ResolveConstants(&Call{Op: OpNotIn, Args: []Expression{
		duration, &List{Values: []string{"2s", "3m"}},
	}})
	require.NoError(t, err)

	_, err = ResolveConstants(&Call{Op: OpIn, Args: []Expression{
		duration, &List{Values: []string{"banana"}, Type: ValueTypeString},
	}})
	require.NoError(t, err, "a declared element type says how to read the list")

	_, err = ResolveConstants(&Call{Op: OpIn, Args: []Expression{
		&AttributeRef{Key: "size"}, &List{Values: []string{"banana"}},
	}})
	require.NoError(t, err, "an attribute's values are storage's to read")

	// ValidateFilter refuses both of these, so resolution only has to not choke on them.
	_, err = ResolveConstants(&Call{Op: OpIn, Args: []Expression{
		duration, &AnyValue{Value: "2s"},
	}})
	require.NoError(t, err, "membership without a list is ValidateFilter's refusal")

	_, err = ResolveConstants(&Call{Op: OpIn, Args: []Expression{
		&FieldRef{Name: "nosuchfield", Level: LevelSpan}, &List{Values: []string{"banana"}},
	}})
	require.NoError(t, err, "an undefined field has no type to read against")
}

// TestResolveConstants_ChecksEnumSpellings pins the two fields that hold one of a closed set of
// words. A misspelling can never match any span, so it is answered here with the set named,
// rather than passed to a backend that would return nothing and say why.
func TestResolveConstants_ChecksEnumSpellings(t *testing.T) {
	kind := &FieldRef{Name: SpanFieldKind, Level: LevelSpan}
	status := &FieldRef{Name: SpanFieldStatus, Level: LevelSpan}

	got, err := ResolveConstants(&Call{Op: OpEq, Args: []Expression{kind, &AnyValue{Value: "server"}}})
	require.NoError(t, err)
	assert.Equal(t, &StringValue{Value: "server"}, got.Args[1])

	_, err = ResolveConstants(&Call{Op: OpEq, Args: []Expression{kind, &AnyValue{Value: "SPAN_KIND_SERVER"}}})
	require.ErrorContains(t, err, `cannot compare span.kind against "SPAN_KIND_SERVER"`)
	require.ErrorContains(t, err, "not one of unspecified, internal, server, client, producer, consumer")

	_, err = ResolveConstants(&Call{Op: OpEq, Args: []Expression{status, &AnyValue{Value: "Error"}}})
	require.ErrorContains(t, err, "not one of unset, ok, error")

	_, err = ResolveConstants(&Call{Op: OpIn, Args: []Expression{
		status, &List{Values: []string{"ok", "banana"}},
	}})
	require.ErrorContains(t, err, `cannot compare span.status against "banana"`)

	// An ID stays a string: one nobody recorded reads the same as one being looked for.
	_, err = ResolveConstants(&Call{Op: OpEq, Args: []Expression{
		&FieldRef{Name: SpanFieldTraceID, Level: LevelSpan}, &AnyValue{Value: "not-hex"},
	}})
	require.NoError(t, err)
}

func TestResolveConstants_LeavesItsInputAlone(t *testing.T) {
	constant := &AnyValue{Value: "2s"}
	filter := &Call{Op: OpGt, Args: []Expression{spanField(SpanFieldDuration), constant}}

	resolved, err := ResolveConstants(filter)
	require.NoError(t, err)

	assert.Equal(t, &DurationValue{Value: 2 * time.Second}, resolved.Args[1])
	assert.Same(t, constant, filter.Args[1], "the original constant is still the one it was")
	assert.NotSame(t, filter, resolved, "the tree is rebuilt rather than edited")
}

// TestResolveConstants_NeverPanics pins that resolution answers for any tree, the same way
// validation does, since a decoder can produce trees a caller could not build.
func TestResolveConstants_NeverPanics(t *testing.T) {
	trees := map[string]*Call{
		"no operator":                {},
		"no arguments":               {Op: OpEq},
		"a nil argument":             {Op: OpEq, Args: []Expression{nil, nil}},
		"a nil nested call":          {Op: OpAnd, Args: []Expression{(*Call)(nil), (*Call)(nil)}},
		"a nil field reference":      {Op: OpEq, Args: []Expression{(*FieldRef)(nil), &AnyValue{Value: "1"}}},
		"one argument too few":       {Op: OpEq, Args: []Expression{spanField(SpanFieldDuration)}},
		"a field against two things": {Op: OpEq, Args: []Expression{spanField(SpanFieldDuration), &AnyValue{Value: "2s"}, &AnyValue{Value: "3s"}}},
	}
	for name, filter := range trees {
		t.Run(name, func(t *testing.T) {
			assert.NotPanics(t, func() { _, _ = ResolveConstants(filter) })
		})
	}

	resolved, err := ResolveConstants(nil)
	require.ErrorContains(t, err, "filter is empty")
	assert.Nil(t, resolved)
}

// TestReadConstant_RefusesAnUndeclaredType pins the answer when a field type has no rule for
// reading its constants, which is the state a type added to the vocabulary alone would leave.
func TestReadConstant_RefusesAnUndeclaredType(t *testing.T) {
	value, err := readConstant("nonesuch", "2s")
	require.ErrorContains(t, err, `no rule for reading a constant as "nonesuch"`)
	assert.Nil(t, value)
}
