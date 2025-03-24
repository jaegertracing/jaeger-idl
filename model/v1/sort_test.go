// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var currTime = time.Now()

func TestSortTraces(t *testing.T) {
	t1 := &Trace{
		Spans: []*Span{
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(2),
				Tags:    []KeyValue{{Key: "world"}, {Key: "hello"}},
				Process: &Process{
					ServiceName: "hello",
					Tags:        []KeyValue{{Key: "hello"}, {Key: "world"}},
				},
			},
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(1),
				Logs: []Log{
					{
						Timestamp: currTime,
						Fields:    []KeyValue{{Key: "world"}, {Key: "hello"}},
					},
					{
						Timestamp: currTime.Add(-time.Hour),
						Fields:    []KeyValue{{Key: "hello"}, {Key: "world"}},
					},
				},
			},
		},
	}
	t2 := &Trace{
		Spans: []*Span{
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(2),
				Tags:    []KeyValue{{Key: "world"}, {Key: "hello"}},
				Process: &Process{
					ServiceName: "hello",
					Tags:        []KeyValue{{Key: "hello"}, {Key: "world"}},
				},
			},
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(1),
				Logs: []Log{
					{
						Timestamp: currTime.Add(-time.Hour),
						Fields:    []KeyValue{{Key: "world"}, {Key: "hello"}},
					},
					{
						Timestamp: currTime,
						Fields:    []KeyValue{{Key: "hello"}, {Key: "world"}},
					},
				},
			},
		},
	}
	SortTrace(t1)
	SortTrace(t2)
	assert.Equal(t, t1, t2)
}

func TestSortListOfTraces(t *testing.T) {
	t1 := &Trace{
		Spans: []*Span{
			{
				TraceID: TraceID{Low: 1},
			},
			{
				TraceID: TraceID{Low: 1},
			},
		},
	}
	t2 := &Trace{
		Spans: []*Span{
			{
				TraceID: TraceID{Low: 2},
			},
		},
	}
	t3 := &Trace{
		Spans: []*Span{
			{
				TraceID: TraceID{Low: 3},
			},
		},
	}
	t4 := &Trace{}

	list1 := []*Trace{t1, t4, t2, t3}
	list2 := []*Trace{t4, t2, t1, t3}
	SortTraces(list1)
	SortTraces(list2)
	assert.Equal(t, list1, list2)
}

func TestSortByTraceID(t *testing.T) {
	traceID := &TraceID{
		High: uint64(1),
		Low:  uint64(1),
	}
	traceID2 := &TraceID{
		High: uint64(2),
		Low:  uint64(0),
	}
	traceID3 := &TraceID{
		High: uint64(1),
		Low:  uint64(0),
	}

	traces := []*TraceID{traceID, traceID2, traceID3}
	// Expect ascending order
	tracesExpected := []*TraceID{traceID3, traceID, traceID2}
	SortTraceIDs(traces)
	assert.Equal(t, tracesExpected, traces)
}
