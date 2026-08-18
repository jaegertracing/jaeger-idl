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
			name:     "a pattern, which stays a pattern rather than becoming the field's type",
			filter:   &Call{Op: OpRegex, Args: []Expression{spanField(SpanFieldName), &AnyValue{Value: ".*"}}},
			expected: &Call{Op: OpRegex, Args: []Expression{spanField(SpanFieldName), &AnyValue{Value: ".*"}}},
		},
		{
			name: "a membership list, which carries its elements as text",
			filter: &Call{Op: OpIn, Args: []Expression{
				spanField(SpanFieldDuration), &List{Values: []string{"2s", "3s"}},
			}},
			expected: &Call{Op: OpIn, Args: []Expression{
				spanField(SpanFieldDuration), &List{Values: []string{"2s", "3s"}},
			}},
		},
		{
			name:     "a comparison against an attribute, whose type only storage knows",
			filter:   eq(attr("http.method"), &AnyValue{Value: "GET"}),
			expected: eq(attr("http.method"), &AnyValue{Value: "GET"}),
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
// TestResolveConstants_ChecksMembershipElements pins that a value refused under a comparison
// is refused under membership too, and that a declared element type does not exempt the list
// from either half of that: the type has to be one the field could hold, and the elements have
// to be readable as it. The list is not rewritten, so what this asserts is the refusal.
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
	require.ErrorContains(t, err, "cannot compare span.duration against a list of string")

	_, err = ResolveConstants(&Call{Op: OpIn, Args: []Expression{
		duration, &List{Values: []string{"12"}, Type: ValueTypeInt},
	}})
	require.ErrorContains(t, err, "cannot compare span.duration against a list of int")

	_, err = ResolveConstants(&Call{Op: OpIn, Args: []Expression{
		duration, &List{Values: []string{"true"}, Type: ValueTypeBool},
	}})
	require.ErrorContains(t, err, "cannot compare span.duration against a list of bool")

	_, err = ResolveConstants(&Call{Op: OpIn, Args: []Expression{
		&FieldRef{Name: SpanFieldName, Level: LevelSpan},
		&List{Values: []string{"GET /a", "GET /b"}, Type: ValueTypeString},
	}})
	require.NoError(t, err, "a declared type the field can hold, with elements that read as it")

	_, err = ResolveConstants(&Call{Op: OpIn, Args: []Expression{
		&AttributeRef{Key: "size"}, &List{Values: []string{"banana"}},
	}})
	require.NoError(t, err, "an attribute's values are storage's to read")

	_, err = ResolveConstants(&Call{Op: OpIn, Args: []Expression{
		&FieldRef{Name: SpanFieldKind, Level: LevelSpan},
		&List{Values: []string{"server", "client"}, Type: ValueTypeString},
	}})
	require.NoError(t, err, "a word-valued field holds text, so a list of strings suits it")

	_, err = ResolveConstants(&Call{Op: OpIn, Args: []Expression{
		&FieldRef{Name: SpanFieldStatus, Level: LevelSpan},
		&List{Values: []string{"banana"}, Type: ValueTypeString},
	}})
	require.ErrorContains(t, err, "not one of unset, ok, error")

	_, err = ResolveConstants(&Call{Op: OpIn, Args: []Expression{
		&FieldRef{Name: SpanFieldName, Level: LevelSpan},
		&List{Values: []string{"true"}, Type: ValueTypeBool},
	}})
	require.ErrorContains(t, err, "cannot compare span.name against a list of bool")

	// A declared element type is authoritative wherever it appears, so the elements are read as
	// it whether or not there is a field opposite to compare it with.
	_, err = ResolveConstants(&Call{Op: OpIn, Args: []Expression{
		&AttributeRef{Key: "size"}, &List{Values: []string{"banana"}, Type: ValueTypeInt},
	}})
	require.ErrorContains(t, err, `element "banana" of a list of int`)

	_, err = ResolveConstants(&Call{Op: OpIn, Args: []Expression{
		&AttributeRef{Key: "size"}, &List{Values: []string{"12", "banana"}, Type: ValueTypeDouble},
	}})
	require.ErrorContains(t, err, `element "banana" of a list of double`)

	_, err = ResolveConstants(&Call{Op: OpIn, Args: []Expression{
		&AttributeRef{Key: "cache.hit"}, &List{Values: []string{"yes"}, Type: ValueTypeBool},
	}})
	require.ErrorContains(t, err, `element "yes" of a list of bool`)

	_, err = ResolveConstants(&Call{Op: OpIn, Args: []Expression{
		&AttributeRef{Key: "size"}, &List{Values: []string{"12"}, Type: ValueTypeInt},
	}})
	require.NoError(t, err, "elements that read as their declared type")

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
// words. A word outside the set can never match any span, so it is answered here with the set,
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

	// Declaring the constant a string does not get it past the vocabulary. Validation has nothing
	// to say about it: span.status holds text and so does the constant.
	_, err = ResolveConstants(&Call{Op: OpEq, Args: []Expression{status, &StringValue{Value: "banana"}}})
	require.ErrorContains(t, err, `cannot compare span.status against "banana"`)
	require.ErrorContains(t, err, "not one of unset, ok, error")

	got, err = ResolveConstants(&Call{Op: OpEq, Args: []Expression{kind, &StringValue{Value: "client"}}})
	require.NoError(t, err)
	assert.Equal(t, &StringValue{Value: "client"}, got.Args[1])
}

