// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func eq(left, right Expression) *Call {
	return &Call{Op: OpEq, Args: []Expression{left, right}}
}

func attr(key string) *AttributeRef {
	return &AttributeRef{Key: key}
}

func TestValidateFilter_Accepts(t *testing.T) {
	tests := []struct {
		name   string
		filter *Call
	}{
		{
			name:   "unqualified attribute equality",
			filter: eq(attr("http.status_code"), &AnyValue{Value: "500"}),
		},
		{
			name:   "text ordered against text",
			filter: &Call{Op: OpGt, Args: []Expression{&FieldRef{Name: SpanFieldName, Level: LevelSpan}, &StringValue{Value: "m"}}},
		},
		{
			name:   "an attribute ordered against a number, since storage decides its type",
			filter: &Call{Op: OpGt, Args: []Expression{attr("size"), &IntValue{Value: 500}}},
		},
		{
			name:   "a timestamp field against an instant",
			filter: &Call{Op: OpLt, Args: []Expression{&FieldRef{Name: SpanFieldStartTime, Level: LevelSpan}, &TimestampValue{Value: time.Unix(0, 0).UTC()}}},
		},
		{
			name:   "level-qualified attribute",
			filter: eq(&AttributeRef{Key: "k8s.pod.name", Level: LevelResource}, &StringValue{Value: "cart-0"}),
		},
		{
			name: "built-in field against a duration",
			filter: &Call{Op: OpGt, Args: []Expression{
				&FieldRef{Name: SpanFieldDuration, Level: LevelSpan},
				&AnyValue{Value: "2s"},
			}},
		},
		{
			name: "the same attribute key at two levels",
			filter: eq(
				&AttributeRef{Key: "enduser.id", Level: LevelSpan},
				&AttributeRef{Key: "enduser.id", Level: LevelResource},
			),
		},
		{
			name: "two attributes ordered against each other",
			filter: &Call{Op: OpGt, Args: []Expression{
				&AttributeRef{Key: "queue.depth.after"},
				&AttributeRef{Key: "queue.depth.before"},
			}},
		},
		{
			name: "two built-in fields holding the same kind of value",
			filter: &Call{Op: OpLt, Args: []Expression{
				&FieldRef{Name: SpanFieldStartTime, Level: LevelSpan},
				&FieldRef{Name: SpanFieldEndTime, Level: LevelSpan},
			}},
		},
		{
			name:   "two constants, which asks nothing about the span but is still a comparison",
			filter: eq(&IntValue{Value: 1}, &IntValue{Value: 1}),
		},
		{
			name: "conjunction of two predicates",
			filter: &Call{Op: OpAnd, Args: []Expression{
				eq(attr("a"), &AnyValue{Value: "1"}),
				eq(attr("b"), &AnyValue{Value: "2"}),
			}},
		},
		{
			name: "nested disjunction under a negation",
			filter: &Call{Op: OpNot, Args: []Expression{
				&Call{Op: OpOr, Args: []Expression{
					eq(attr("a"), &AnyValue{Value: "1"}),
					eq(attr("b"), &AnyValue{Value: "2"}),
				}},
			}},
		},
		{
			name: "set membership",
			filter: &Call{Op: OpIn, Args: []Expression{
				attr("http.status_code"),
				&List{Values: []string{"500", "503"}, Type: ValueTypeInt},
			}},
		},
		{
			name:   "existence of an attribute",
			filter: &Call{Op: OpExists, Args: []Expression{attr("error")}},
		},
		{
			name: "a regular expression over a field",
			filter: &Call{Op: OpRegex, Args: []Expression{
				&FieldRef{Name: SpanFieldName, Level: LevelSpan},
				&AnyValue{Value: "GET .*"},
			}},
		},
		{
			name: "membership of a field holding one of a set of words in a list of strings",
			filter: &Call{Op: OpIn, Args: []Expression{
				&FieldRef{Name: SpanFieldKind, Level: LevelSpan},
				&List{Values: []string{"server", "client"}, Type: ValueTypeString},
			}},
		},
		{
			name: "a comparison written with the constant on the left",
			filter: &Call{Op: OpGt, Args: []Expression{
				&AnyValue{Value: "2s"}, &FieldRef{Name: SpanFieldDuration, Level: LevelSpan},
			}},
		},
		{
			name: "a regular expression over a field holding one of a set of words",
			filter: &Call{Op: OpRegex, Args: []Expression{
				&FieldRef{Name: SpanFieldKind, Level: LevelSpan},
				&StringValue{Value: "produ.*"},
			}},
		},
		{
			name: "a regular expression with a typed pattern",
			filter: &Call{Op: OpRegex, Args: []Expression{
				attr("http.route"),
				&StringValue{Value: "/api/.*"},
			}},
		},
		{
			name: "an ordered comparison of typed constants",
			filter: &Call{Op: OpGte, Args: []Expression{
				attr("http.response.size"),
				&IntValue{Value: 500},
			}},
		},
		{
			name: "correlated match over the event collection",
			filter: &Call{Op: OpSome, Args: []Expression{
				&NestedRef{Level: LevelEvent},
				&Call{Op: OpAnd, Args: []Expression{
					eq(&FieldRef{Name: EventFieldName, Level: LevelEvent}, &StringValue{Value: "exception"}),
					&Call{Op: OpGt, Args: []Expression{
						&FieldRef{Name: EventFieldTimeSinceStart, Level: LevelEvent},
						&AnyValue{Value: "50us"},
					}},
				}},
			}},
		},
		{
			name: "a quantifier nested over the other collection",
			filter: &Call{Op: OpSome, Args: []Expression{
				&NestedRef{Level: LevelEvent},
				&Call{Op: OpSome, Args: []Expression{
					&NestedRef{Level: LevelLink},
					&Call{Op: OpExists, Args: []Expression{&FieldRef{Name: LinkFieldTraceID, Level: LevelLink}}},
				}},
			}},
		},
		{
			name: "two quantifiers over the same level side by side",
			filter: &Call{Op: OpAnd, Args: []Expression{
				&Call{Op: OpSome, Args: []Expression{
					&NestedRef{Level: LevelEvent},
					eq(&FieldRef{Name: EventFieldName, Level: LevelEvent}, &StringValue{Value: "exception"}),
				}},
				&Call{Op: OpSome, Args: []Expression{
					&NestedRef{Level: LevelEvent},
					eq(&FieldRef{Name: EventFieldName, Level: LevelEvent}, &StringValue{Value: "retry"}),
				}},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, ValidateFilter(test.filter))
		})
	}
}

