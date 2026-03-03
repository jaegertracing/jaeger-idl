package model_test

import (
	"bytes"
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger-idl/model/v1/prototest"
)

func FuzzSpanRef(f *testing.F) {
	//Add seed inputs to cover normal, zero and max boundary values.
	f.Add(uint64(2), uint64(3), uint64(11))
	f.Add(uint64(0), uint64(0), uint64(0))
	f.Add(^uint64(0), ^uint64(0), ^uint64(0))

	f.Fuzz(func(t *testing.T, high, low, span uint64) {
		//Construct SpanRef using custom model types.
		ref1 := model.SpanRef{
			TraceID: model.NewTraceID(high, low),
			SpanID:  model.NewSpanID(span),
		}

		//Convert traceID and spanID into raw byte format before
		//constructing SpanRefs.
		traceID := model.NewTraceID(high, low)
		spanID := model.NewSpanID(span)

		traceBytes := make([]byte, traceID.Size())
		spanBytes := make([]byte, spanID.Size())

		_, _ = traceID.MarshalTo(traceBytes)
		_, _ = spanID.MarshalTo(spanBytes)

		ref2 := prototest.SpanRef{
			TraceId: traceBytes,
			SpanId:  spanBytes,
		}

		//Convert both SpanRefs into proto binary formats before
		//comparing to match with the standard protobuf encoding.
		d1, err := proto.Marshal(&ref1)
		if err != nil {
			t.Fatalf("marshal ref1 failed")
		}

		d2, err := proto.Marshal(&ref2)
		if err != nil {
			t.Fatalf("marshal ref2 failed")
		}

		if !bytes.Equal(d1, d2) {
			t.Fatalf("profound encoding mismatch")
		}

		var ref1u model.SpanRef
		if err := proto.Unmarshal(d1, &ref1u); err != nil {
			t.Fatalf("protobuf unmarshal failed: %v", err)
		}

		//Verify output of protobuf roundtrip to ensure there are no changes in the data.
		if !proto.Equal(&ref1, &ref1u) {
			t.Fatalf("protobuf roundtrip mismatched")
		}

		out := new(bytes.Buffer)
		if err := new(jsonpb.Marshaler).Marshal(out, &ref1); err != nil {
			t.Fatalf("json marshal failed: %v", err)
		}

		var ref1j model.SpanRef
		if err := jsonpb.Unmarshal(bytes.NewReader(out.Bytes()), &ref1j); err != nil {
			t.Fatalf("json unmarshal failed: %v", err)
		}

		if !proto.Equal(&ref1, &ref1j) {
			t.Fatalf("json roundtrip mismatch")
		}
	})
}