// TestResolveConstants_AcceptsCompatibleTypedConstants is the other half: a constant whose
// declared type the field holds needs no rewriting and is passed through as it stands.
func TestResolveConstants_AcceptsCompatibleTypedConstants(t *testing.T) {
	tests := []struct {
		name     string
		filter   *Call
		expected Expression
	}{
		{
			name: "a duration against a duration field",
			filter: &Call{Op: OpGt, Args: []Expression{
				spanField(SpanFieldDuration), &DurationValue{Value: 2 * time.Second},
			}},
			expected: &DurationValue{Value: 2 * time.Second},
		},
		{
			name: "a string against a text field",
			filter: &Call{Op: OpEq, Args: []Expression{
				spanField(SpanFieldName), &StringValue{Value: "GET /"},
			}},
			expected: &StringValue{Value: "GET /"},
		},
		{
			name: "a boolean against a field that holds one, which no built-in field does",
			filter: &Call{Op: OpEq, Args: []Expression{
				&AttributeRef{Key: "cache.hit"}, &BoolValue{Value: true},
			}},
			expected: &BoolValue{Value: true},
		},
		{
			name: "an integer against an attribute, whose type only storage knows",
			filter: &Call{Op: OpGt, Args: []Expression{
				&AttributeRef{Key: "http.status_code"}, &IntValue{Value: 500},
			}},
			expected: &IntValue{Value: 500},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved, err := ResolveConstants(test.filter)
			require.NoError(t, err)
			assert.Equal(t, test.expected, resolved.Args[1])
		})
	}
}