func TestValidateFilter_Rejects(t *testing.T) {
	tests := []struct {
		name        string
		filter      *Call
		expectedErr string
	}{
		{
			name:        "no filter",
			filter:      nil,
			expectedErr: "filter is empty",
		},
		{
			name:        "unknown operator",
			expectedErr: `unknown filter operator "matches"`,
			filter:      &Call{Op: "matches", Args: []Expression{attr("a"), &AnyValue{Value: "b"}}},
		},
		{
			name:        "conjunction of one",
			expectedErr: `operator "and" takes at least two arguments, got 1`,
			filter:      &Call{Op: OpAnd, Args: []Expression{eq(attr("a"), &AnyValue{Value: "1"})}},
		},
		{
			name:        "conjunction over a bare reference",
			expectedErr: `operator "and" takes predicates as arguments, got an attribute reference`,
			filter: &Call{Op: OpAnd, Args: []Expression{
				attr("a"),
				eq(attr("b"), &AnyValue{Value: "1"}),
			}},
		},
		{
			name:        "invalid predicate nested in a conjunction",
			expectedErr: `unknown filter level "pod"`,
			filter: &Call{Op: OpAnd, Args: []Expression{
				eq(attr("a"), &AnyValue{Value: "1"}),
				eq(&AttributeRef{Key: "b", Level: "pod"}, &AnyValue{Value: "2"}),
			}},
		},
		{
			name:        "negation of two predicates",
			expectedErr: `operator "not" takes 1 argument(s), got 2`,
			filter: &Call{Op: OpNot, Args: []Expression{
				eq(attr("a"), &AnyValue{Value: "1"}),
				eq(attr("b"), &AnyValue{Value: "2"}),
			}},
		},
		{
			name:        "equality of one operand",
			expectedErr: `operator "eq" takes 2 argument(s), got 1`,
			filter:      &Call{Op: OpEq, Args: []Expression{attr("a")}},
		},
		{
			name:        "equality of constants of different kinds",
			expectedErr: `operator "eq" compares an integer constant against a string constant, which hold different kinds of value`,
			filter:      eq(&IntValue{Value: 1}, &StringValue{Value: "1"}),
		},
		{
			name:        "existence of a constant",
			expectedErr: `operator "exists" takes a reference, got an untyped constant`,
			filter:      &Call{Op: OpExists, Args: []Expression{&AnyValue{Value: "a"}}},
		},
		{
			name:        "existence of an attribute with no key",
			expectedErr: "attribute reference has no key",
			filter:      &Call{Op: OpExists, Args: []Expression{&AttributeRef{Level: LevelSpan}}},
		},
		{
			name:        "existence of a field with no name",
			expectedErr: "field reference has no name",
			filter:      &Call{Op: OpExists, Args: []Expression{&FieldRef{Level: LevelSpan}}},
		},
		{
			name:        "a field with no level",
			expectedErr: "field reference has no level, and a built-in field belongs to one",
			filter:      eq(&FieldRef{Name: SpanFieldDuration}, &AnyValue{Value: "2s"}),
		},
		{
			name:        "a field at an unknown level",
			expectedErr: `unknown filter level "pod"`,
			filter:      eq(&FieldRef{Name: SpanFieldDuration, Level: "pod"}, &AnyValue{Value: "2s"}),
		},
		{
			name:        "membership of a constant rather than a list",
			expectedErr: `operator "in" takes a list as its second argument, got an untyped constant`,
			filter:      &Call{Op: OpIn, Args: []Expression{attr("a"), &AnyValue{Value: "1"}}},
		},
		{
			name:        "membership in an empty list",
			expectedErr: `operator "in" takes a list with at least one element`,
			filter:      &Call{Op: OpIn, Args: []Expression{attr("a"), &List{}}},
		},
		{
			name:        "membership of a constant subject",
			expectedErr: `operator "in" takes a reference, got a string constant`,
			filter:      &Call{Op: OpIn, Args: []Expression{&StringValue{Value: "a"}, &List{Values: []string{"1"}}}},
		},
		{
			name:        "list with an unknown type",
			expectedErr: `unknown filter value type "number"`,
			filter:      &Call{Op: OpNotIn, Args: []Expression{attr("a"), &List{Values: []string{"1"}, Type: "number"}}},
		},
		{
			name:        "list compared with equality",
			expectedErr: `operator "eq" compares a reference or a constant, got a list`,
			filter:      eq(attr("a"), &List{Values: []string{"1"}}),
		},
		{
			name:        "a call result as an operand",
			expectedErr: `operator "eq" compares a reference or a constant, got a predicate`,
			filter: eq(
				&Call{Op: OpExists, Args: []Expression{attr("a")}},
				&BoolValue{Value: true},
			),
		},
		{
			name:        "a call result as the subject of membership",
			expectedErr: `operator "in" takes a reference, got a predicate`,
			filter: &Call{Op: OpIn, Args: []Expression{
				&Call{Op: OpExists, Args: []Expression{attr("a")}},
				&List{Values: []string{"true"}, Type: ValueTypeBool},
			}},
		},
		{
			name:        "an ordered comparison of a word-valued field",
			expectedErr: `operator "gt" has no ordering for span.kind`,
			filter:      &Call{Op: OpGt, Args: []Expression{&FieldRef{Name: SpanFieldKind, Level: LevelSpan}, &AnyValue{Value: "server"}}},
		},
		{
			name:        "an ordered comparison of a boolean",
			expectedErr: `operator "gt" has no ordering for a boolean constant`,
			filter:      &Call{Op: OpGt, Args: []Expression{attr("ok"), &BoolValue{Value: true}}},
		},
		{
			name:        "a duration field against a number",
			expectedErr: `operator "gt" compares span.duration against an integer constant, which hold different kinds of value`,
			filter:      &Call{Op: OpGt, Args: []Expression{&FieldRef{Name: SpanFieldDuration, Level: LevelSpan}, &IntValue{Value: 42}}},
		},
		{
			name:        "an argument with no term",
			expectedErr: `operator "eq" compares a reference or a constant, got an empty term`,
			filter:      &Call{Op: OpEq, Args: []Expression{attr("a"), nil}},
		},
		{
			name:        "an ordered comparison against text",
			expectedErr: `operator "gt" compares span.duration against a string constant, which hold different kinds of value`,
			filter: &Call{Op: OpGt, Args: []Expression{
				&FieldRef{Name: SpanFieldDuration, Level: LevelSpan},
				&StringValue{Value: "2s"},
			}},
		},
		{
			name:        "an ordered comparison of constants of different kinds",
			expectedErr: `operator "gte" compares an integer constant against a boolean constant, which hold different kinds of value`,
			filter:      &Call{Op: OpGte, Args: []Expression{&IntValue{Value: 1}, &BoolValue{Value: true}}},
		},
		{
			name:        "an ordered comparison against a boolean",
			expectedErr: `operator "lte" has no ordering for a boolean constant`,
			filter:      &Call{Op: OpLte, Args: []Expression{attr("a"), &BoolValue{Value: true}}},
		},
		{
			name:        "two built-in fields holding different kinds of value",
			expectedErr: `operator "eq" compares span.duration against span.name, which hold different kinds of value`,
			filter: &Call{Op: OpEq, Args: []Expression{
				&FieldRef{Name: SpanFieldDuration, Level: LevelSpan},
				&FieldRef{Name: SpanFieldName, Level: LevelSpan},
			}},
		},
		{
			name:        "an instant ordered against a duration",
			expectedErr: `operator "gt" compares span.startTime against span.duration, which hold different kinds of value`,
			filter: &Call{Op: OpGt, Args: []Expression{
				&FieldRef{Name: SpanFieldStartTime, Level: LevelSpan},
				&FieldRef{Name: SpanFieldDuration, Level: LevelSpan},
			}},
		},
		{
			name:        "an integer against a duration field",
			expectedErr: `operator "eq" compares span.duration against an integer constant, which hold different kinds of value`,
			filter: &Call{Op: OpEq, Args: []Expression{
				&FieldRef{Name: SpanFieldDuration, Level: LevelSpan}, &IntValue{Value: 2},
			}},
		},
		{
			name:        "a timestamp against a duration field",
			expectedErr: `operator "gt" compares span.duration against a timestamp constant, which hold different kinds of value`,
			filter: &Call{Op: OpGt, Args: []Expression{
				&FieldRef{Name: SpanFieldDuration, Level: LevelSpan},
				&TimestampValue{Value: time.Unix(0, 0).UTC()},
			}},
		},
		{
			name:        "a duration against a text field",
			expectedErr: `operator "eq" compares span.name against a duration constant, which hold different kinds of value`,
			filter: &Call{Op: OpEq, Args: []Expression{
				&FieldRef{Name: SpanFieldName, Level: LevelSpan}, &DurationValue{Value: time.Second},
			}},
		},
		{
			name:        "a number against a field holding one of a set of words",
			expectedErr: `operator "eq" compares span.kind against an integer constant, which hold different kinds of value`,
			filter: &Call{Op: OpEq, Args: []Expression{
				&FieldRef{Name: SpanFieldKind, Level: LevelSpan}, &IntValue{Value: 1},
			}},
		},
		{
			name:        "a boolean against a text field",
			expectedErr: `operator "eq" compares span.name against a boolean constant, which hold different kinds of value`,
			filter: &Call{Op: OpEq, Args: []Expression{
				&FieldRef{Name: SpanFieldName, Level: LevelSpan}, &BoolValue{Value: true},
			}},
		},
		{
			name:        "a duration constant compared against an attribute",
			expectedErr: `operator "gt" compares a duration constant against an attribute, and the wire has no duration type`,
			filter: &Call{Op: OpGt, Args: []Expression{
				attr("latency"), &DurationValue{Value: 2 * time.Second},
			}},
		},
		{
			name:        "a timestamp constant compared against an attribute",
			expectedErr: `operator "eq" compares a timestamp constant against an attribute, and the wire has no timestamp type`,
			filter: &Call{Op: OpEq, Args: []Expression{
				attr("deadline"), &TimestampValue{Value: time.Unix(0, 0).UTC()},
			}},
		},
		{
			name:        "an anchored regular expression",
			expectedErr: `operator "regex" matches anywhere in the value, so a pattern cannot anchor itself`,
			filter: &Call{Op: OpRegex, Args: []Expression{
				&FieldRef{Name: SpanFieldName, Level: LevelSpan}, &StringValue{Value: "^GET"},
			}},
		},
		{
			name:        "a regular expression that will not compile",
			expectedErr: `operator "regex" takes a pattern in RE2 syntax`,
			filter: &Call{Op: OpRegex, Args: []Expression{
				&FieldRef{Name: SpanFieldName, Level: LevelSpan}, &StringValue{Value: "GET ("},
			}},
		},
		{
			name:        "a regular expression with a word boundary",
			expectedErr: `operator "regex" takes a pattern without word boundaries`,
			filter: &Call{Op: OpRegex, Args: []Expression{
				&FieldRef{Name: SpanFieldName, Level: LevelSpan}, &StringValue{Value: `\bGET`},
			}},
		},
		{
			name:        "a regular expression with a lazy quantifier",
			expectedErr: `operator "regex" asks whether the value matches, so a quantifier cannot be lazy`,
			filter: &Call{Op: OpRegex, Args: []Expression{
				&FieldRef{Name: SpanFieldName, Level: LevelSpan}, &StringValue{Value: "GET.*?/"},
			}},
		},
		{
			name:        "a regular expression asking to fold case",
			expectedErr: `operator "regex" matches case-sensitively, so a pattern cannot fold case`,
			filter: &Call{Op: OpRegex, Args: []Expression{
				&FieldRef{Name: SpanFieldName, Level: LevelSpan}, &StringValue{Value: "(?i)get"},
			}},
		},
		{
			name:        "a regular expression over a duration field",
			expectedErr: `operator "regex" matches text, and span.duration holds a duration`,
			filter: &Call{Op: OpRegex, Args: []Expression{
				&FieldRef{Name: SpanFieldDuration, Level: LevelSpan}, &StringValue{Value: "2s"},
			}},
		},
		{
			name:        "a regular expression over a timestamp field",
			expectedErr: `operator "regex" matches text, and span.startTime holds a timestamp`,
			filter: &Call{Op: OpRegex, Args: []Expression{
				&FieldRef{Name: SpanFieldStartTime, Level: LevelSpan}, &StringValue{Value: "2026-.*"},
			}},
		},
		{
			name:        "a regular expression over a numeric pattern",
			expectedErr: `operator "regex" takes a constant string as its pattern, got an integer constant`,
			filter:      &Call{Op: OpRegex, Args: []Expression{attr("a"), &IntValue{Value: 1}}},
		},
		{
			name:        "a regular expression matched against a reference",
			expectedErr: `operator "regex" takes a constant string as its pattern, got an attribute reference`,
			filter:      &Call{Op: OpRegex, Args: []Expression{attr("a"), attr("b")}},
		},
		{
			name:        "a regular expression of one argument",
			expectedErr: `operator "regex" takes 2 argument(s), got 1`,
			filter:      &Call{Op: OpRegex, Args: []Expression{attr("a")}},
		},
		{
			name:        "a regular expression over a constant",
			expectedErr: `operator "regex" takes a reference, got a string constant`,
			filter:      &Call{Op: OpRegex, Args: []Expression{&StringValue{Value: "a"}, &StringValue{Value: "b"}}},
		},
		{
			name:        "quantifier over a constant",
			expectedErr: `operator "some" takes a collection reference as its first argument, got an untyped constant`,
			filter: &Call{Op: OpSome, Args: []Expression{
				&AnyValue{Value: "event"},
				&Call{Op: OpExists, Args: []Expression{&FieldRef{Name: EventFieldName, Level: LevelEvent}}},
			}},
		},
		{
			name:        "quantifier over an attribute",
			expectedErr: `operator "some" takes a collection reference as its first argument, got an attribute reference`,
			filter: &Call{Op: OpSome, Args: []Expression{
				&AttributeRef{Key: "a", Level: LevelEvent},
				&Call{Op: OpExists, Args: []Expression{attr("a")}},
			}},
		},
		{
			name:        "quantifier over the span",
			expectedErr: `operator "some" quantifies over "event" or "link", got level "span"`,
			filter: &Call{Op: OpSome, Args: []Expression{
				&NestedRef{Level: LevelSpan},
				&Call{Op: OpExists, Args: []Expression{attr("a")}},
			}},
		},
		{
			name:        "quantifier over a level-less collection",
			expectedErr: `operator "some" quantifies over "event" or "link", got level ""`,
			filter: &Call{Op: OpSome, Args: []Expression{
				&NestedRef{},
				&Call{Op: OpExists, Args: []Expression{attr("a")}},
			}},
		},
		{
			name:        "quantifier over a constant predicate",
			expectedErr: `operator "some" takes a predicate as its second argument, got an untyped constant`,
			filter: &Call{Op: OpSome, Args: []Expression{
				&NestedRef{Level: LevelEvent},
				&AnyValue{Value: "true"},
			}},
		},
		{
			name:        "quantifier with an invalid predicate",
			expectedErr: `unknown filter operator "matches"`,
			filter: &Call{Op: OpSome, Args: []Expression{
				&NestedRef{Level: LevelLink},
				&Call{Op: "matches", Args: []Expression{attr("a")}},
			}},
		},
		{
			name:        "quantifier of one argument",
			expectedErr: `operator "some" takes 2 argument(s), got 1`,
			filter:      &Call{Op: OpSome, Args: []Expression{&NestedRef{Level: LevelEvent}}},
		},
		{
			name:        "existence of two references",
			expectedErr: `operator "exists" takes 1 argument(s), got 2`,
			filter:      &Call{Op: OpExists, Args: []Expression{attr("a"), attr("b")}},
		},
		{
			name:        "membership without a set",
			expectedErr: `operator "in" takes 2 argument(s), got 1`,
			filter:      &Call{Op: OpIn, Args: []Expression{attr("a")}},
		},
		{
			name:        "conjunction of a predicate and a list",
			expectedErr: `operator "and" takes predicates as arguments, got a list`,
			filter: &Call{Op: OpAnd, Args: []Expression{
				eq(attr("a"), &AnyValue{Value: "1"}),
				&List{Values: []string{"1"}},
			}},
		},
		{
			name:        "membership with an invalid left operand",
			expectedErr: `unknown filter level "pod"`,
			filter:      &Call{Op: OpIn, Args: []Expression{&AttributeRef{Key: "a", Level: "pod"}, &List{Values: []string{"1"}}}},
		},
		{
			name:        "invalid nested call as an operand",
			expectedErr: `operator "eq" compares a reference or a constant, got a predicate`,
			filter:      eq(&Call{Op: "matches", Args: []Expression{attr("a")}}, &AnyValue{Value: "1"}),
		},
		{
			name:        "invalid nested call as the subject of membership",
			expectedErr: `operator "in" takes a reference, got a predicate`,
			filter: &Call{Op: OpIn, Args: []Expression{
				&Call{Op: "matches", Args: []Expression{attr("a")}},
				&List{Values: []string{"1"}},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateFilter(test.filter)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.expectedErr)
		})
	}
}

