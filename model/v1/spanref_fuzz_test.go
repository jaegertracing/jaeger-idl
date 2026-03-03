// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model_test

import (
	"bytes"
	"sort"
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger-idl/model/v1/prototest"
)

func FuzzSpanRef(f *testing.F) {
	var enumValues []model.SpanRefType
	for k := range model.SpanRefType_name {
		enumValues = append(enumValues, model.SpanRefType(k))
	}

	sort.Slice(enumValues, func(i, j int) bool {
		return enumValues[i] < enumValues[j]
	})

	var marshaler = &jsonpb.Marshaler{}

	// Add seed inputs to cover normal, zero and max boundary values.
	f.Add(uint64(2), uint64(3), uint64(11), uint8(0))
	f.Add(uint64(0), uint64(0), uint64(0), uint8(1))
	f.Add(^uint64(0), ^uint64(0), ^uint64(0), uint8(0))

	f.Fuzz(func(t *testing.T, high, low, span uint64, refType uint8) {
		rt := enumValues[int(refType)%len(enumValues)]

		traceID := model.NewTraceID(high, low)
		spanID := model.NewSpanID(span)

		// Construct SpanRef using custom model types.
		ref1 := model.SpanRef{
			TraceID: traceID,
			SpanID:  spanID,
			RefType: rt,
		}

		// Convert TraceID and SpanID into raw bytes.
		traceBytes := make([]byte, traceID.Size())
		spanBytes := make([]byte, spanID.Size())

		n1, err := traceID.MarshalTo(traceBytes)
		if err != nil {
			t.Fatalf("traceID MarshalTo failed: %v", err)
		}

		if n1 != len(traceBytes) {
			t.Fatalf("traceID MarshalTo wrote %d bytes, expected %d", n1, len(traceBytes))
		}
		n2, err := spanID.MarshalTo(spanBytes)
		if err != nil {
			t.Fatalf("spanID MarshalTo failed: %v", err)
		}

		if n2 != len(spanBytes) {
			t.Fatalf("spanID MarshalTo wrote %d bytes, expected %d", n2, len(spanBytes))
		}

		ref2 := prototest.SpanRef{
			TraceId: traceBytes,
			SpanId:  spanBytes,
			RefType: prototest.SpanRefType(rt),
		}

		// Convert both SpanRefs into proto binary formats before
		// comparing to match with the standard protobuf encoding.
		d1, err := proto.Marshal(&ref1)
		if err != nil {
			t.Fatalf("marshal ref1 failed: %v", err)
		}

		d2, err := proto.Marshal(&ref2)
		if err != nil {
			t.Fatalf("marshal ref2 failed: %v", err)
		}

		if !bytes.Equal(d1, d2) {
			t.Fatalf("protobuf encoding mismatch between model.SpanRef and prototest.SpanRef")
		}

		var ref1u model.SpanRef
		if err := proto.Unmarshal(d2, &ref1u); err != nil {
			t.Fatalf("protobuf unmarshal failed: %v", err)
		}

		// Verify output of protobuf roundtrip to ensure there are no changes in the data.
		if !proto.Equal(&ref1, &ref1u) {
			t.Fatalf("protobuf roundtrip mismatched")
		}

		var out1, out2 bytes.Buffer
		if err := marshaler.Marshal(&out1, &ref1); err != nil {
			t.Fatalf("json marshal ref1 failed: %v", err)
		}

		if err := marshaler.Marshal(&out2, &ref2); err != nil {
			t.Fatalf("json marshal ref1 failed: %v", err)
		}

		var j1, j2 model.SpanRef
		if err := jsonpb.Unmarshal(bytes.NewReader(out1.Bytes()), &j1); err != nil {
			t.Fatalf("json unmarshal j1 failed: %v", err)
		}

		if err := jsonpb.Unmarshal(bytes.NewReader(out2.Bytes()), &j2); err != nil {
			t.Fatalf("json unmarshal j2 failed: %v", err)
		}

		if !proto.Equal(&j1, &j2) {
			t.Fatalf("json encoding mismatch between model and prototest")
		}

		if !proto.Equal(&ref1, &j1) {
			t.Fatalf("json roundtrip mismatched original ref1")
		}
	})
}
