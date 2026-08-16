// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func eq(left, right Expression) *Call {
	return &Call{Op: OpEq, Args: []Expression{left, right}}
}

func TestValidateFilter_Accepts(t *testing.T) {
	tests := []struct {
		name   string
		filter *Call
	}{
		{
			name:   "unqualified attribute equality",
			filter: eq(&Reference{Name: "http.status_code"}, &Scalar{Value: "500"}),
		},
		{
			name:   "level-qualified attribute",
			filter: eq(&Reference{Name: "k8s.pod.name", Level: LevelResource, Attr: true}, &Scalar{Value: "cart-0"}),
		},
		{
			name:   "built-in field against a typed constant",
			filter: &Call{Op: OpGt, Args: []Expression{&Reference{Name: "duration", Level: LevelSpan}, &Scalar{Value: "2s"}}},
		},
		{
			name:   "reference against reference",
			filter: eq(&Reference{Name: "enduser.id", Level: LevelSpan, Attr: true}, &Reference{Name: "enduser.id", Level: LevelResource, Attr: true}),
		},
		{
			name: "conjunction of two predicates",
			filter: &Call{Op: OpAnd, Args: []Expression{
				eq(&Reference{Name: "a"}, &Scalar{Value: "1"}),
				eq(&Reference{Name: "b"}, &Scalar{Value: "2"}),
			}},
		},
		{
			name: "nested disjunction under a negation",
			filter: &Call{Op: OpNot, Args: []Expression{
				&Call{Op: OpOr, Args: []Expression{
					eq(&Reference{Name: "a"}, &Scalar{Value: "1"}),
					eq(&Reference{Name: "b"}, &Scalar{Value: "2"}),
				}},
			}},
		},
		{
			name: "set membership",
			filter: &Call{Op: OpIn, Args: []Expression{
				&Reference{Name: "http.status_code"},
				&List{Values: []string{"500", "503"}, Type: ValueTypeInt},
			}},
		},
		{
			name:   "existence of an attribute",
			filter: &Call{Op: OpExists, Args: []Expression{&Reference{Name: "error"}}},
		},
		{
			name: "correlated match over the event collection",
			filter: &Call{Op: OpSome, Args: []Expression{
				&Reference{Level: LevelEvent},
				&Call{Op: OpAnd, Args: []Expression{
					eq(&Reference{Name: "name", Level: LevelEvent}, &Scalar{Value: "exception"}),
					&Call{Op: OpGt, Args: []Expression{
						&Reference{Name: "timeSinceStart", Level: LevelEvent},
						&Scalar{Value: "50us"},
					}},
				}},
			}},
		},
		{
			name: "a call result as an operand",
			filter: eq(
				&Call{Op: OpExists, Args: []Expression{&Reference{Name: "a"}}},
				&Scalar{Value: "true", Type: ValueTypeBool},
			),
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
			filter:      &Call{Op: "matches", Args: []Expression{&Reference{Name: "a"}, &Scalar{Value: "b"}}},
		},
		{
			name:        "conjunction of one",
			expectedErr: `operator "and" takes at least two arguments, got 1`,
			filter:      &Call{Op: OpAnd, Args: []Expression{eq(&Reference{Name: "a"}, &Scalar{Value: "1"})}},
		},
		{
			name:        "conjunction over a bare reference",
			expectedErr: `operator "and" takes predicates as arguments, got a reference`,
			filter: &Call{Op: OpAnd, Args: []Expression{
				&Reference{Name: "a"},
				eq(&Reference{Name: "b"}, &Scalar{Value: "1"}),
			}},
		},
		{
			name:        "invalid predicate nested in a conjunction",
			expectedErr: `unknown filter level "pod"`,
			filter: &Call{Op: OpAnd, Args: []Expression{
				eq(&Reference{Name: "a"}, &Scalar{Value: "1"}),
				eq(&Reference{Name: "b", Level: "pod"}, &Scalar{Value: "2"}),
			}},
		},
		{
			name:        "negation of two predicates",
			expectedErr: `operator "not" takes 1 argument(s), got 2`,
			filter: &Call{Op: OpNot, Args: []Expression{
				eq(&Reference{Name: "a"}, &Scalar{Value: "1"}),
				eq(&Reference{Name: "b"}, &Scalar{Value: "2"}),
			}},
		},
		{
			name:        "equality of one operand",
			expectedErr: `operator "eq" takes 2 argument(s), got 1`,
			filter:      &Call{Op: OpEq, Args: []Expression{&Reference{Name: "a"}}},
		},
		{
			name:        "existence of a constant",
			expectedErr: `operator "exists" takes a reference, got a constant`,
			filter:      &Call{Op: OpExists, Args: []Expression{&Scalar{Value: "a"}}},
		},
		{
			name:        "existence of an unnamed reference",
			expectedErr: "filter reference has no name",
			filter:      &Call{Op: OpExists, Args: []Expression{&Reference{Level: LevelSpan}}},
		},
		{
			name:        "membership of a constant rather than a list",
			expectedErr: `operator "in" takes a list as its second argument, got a constant`,
			filter:      &Call{Op: OpIn, Args: []Expression{&Reference{Name: "a"}, &Scalar{Value: "1"}}},
		},
		{
			name:        "list with an unknown type",
			expectedErr: `unknown filter value type "number"`,
			filter:      &Call{Op: OpNotIn, Args: []Expression{&Reference{Name: "a"}, &List{Values: []string{"1"}, Type: "number"}}},
		},
		{
			name:        "constant with an unknown type",
			expectedErr: `unknown filter value type "timestamp"`,
			filter:      eq(&Reference{Name: "a"}, &Scalar{Value: "1", Type: "timestamp"}),
		},
		{
			name:        "list compared with equality",
			expectedErr: "a list cannot be compared",
			filter:      eq(&Reference{Name: "a"}, &List{Values: []string{"1"}}),
		},
		{
			name:        "an argument with no term",
			expectedErr: "an empty term cannot be compared",
			filter:      &Call{Op: OpEq, Args: []Expression{&Reference{Name: "a"}, nil}},
		},
		{
			name:        "quantifier over a constant",
			expectedErr: `operator "some" takes a collection reference as its first argument, got a constant`,
			filter: &Call{Op: OpSome, Args: []Expression{
				&Scalar{Value: "event"},
				&Call{Op: OpExists, Args: []Expression{&Reference{Name: "name", Level: LevelEvent}}},
			}},
		},
		{
			name:        "quantifier over the span",
			expectedErr: `operator "some" quantifies over "event" or "link", got level "span"`,
			filter: &Call{Op: OpSome, Args: []Expression{
				&Reference{Level: LevelSpan},
				&Call{Op: OpExists, Args: []Expression{&Reference{Name: "a"}}},
			}},
		},
		{
			name:        "quantifier over a named event field",
			expectedErr: `operator "some" takes the whole collection, so its first argument must not name "name"`,
			filter: &Call{Op: OpSome, Args: []Expression{
				&Reference{Name: "name", Level: LevelEvent},
				&Call{Op: OpExists, Args: []Expression{&Reference{Name: "a"}}},
			}},
		},
		{
			name:        "quantifier over a constant predicate",
			expectedErr: `operator "some" takes a predicate as its second argument, got a constant`,
			filter: &Call{Op: OpSome, Args: []Expression{
				&Reference{Level: LevelEvent},
				&Scalar{Value: "true"},
			}},
		},
		{
			name:        "quantifier with an invalid predicate",
			expectedErr: `unknown filter operator "matches"`,
			filter: &Call{Op: OpSome, Args: []Expression{
				&Reference{Level: LevelLink},
				&Call{Op: "matches", Args: []Expression{&Reference{Name: "a"}}},
			}},
		},
		{
			name:        "quantifier of one argument",
			expectedErr: `operator "some" takes 2 argument(s), got 1`,
			filter:      &Call{Op: OpSome, Args: []Expression{&Reference{Level: LevelEvent}}},
		},
		{
			name:        "existence of two references",
			expectedErr: `operator "exists" takes 1 argument(s), got 2`,
			filter:      &Call{Op: OpExists, Args: []Expression{&Reference{Name: "a"}, &Reference{Name: "b"}}},
		},
		{
			name:        "membership without a set",
			expectedErr: `operator "in" takes 2 argument(s), got 1`,
			filter:      &Call{Op: OpIn, Args: []Expression{&Reference{Name: "a"}}},
		},
		{
			name:        "conjunction of a predicate and a list",
			expectedErr: `operator "and" takes predicates as arguments, got a list`,
			filter: &Call{Op: OpAnd, Args: []Expression{
				eq(&Reference{Name: "a"}, &Scalar{Value: "1"}),
				&List{Values: []string{"1"}},
			}},
		},
		{
			name:        "membership with an invalid left operand",
			expectedErr: `unknown filter level "pod"`,
			filter:      &Call{Op: OpIn, Args: []Expression{&Reference{Name: "a", Level: "pod"}, &List{Values: []string{"1"}}}},
		},
		{
			name:        "invalid nested call as an operand",
			expectedErr: `unknown filter operator "matches"`,
			filter:      eq(&Call{Op: "matches", Args: []Expression{&Reference{Name: "a"}}}, &Scalar{Value: "1"}),
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
		ValidateFilter(eq(&Reference{Level: LevelResource, Name: SpanFieldDuration}, &Scalar{Value: "2s"})),
		`unknown built-in field "duration" at the "resource" level`)
}

// The tests below are the systematic half. The tables above enumerate cases someone thought of;
// these walk the vocabulary itself, so a constant added without a matching case in the
// validator fails here rather than passing unnoticed until a caller sends it.

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

// TestValidateFilter_AcceptsEveryLevel pins that a reference may name any declared level.
func TestValidateFilter_AcceptsEveryLevel(t *testing.T) {
	for _, level := range levels {
		t.Run(string(level), func(t *testing.T) {
			// An attribute, so the field vocabulary is not what is under test here.
			ref := &Reference{Level: level, Name: "a", Attr: true}
			require.NoError(t, ValidateFilter(eq(ref, &Scalar{Value: "1"})))
		})
	}
	require.NoError(t, ValidateFilter(eq(&Reference{Name: "a"}, &Scalar{Value: "1"})),
		"an empty level is the unqualified attribute and is always allowed")
}

// TestValidateFilter_AcceptsEveryValueType pins that a constant may declare any declared type,
// and that declaring none is allowed because it means "any type" rather than a type.
func TestValidateFilter_AcceptsEveryValueType(t *testing.T) {
	for _, vt := range append([]ValueType{""}, valueTypes...) {
		t.Run("scalar/"+string(vt), func(t *testing.T) {
			require.NoError(t, ValidateFilter(eq(&Reference{Name: "a"}, &Scalar{Value: "1", Type: vt})))
		})
		t.Run("list/"+string(vt), func(t *testing.T) {
			in := &Call{Op: OpIn, Args: []Expression{
				&Reference{Name: "a"},
				&List{Values: []string{"1"}, Type: vt},
			}}
			require.NoError(t, ValidateFilter(in))
		})
	}
}

// TestValidateFilter_AcceptsEveryField pins that the field enumeration and the validator agree:
// every field Fields() offers is one a query may actually name at that level.
func TestValidateFilter_AcceptsEveryField(t *testing.T) {
	for _, f := range Fields() {
		t.Run(string(f.Level)+"."+f.Name, func(t *testing.T) {
			ref := &Reference{Level: f.Level, Name: f.Name}
			require.NoError(t, ValidateFilter(eq(ref, &Scalar{Value: "x"})))
		})
	}
}

// TestValidateFilter_CatchesAnInvalidNodeAtAnyDepth pins that validation recurses down every
// path a node can hide in, not just the root. Each case buries the same bad reference — an
// unknown level — somewhere different.
func TestValidateFilter_CatchesAnInvalidNodeAtAnyDepth(t *testing.T) {
	bad := &Reference{Level: "nonesuch", Name: "a", Attr: true}
	good := eq(&Reference{Name: "ok"}, &Scalar{Value: "1"})

	tests := map[string]*Call{
		"under and":               {Op: OpAnd, Args: []Expression{good, eq(bad, &Scalar{Value: "1"})}},
		"under or":                {Op: OpOr, Args: []Expression{good, eq(bad, &Scalar{Value: "1"})}},
		"under not":               {Op: OpNot, Args: []Expression{eq(bad, &Scalar{Value: "1"})}},
		"as a comparison operand": eq(bad, &Scalar{Value: "1"}),
		"as the right operand":    eq(&Reference{Name: "ok"}, bad),
		"as the subject of in":    {Op: OpIn, Args: []Expression{bad, &List{Values: []string{"1"}}}},
		"under exists":            {Op: OpExists, Args: []Expression{bad}},
		"inside a nested call":    eq(&Call{Op: OpExists, Args: []Expression{bad}}, &Scalar{Value: "1"}),
		"inside a some predicate": {Op: OpSome, Args: []Expression{&Reference{Level: LevelEvent}, eq(bad, &Scalar{Value: "1"})}},
		"two conjunctions deep":   {Op: OpAnd, Args: []Expression{good, &Call{Op: OpAnd, Args: []Expression{good, eq(bad, &Scalar{Value: "1"})}}}},
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
	deep := &Call{Op: OpEq, Args: []Expression{&Reference{Name: "a"}, &Scalar{Value: "1"}}}
	for range 64 {
		deep = &Call{Op: OpNot, Args: []Expression{deep}}
	}

	trees := map[string]*Call{
		"no operator":                      {},
		"no arguments":                     {Op: OpEq},
		"a nil argument":                   {Op: OpEq, Args: []Expression{nil, nil}},
		"a nil reference":                  {Op: OpEq, Args: []Expression{(*Reference)(nil), &Scalar{Value: "1"}}},
		"a nil constant":                   {Op: OpEq, Args: []Expression{&Reference{Name: "a"}, (*Scalar)(nil)}},
		"a nil list":                       {Op: OpIn, Args: []Expression{&Reference{Name: "a"}, (*List)(nil)}},
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