// TestValidateFilter_RejectsANestedRefOutsideSome pins that the nested collection is readable
// only by the quantifier. Anywhere else it is many values where one is expected.
func TestValidateFilter_RejectsANestedRefOutsideSome(t *testing.T) {
	expected := `a collection reference is only the first argument of "some"`
	collection := &NestedRef{Level: LevelEvent}

	tests := map[string]*Call{
		"compared against a constant": eq(collection, &AnyValue{Value: "1"}),
		"under exists":                {Op: OpExists, Args: []Expression{collection}},
		"as the subject of membership": {Op: OpIn, Args: []Expression{
			collection, &List{Values: []string{"1"}},
		}},
		"as the predicate of a quantifier": {Op: OpSome, Args: []Expression{
			&NestedRef{Level: LevelLink},
			&Call{Op: OpExists, Args: []Expression{collection}},
		}},
	}
	for name, filter := range tests {
		t.Run(name, func(t *testing.T) {
			require.ErrorContains(t, ValidateFilter(filter), expected)
		})
	}
}

// TestValidateFilter_RejectsANestedSomeOverTheSameLevel pins RFC 0005 §5.5 rule 4. The inner
// quantifier would have to either shadow the outer element or reach back to it, and the version
// that answers that question is not this one.
func TestValidateFilter_RejectsANestedSomeOverTheSameLevel(t *testing.T) {
	inner := func(level Level) *Call {
		return &Call{Op: OpSome, Args: []Expression{
			&NestedRef{Level: level},
			&Call{Op: OpExists, Args: []Expression{&FieldRef{Name: EventFieldName, Level: LevelEvent}}},
		}}
	}
	directly := &Call{Op: OpSome, Args: []Expression{
		&NestedRef{Level: LevelEvent},
		inner(LevelEvent),
	}}
	require.ErrorContains(t, ValidateFilter(directly),
		`operator "some" is already quantifying over "event", and this version does not define what a nested one would bind`)

	// However deep the inner one sits, and whichever level is doubled up.
	deeper := &Call{Op: OpSome, Args: []Expression{
		&NestedRef{Level: LevelLink},
		&Call{Op: OpAnd, Args: []Expression{
			eq(&FieldRef{Name: LinkFieldTraceID, Level: LevelLink}, &StringValue{Value: "abc"}),
			&Call{Op: OpNot, Args: []Expression{inner(LevelLink)}},
		}},
	}}
	require.ErrorContains(t, ValidateFilter(deeper), `already quantifying over "link"`)
}

