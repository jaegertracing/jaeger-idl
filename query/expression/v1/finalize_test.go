// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFinalize(t *testing.T) {
	filter := &Call{Op: OpAnd, Args: []Expression{
		&Call{Op: OpGt, Args: []Expression{
			&AnyValue{Value: "2s"}, &FieldRef{Name: SpanFieldDuration, Level: LevelSpan},
		}},
		&Call{Op: OpEq, Args: []Expression{
			&AttributeRef{Key: "http.method"}, &AnyValue{Value: "GET"},
		}},
	}}

	finalized, err := Finalize(filter)
	require.NoError(t, err)
	assert.Equal(t, &Call{Op: OpAnd, Args: []Expression{
		&Call{Op: OpLt, Args: []Expression{
			&FieldRef{Name: SpanFieldDuration, Level: LevelSpan},
			&DurationValue{Value: 2 * time.Second},
		}},
		&Call{Op: OpEq, Args: []Expression{
			&AttributeRef{Key: "http.method"}, &AnyValue{Value: "GET"},
		}},
	}}, finalized, "the constant is read, and the reference comes first")
}

// TestFinalize_IsIdempotent is what lets every boundary finalize a filter it did not build: the
// query service after an interceptor has edited one, and the remote-storage server on a tree a
// client may already have finalized.
func TestFinalize_IsIdempotent(t *testing.T) {
	filters := map[string]*Call{
		"an ordered comparison against a duration field": {Op: OpGt, Args: []Expression{
			&AnyValue{Value: "2s"}, &FieldRef{Name: SpanFieldDuration, Level: LevelSpan},
		}},
		"an equality against a word-valued field": {Op: OpEq, Args: []Expression{
			&FieldRef{Name: SpanFieldKind, Level: LevelSpan}, &AnyValue{Value: "server"},
		}},
		"a timestamp bound": {Op: OpLte, Args: []Expression{
			&FieldRef{Name: SpanFieldStartTime, Level: LevelSpan},
			&AnyValue{Value: "2026-08-18T00:00:00Z"},
		}},
		"membership": {Op: OpIn, Args: []Expression{
			&FieldRef{Name: SpanFieldStatus, Level: LevelSpan},
			&List{Values: []string{"error"}, Type: ValueTypeString},
		}},
		"a quantified predicate": {Op: OpSome, Args: []Expression{
			&NestedRef{Level: LevelEvent},
			&Call{Op: OpEq, Args: []Expression{
				&FieldRef{Name: EventFieldName, Level: LevelEvent}, &AnyValue{Value: "exception"},
			}},
		}},
	}
	for name, filter := range filters {
		t.Run(name, func(t *testing.T) {
			once, err := Finalize(filter)
			require.NoError(t, err)
			twice, err := Finalize(once)
			require.NoError(t, err)
			assert.Equal(t, once, twice)
		})
	}
}

// TestFinalize_RefusesADepthNoConsumerCouldWalk covers the bound at the entry point every boundary
// calls, since that is where a tree arriving off a wire is stopped.
func TestFinalize_RefusesADepthNoConsumerCouldWalk(t *testing.T) {
	deep := eq(&AttributeRef{Key: "a"}, &AnyValue{Value: "1"})
	for range MaxNestingDepth {
		deep = &Call{Op: OpNot, Args: []Expression{deep}}
	}
	_, err := Finalize(deep)
	require.ErrorIs(t, err, ErrTooDeeplyNested)
}

func TestFinalize_RefusesWhatValidationRefuses(t *testing.T) {
	_, err := Finalize(&Call{Op: "matches", Args: []Expression{
		&AttributeRef{Key: "a"}, &AnyValue{Value: "b"},
	}})
	require.ErrorContains(t, err, `unknown filter operator "matches"`)

	_, err = Finalize(&Call{Op: OpGt, Args: []Expression{
		&FieldRef{Name: SpanFieldDuration, Level: LevelSpan}, &AnyValue{Value: "banana"},
	}})
	require.ErrorContains(t, err, `cannot compare span.duration against "banana"`)
}