// TestResolveConstants_PutsTheReferenceFirst pins the orientation every consumer downstream
// relies on. A caller may write the constant on the left, so resolution swaps the operands and
// inverts an ordered operator, which leaves the query asking the same thing.
func TestResolveConstants_PutsTheReferenceFirst(t *testing.T) {
	duration := spanField(SpanFieldDuration)
	name := spanField(SpanFieldName)

	tests := []struct {
		name     string
		filter   *Call
		expected *Call
	}{
		{
			name:     "greater than becomes less than",
			filter:   &Call{Op: OpGt, Args: []Expression{&AnyValue{Value: "2s"}, duration}},
			expected: &Call{Op: OpLt, Args: []Expression{duration, &DurationValue{Value: 2 * time.Second}}},
		},
		{
			name:     "at least becomes at most",
			filter:   &Call{Op: OpGte, Args: []Expression{&AnyValue{Value: "2s"}, duration}},
			expected: &Call{Op: OpLte, Args: []Expression{duration, &DurationValue{Value: 2 * time.Second}}},
		},
		{
			name:     "less than becomes greater than",
			filter:   &Call{Op: OpLt, Args: []Expression{&AnyValue{Value: "2s"}, duration}},
			expected: &Call{Op: OpGt, Args: []Expression{duration, &DurationValue{Value: 2 * time.Second}}},
		},
		{
			name:     "at most becomes at least",
			filter:   &Call{Op: OpLte, Args: []Expression{&AnyValue{Value: "2s"}, duration}},
			expected: &Call{Op: OpGte, Args: []Expression{duration, &DurationValue{Value: 2 * time.Second}}},
		},
		{
			name:     "equality keeps its operator",
			filter:   &Call{Op: OpEq, Args: []Expression{&AnyValue{Value: "GET /"}, name}},
			expected: &Call{Op: OpEq, Args: []Expression{name, &StringValue{Value: "GET /"}}},
		},
		{
			name:     "inequality keeps its operator",
			filter:   &Call{Op: OpNe, Args: []Expression{&AnyValue{Value: "GET /"}, name}},
			expected: &Call{Op: OpNe, Args: []Expression{name, &StringValue{Value: "GET /"}}},
		},
		{
			name:     "a comparison already the right way round is left alone",
			filter:   &Call{Op: OpGt, Args: []Expression{duration, &AnyValue{Value: "2s"}}},
			expected: &Call{Op: OpGt, Args: []Expression{duration, &DurationValue{Value: 2 * time.Second}}},
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

// TestResolveConstants_LeavesTwoReferencesAlone pins that resolution answers for a tree
// validation would have refused. There is no constant to read between two references, and
// refusing the comparison is ValidateFilter's job rather than this stage's.
// TestResolveConstants_ReadsEveryListElement walks the list forms a filter can carry, since the
// element type comes from two different places and an element that cannot be read as it is the
// one thing membership refuses.
func TestResolveConstants_ReadsEveryListElement(t *testing.T) {
	tests := []struct {
		name        string
		filter      *Call
		expectedErr string
	}{
		{
			name: "a declared type its elements read as",
			filter: &Call{Op: OpIn, Args: []Expression{
				attr("size"), &List{Values: []string{"1", "2"}, Type: ValueTypeInt},
			}},
		},
		{
			name: "durations, whose type the field supplies",
			filter: &Call{Op: OpIn, Args: []Expression{
				spanField(SpanFieldDuration), &List{Values: []string{"2s", "50us"}},
			}},
		},
		{
			name: "instants, whose type the field supplies",
			filter: &Call{Op: OpNotIn, Args: []Expression{
				spanField(SpanFieldStartTime), &List{Values: []string{"2026-08-18T00:00:00Z"}},
			}},
		},
		{
			name: "words a closed set holds",
			filter: &Call{Op: OpIn, Args: []Expression{
				spanField(SpanFieldKind), &List{Values: []string{"server", "client"}, Type: ValueTypeString},
			}},
		},
		{
			name: "elements of mixed types under one declared type",
			filter: &Call{Op: OpIn, Args: []Expression{
				attr("size"), &List{Values: []string{"1", "true"}, Type: ValueTypeInt},
			}},
			expectedErr: `element "true" of a list of int`,
		},
		{
			name: "a duration among instants",
			filter: &Call{Op: OpIn, Args: []Expression{
				spanField(SpanFieldStartTime), &List{Values: []string{"2026-08-18T00:00:00Z", "2s"}},
			}},
			expectedErr: `cannot compare span.startTime against "2s"`,
		},
		{
			name: "a word outside the set the field holds",
			filter: &Call{Op: OpIn, Args: []Expression{
				spanField(SpanFieldKind), &List{Values: []string{"server", "banana"}, Type: ValueTypeString},
			}},
			expectedErr: "not one of unspecified, internal, server, client, producer, consumer",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, ValidateFilter(test.filter), "the fixture is a filter validation accepts")
			resolved, err := ResolveConstants(test.filter)
			if test.expectedErr != "" {
				require.ErrorContains(t, err, test.expectedErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.filter.Args[1], resolved.Args[1], "a list is carried over as it stands")
		})
	}
}

func TestResolveConstants_LeavesTwoReferencesAlone(t *testing.T) {
	filter := &Call{Op: OpLt, Args: []Expression{
		spanField(SpanFieldStartTime), spanField(SpanFieldEndTime),
	}}
	resolved, err := ResolveConstants(filter)
	require.NoError(t, err)
	assert.Equal(t, filter, resolved)
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
// TestResolveConstants_BoundsNesting pins that resolution bounds its own recursion. It is
// documented to answer for any tree, including one validation never saw, so a filter that contains
// itself has to stop here too rather than run the stack out.
// TestReadElement covers the one reading operation a consumer needs for a list: the type comes from
// the list where it declares one, and from the field opposite it where it does not. On a finalized
// filter it cannot fail, since finalizing refused every element that would not read.
func TestReadElement(t *testing.T) {
	tests := []struct {
		name      string
		list      *List
		fieldType FieldType
		element   string
		expected  Expression
		wantErr   string
	}{
		{
			name:     "a declared integer",
			list:     &List{Values: []string{"500"}, Type: ValueTypeInt},
			element:  "500",
			expected: &IntValue{Value: 500},
		},
		{
			name:     "a declared double",
			list:     &List{Values: []string{"1.50"}, Type: ValueTypeDouble},
			element:  "1.50",
			expected: &DoubleValue{Value: 1.5},
		},
		{
			name:     "a declared boolean",
			list:     &List{Values: []string{"true"}, Type: ValueTypeBool},
			element:  "true",
			expected: &BoolValue{Value: true},
		},
		{
			name:     "a declared string",
			list:     &List{Values: []string{"/cart"}, Type: ValueTypeString},
			element:  "/cart",
			expected: &StringValue{Value: "/cart"},
		},
		{
			name:      "a duration, whose type the field supplies",
			list:      &List{Values: []string{"2s"}},
			fieldType: FieldTypeDuration,
			element:   "2s",
			expected:  &DurationValue{Value: 2 * time.Second},
		},
		{
			name:      "a word the field's closed set holds",
			list:      &List{Values: []string{"server"}},
			fieldType: FieldTypeSpanKind,
			element:   "server",
			expected:  &StringValue{Value: "server"},
		},
		{
			name:      "a word outside that set",
			list:      &List{Values: []string{"banana"}},
			fieldType: FieldTypeSpanKind,
			element:   "banana",
			wantErr:   "not one of unspecified, internal, server, client, producer, consumer",
		},
		{
			name:    "an element that is not the type the list declares",
			list:    &List{Values: []string{"banana"}, Type: ValueTypeInt},
			element: "banana",
			wantErr: `element "banana" of a list of int`,
		},
		{
			name:    "no list at all",
			element: "500",
			wantErr: "list is empty",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := ReadElement(test.list, test.fieldType, test.element)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.expected, value)
		})
	}
}

// TestReadConstant covers the same reading for a value the wire carried as text against a built-in
// field, which is what finalizing does and what a consumer holding an unfinalized tree needs.
func TestReadConstant(t *testing.T) {
	value, err := ReadConstant(FieldTypeDuration, "50us")
	require.NoError(t, err)
	assert.Equal(t, &DurationValue{Value: 50 * time.Microsecond}, value)

	_, err = ReadConstant(FieldTypeTimestamp, "yesterday")
	require.Error(t, err)
}

func TestResolveConstants_BoundsNesting(t *testing.T) {
	resolved, err := ResolveConstants(nestedTo(MaxNestingDepth))
	require.NoError(t, err)
	assert.NotNil(t, resolved)

	_, err = ResolveConstants(nestedTo(MaxNestingDepth + 1))
	require.ErrorIs(t, err, ErrTooDeeplyNested)

	cycle := &Call{Op: OpNot}
	cycle.Args = []Expression{cycle}
	_, err = ResolveConstants(cycle)
	require.ErrorIs(t, err, ErrTooDeeplyNested)
}

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