// TestValidateFilter_RejectsUnknownField pins that naming a field this API does not define is
// refused, and that the message says how to ask for an attribute of that name instead.
func TestValidateFilter_RejectsUnknownField(t *testing.T) {
	err := ValidateFilter(eq(&FieldRef{Level: LevelSpan, Name: "durtion"}, &AnyValue{Value: "2s"}))
	require.ErrorContains(t, err, `unknown built-in field "durtion" at the "span" level`)
	require.ErrorContains(t, err, "name an attribute to match a tag of that name instead")

	// The same name as an attribute is fine, because an attribute key is arbitrary.
	require.NoError(t, ValidateFilter(
		eq(&AttributeRef{Level: LevelSpan, Key: "durtion"}, &AnyValue{Value: "2s"})))

	// A field of the wrong level is refused too.
	require.ErrorContains(t,
		ValidateFilter(eq(&FieldRef{Level: LevelResource, Name: SpanFieldDuration}, &AnyValue{Value: "2s"})),
		`unknown built-in field "duration" at the "resource" level`)
}

// The tests below are the systematic half. The tables above enumerate cases someone thought of;
// these walk the vocabulary itself, so a constant added without a matching case in the
// validator fails here rather than passing unnoticed until a caller sends it.

// TestDomainOfAnUndeclaredType pins what an unnamed type says about a value: nothing. Validation
// refuses an undefined field and an unknown value type before either domain is asked about, so
// this is the one place the zero value of each is exercised.
func TestDomainOfAnUndeclaredType(t *testing.T) {
	assert.Equal(t, domainUnknown, domainOfFieldType(""))
	assert.Equal(t, domainUnknown, domainOfValueType(""))
}

// TestValidateFilter_HandlesEveryOperator pins that each declared operator has a case in
// validateCall. Without this, adding an operator constant and forgetting the case would report
// it to callers as unknown — the one answer that is certainly wrong, since the API defines it.
func TestValidateFilter_HandlesEveryOperator(t *testing.T) {
	for _, op := range operators {
		t.Run(string(op), func(t *testing.T) {
			// Deliberately the wrong arguments: what matters is only that the operator is
			// recognised, so any complaint except "unknown" means it has a case.
			err := ValidateFilter(&Call{Op: op})
			if err != nil {
				assert.NotContains(t, err.Error(), "unknown filter operator",
					"operator %q is declared but validateCall has no case for it", op)
			}
		})
	}
}

// TestValidateFilter_AcceptsEveryLevel pins that an attribute may name any declared level.
func TestValidateFilter_AcceptsEveryLevel(t *testing.T) {
	for _, level := range levels {
		t.Run(string(level), func(t *testing.T) {
			ref := &AttributeRef{Level: level, Key: "a"}
			require.NoError(t, ValidateFilter(eq(ref, &AnyValue{Value: "1"})))
		})
	}
	require.NoError(t, ValidateFilter(eq(attr("a"), &AnyValue{Value: "1"})),
		"an empty level is the unqualified attribute and is always allowed")
}

// TestValidateFilter_AcceptsEveryValueType pins that a list may declare any of the defined types.
// Declaring none is accepted only where the field opposite the list declares one instead, which is
// what the two subjects here distinguish.
func TestValidateFilter_AcceptsEveryValueType(t *testing.T) {
	for _, vt := range valueTypes {
		t.Run(string(vt), func(t *testing.T) {
			in := &Call{Op: OpIn, Args: []Expression{
				attr("a"),
				&List{Values: []string{"1"}, Type: vt},
			}}
			require.NoError(t, ValidateFilter(in))
		})
	}

	t.Run("no type, against a built-in field", func(t *testing.T) {
		in := &Call{Op: OpIn, Args: []Expression{
			spanField(SpanFieldDuration),
			&List{Values: []string{"2s"}},
		}}
		require.NoError(t, ValidateFilter(in))
	})

	t.Run("no type, against an attribute", func(t *testing.T) {
		in := &Call{Op: OpIn, Args: []Expression{
			attr("a"),
			&List{Values: []string{"1"}},
		}}
		require.ErrorContains(t, ValidateFilter(in),
			`operator "in" takes a list that declares its element type when it is compared against an attribute`)
	})
}

// TestValidateFilter_AcceptsEveryConstant pins that every constant node is comparable against
// something, since each one is what a wire hint or a resolution can produce. A duration and an
// instant are comparable only against a built-in field that declares that type (§5.4), which is why
// the reference this compares against depends on the constant.
func TestValidateFilter_AcceptsEveryConstant(t *testing.T) {
	for _, test := range allTerms {
		if !isConstant(test.term) {
			continue
		}
		t.Run(test.name, func(t *testing.T) {
			var reference Expression = attr("a")
			switch test.term.(type) {
			case *DurationValue:
				reference = spanField(SpanFieldDuration)
			case *TimestampValue:
				reference = spanField(SpanFieldStartTime)
			}
			require.NoError(t, ValidateFilter(eq(reference, test.term)))
		})
	}
}

// TestValidateFilter_AcceptsEveryField pins that the field enumeration and the validator agree:
// every field Fields() offers is one a query may actually name at that level.
func TestValidateFilter_AcceptsEveryField(t *testing.T) {
	for _, f := range Fields() {
		t.Run(string(f.Level)+"."+f.Name, func(t *testing.T) {
			ref := &FieldRef{Level: f.Level, Name: f.Name}
			require.NoError(t, ValidateFilter(eq(ref, &AnyValue{Value: "x"})))
		})
	}
}

// TestValidateFilter_CatchesAnInvalidNodeAtAnyDepth pins that validation recurses down every
// path a node can hide in, not just the root. Each case buries the same bad reference — an
// unknown level — somewhere different.
func TestValidateFilter_CatchesAnInvalidNodeAtAnyDepth(t *testing.T) {
	bad := &AttributeRef{Level: "nonesuch", Key: "a"}
	good := eq(attr("ok"), &AnyValue{Value: "1"})

	tests := map[string]*Call{
		"under and":               {Op: OpAnd, Args: []Expression{good, eq(bad, &AnyValue{Value: "1"})}},
		"under or":                {Op: OpOr, Args: []Expression{good, eq(bad, &AnyValue{Value: "1"})}},
		"under not":               {Op: OpNot, Args: []Expression{eq(bad, &AnyValue{Value: "1"})}},
		"as a comparison operand": eq(bad, &AnyValue{Value: "1"}),
		"as the right operand":    eq(attr("ok"), bad),
		"as the subject of in":    {Op: OpIn, Args: []Expression{bad, &List{Values: []string{"1"}}}},
		"as the subject of regex": {Op: OpRegex, Args: []Expression{bad, &StringValue{Value: "1"}}},
		"under exists":            {Op: OpExists, Args: []Expression{bad}},
		"inside a some predicate": {Op: OpSome, Args: []Expression{&NestedRef{Level: LevelEvent}, eq(bad, &AnyValue{Value: "1"})}},
		"two conjunctions deep":   {Op: OpAnd, Args: []Expression{good, &Call{Op: OpAnd, Args: []Expression{good, eq(bad, &AnyValue{Value: "1"})}}}},
	}
	for name, filter := range tests {
		t.Run(name, func(t *testing.T) {
			require.ErrorContains(t, ValidateFilter(filter), `unknown filter level "nonesuch"`)
		})
	}
}

// TestValidateFilter_NeverPanics pins that validation answers for any tree at all, however
// malformed. It reaches trees a caller should not be able to build but a decoder can produce —
// a nil argument, a nil interface, a term where a predicate belongs — and asserts only that it
// returns. A validator that panicked would take the process down on a hostile request.
func TestValidateFilter_NeverPanics(t *testing.T) {
	deep := eq(attr("a"), &AnyValue{Value: "1"})
	for range 64 {
		deep = &Call{Op: OpNot, Args: []Expression{deep}}
	}

	trees := map[string]*Call{
		"no operator":                      {},
		"no arguments":                     {Op: OpEq},
		"a nil argument":                   {Op: OpEq, Args: []Expression{nil, nil}},
		"a nil attribute reference":        {Op: OpEq, Args: []Expression{(*AttributeRef)(nil), &AnyValue{Value: "1"}}},
		"a nil field reference":            {Op: OpEq, Args: []Expression{(*FieldRef)(nil), &AnyValue{Value: "1"}}},
		"a nil collection reference":       {Op: OpSome, Args: []Expression{(*NestedRef)(nil), &Call{Op: OpExists}}},
		"a nil constant":                   {Op: OpEq, Args: []Expression{attr("a"), (*AnyValue)(nil)}},
		"a nil list":                       {Op: OpIn, Args: []Expression{attr("a"), (*List)(nil)}},
		"a nil nested call":                {Op: OpAnd, Args: []Expression{(*Call)(nil), (*Call)(nil)}},
		"a list where a predicate belongs": {Op: OpAnd, Args: []Expression{&List{}, &List{}}},
		"a call as its own argument":       {Op: OpNot, Args: []Expression{&Call{Op: OpNot, Args: nil}}},
		"some over nothing":                {Op: OpSome, Args: []Expression{nil, nil}},
		"deeply nested":                    deep,
	}
	for name, filter := range trees {
		t.Run(name, func(t *testing.T) {
			assert.NotPanics(t, func() { _ = ValidateFilter(filter) })
		})
	}
	assert.NotPanics(t, func() { _ = ValidateFilter(nil) })
}
