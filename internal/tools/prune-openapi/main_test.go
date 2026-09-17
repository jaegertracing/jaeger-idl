// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func mkParam(name string) *yaml.Node {
	return mappingNode(scalarNode("name", 0), scalarNode(name, 0))
}

func paramNames(seq *yaml.Node) []string {
	var out []string
	for _, p := range seq.Content {
		out = append(out, paramName(p))
	}
	return out
}

func TestCollapseFilterParams(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			// The two query.filter.* entries collapse into one query.filter (appended
			// last); query.filter_mode is a sibling that starts with the same prefix and
			// must survive — the bug this test guards against.
			name: "collapse the query.filter.* expansion, keep siblings",
			in:   []string{"query.service_name", "query.filter.op", "query.filter.args", "query.filter_mode", "query.raw_traces"},
			want: []string{"query.service_name", "query.filter_mode", "query.raw_traces", "query.filter"},
		},
		{
			name: "no filter params: list unchanged",
			in:   []string{"query.service_name", "query.filter_mode", "query.raw_traces"},
			want: []string{"query.service_name", "query.filter_mode", "query.raw_traces"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seq := &yaml.Node{Kind: yaml.SequenceNode}
			for _, n := range tt.in {
				seq.Content = append(seq.Content, mkParam(n))
			}
			collapseFilterParams(mappingNode(scalarNode("parameters", 0), seq))
			if got := paramNames(seq); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFilterQueryParamExample checks the published example against the terms the expression proto
// actually defines. The example is the one part of the specification a reader is most likely to
// copy, and nothing else would catch it going stale when a term is renamed: the generator does not
// read it, and the pruning tool writes it as an opaque string.
func TestFilterQueryParamExample(t *testing.T) {
	arms := map[string]bool{
		"attr": true, "field": true, "nested": true, "scalar": true, "list": true, "call": true,
	}

	var example string
	traverse(filterQueryParam(), func(node *yaml.Node) {
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "example" {
				example = node.Content[i+1].Value
			}
		}
	})
	if example == "" {
		t.Fatal("the filter parameter publishes no example")
	}

	var call struct {
		Op   string                       `json:"op"`
		Args []map[string]json.RawMessage `json:"args"`
	}
	if err := json.Unmarshal([]byte(example), &call); err != nil {
		t.Fatalf("the example is not a JSON Call: %v", err)
	}
	if call.Op == "" {
		t.Errorf("the example names no operator: %s", example)
	}
	if len(call.Args) == 0 {
		t.Errorf("the example passes no arguments: %s", example)
	}
	for _, arg := range call.Args {
		for term := range arg {
			if !arms[term] {
				t.Errorf("the example uses %q, which is not a term of jaeger.expression.v1.Expression: %s",
					term, example)
			}
		}
	}
	if strings.Contains(example, `"ref"`) {
		t.Errorf("the example uses the removed single reference term: %s", example)
	}
}

// mkGetTracePaths builds the nesting the GetTrace 200 response actually sits in, so the
// walk down to the schema is exercised rather than stubbed.
func mkGetTracePaths(ref string) *yaml.Node {
	return mappingNode(
		scalarNode(getTracePath, 0), mappingNode(
			scalarNode("get", 0), mappingNode(
				scalarNode("responses", 0), mappingNode(
					scalarNode("200", 0), mappingNode(
						scalarNode("content", 0), mappingNode(
							scalarNode("application/json", 0), mappingNode(
								scalarNode("schema", 0), mappingNode(
									scalarNode("$ref", 0), scalarNode(ref, 0),
								),
							),
						),
					),
				),
			),
		),
	)
}

func mapKeys(mapping *yaml.Node) []string {
	var out []string
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		out = append(out, mapping.Content[i].Value)
	}
	return out
}

func seqValues(seq *yaml.Node) []string {
	var out []string
	for _, n := range seq.Content {
		out = append(out, n.Value)
	}
	return out
}

// TestEnvelopeGetTraceResponse covers the rewrite and, just as importantly, the two cases
// where it must decline: the return value gates schema injection, so a false positive would
// publish a second copy of the envelope schema.
func TestEnvelopeGetTraceResponse(t *testing.T) {
	tests := []struct {
		name    string
		paths   *yaml.Node
		want    bool
		wantRef string
	}{
		{
			name:    "rewrites the bare TracesData ref",
			paths:   mkGetTracePaths(schemaRefPrefix + tracesDataSchema),
			want:    true,
			wantRef: schemaRefPrefix + envelopeSchemaName,
		},
		{
			// If a future generator emits the envelope itself, the rewrite is already done
			// and injecting the schema again would duplicate the key.
			name:    "leaves an already enveloped ref alone",
			paths:   mkGetTracePaths(schemaRefPrefix + envelopeSchemaName),
			want:    false,
			wantRef: schemaRefPrefix + envelopeSchemaName,
		},
		{
			name:  "no GetTrace path: nothing to rewrite",
			paths: mappingNode(scalarNode("/api/v3/services", 0), mappingNode()),
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := envelopeGetTraceResponse(tt.paths); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			if tt.wantRef == "" {
				return
			}
			schema := descend(tt.paths, getTracePath, "get", "responses", "200", "content", "application/json", "schema")
			if got := descend(schema, "$ref").Value; got != tt.wantRef {
				t.Errorf("ref is %q, want %q", got, tt.wantRef)
			}
		})
	}
}

// TestEnvelopeSchemaShape pins the published envelope against the contract the operation
// description states: a `result` object that is always present and holds a TracesData.
// Nothing else would catch the schema drifting away from the ref the rewrite installs.
func TestEnvelopeSchemaShape(t *testing.T) {
	schema := envelopeSchema()

	required := findNode(schema, "required")
	if required == nil {
		t.Fatal("the envelope declares no required list")
	}
	if got := seqValues(required); !reflect.DeepEqual(got, []string{"result"}) {
		t.Errorf("required is %v, want [result]", got)
	}

	result := descend(schema, "properties", "result")
	if result == nil {
		t.Fatal("the envelope has no result property")
	}
	var refs []string
	traverse(result, func(node *yaml.Node) {
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "$ref" {
				refs = append(refs, node.Content[i+1].Value)
			}
		}
	})
	if !reflect.DeepEqual(refs, []string{schemaRefPrefix + tracesDataSchema}) {
		t.Errorf("result refs %v, want the bare TracesData", refs)
	}
}

// TestInsertSchema checks the sorted placement. The generator emits components/schemas in
// byte order, so inserting anywhere else would show up as unrelated churn the next time
// someone reads the document, and `GRPC` sorting before `Get` is the case that catches a
// case-insensitive comparison.
func TestInsertSchema(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		add  string
		want []string
	}{
		{
			name: "GRPCGatewayWrapper sorts before GetOperationsResponse",
			in:   []string{"jaeger.api_v3.FindTracesRequest", "jaeger.api_v3.GetOperationsResponse"},
			add:  envelopeSchemaName,
			want: []string{"jaeger.api_v3.FindTracesRequest", envelopeSchemaName, "jaeger.api_v3.GetOperationsResponse"},
		},
		{
			name: "sorts last when nothing follows it",
			in:   []string{"google.protobuf.Any"},
			add:  envelopeSchemaName,
			want: []string{"google.protobuf.Any", envelopeSchemaName},
		},
		{
			name: "replaces in place, no duplicate key",
			in:   []string{"google.protobuf.Any", envelopeSchemaName, "opentelemetry.proto.trace.v1.Span"},
			add:  envelopeSchemaName,
			want: []string{"google.protobuf.Any", envelopeSchemaName, "opentelemetry.proto.trace.v1.Span"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schemas := mappingNode()
			for _, name := range tt.in {
				schemas.Content = append(schemas.Content, scalarNode(name, 0), mappingNode())
			}
			insertSchema(schemas, tt.add, envelopeSchema())
			if got := mapKeys(schemas); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
			if findNode(schemas, tt.add) == nil {
				t.Error("the inserted schema is not reachable by its own name")
			}
		})
	}
}

// TestMarkRequiredFields covers the placement and the drift guard. A required list naming a
// field that no longer exists under properties is an invalid document, so an upstream rename
// must drop the name rather than publish it.
func TestMarkRequiredFields(t *testing.T) {
	spanSchema := func(fields ...string) *yaml.Node {
		properties := mappingNode()
		for _, f := range fields {
			properties.Content = append(properties.Content, scalarNode(f, 0), mappingNode())
		}
		return mappingNode(
			scalarNode("type", 0), scalarNode("object", 0),
			scalarNode("properties", 0), properties,
		)
	}

	t.Run("required is prepended, where the generator puts it", func(t *testing.T) {
		schemas := mappingNode(scalarNode("opentelemetry.proto.trace.v1.Span", 0), spanSchema("traceId", "spanId", "name"))
		markRequiredFields(schemas)
		schema := findNode(schemas, "opentelemetry.proto.trace.v1.Span")
		if got := mapKeys(schema); !reflect.DeepEqual(got, []string{"required", "type", "properties"}) {
			t.Errorf("key order is %v, want required first", got)
		}
		if got := seqValues(findNode(schema, "required")); !reflect.DeepEqual(got, []string{"traceId", "spanId"}) {
			t.Errorf("required is %v, want [traceId spanId]", got)
		}
	})

	t.Run("a renamed field is dropped, not published", func(t *testing.T) {
		schemas := mappingNode(scalarNode("opentelemetry.proto.trace.v1.Span", 0), spanSchema("spanId"))
		markRequiredFields(schemas)
		schema := findNode(schemas, "opentelemetry.proto.trace.v1.Span")
		if got := seqValues(findNode(schema, "required")); !reflect.DeepEqual(got, []string{"spanId"}) {
			t.Errorf("required is %v, want only the field that exists", got)
		}
	})

	// The generator emits `required` for the schemas it owns, so a schema in this table
	// could arrive already carrying one; replacing it in place keeps the key order the
	// document already has rather than moving the list to the end.
	t.Run("an existing required list is replaced in place", func(t *testing.T) {
		schema := mappingNode(
			scalarNode("type", 0), scalarNode("object", 0),
			scalarNode("required", 0), seqNode(scalarNode("name", 0)),
			scalarNode("properties", 0), mappingNode(
				scalarNode("traceId", 0), mappingNode(),
				scalarNode("spanId", 0), mappingNode(),
			),
		)
		schemas := mappingNode(scalarNode("opentelemetry.proto.trace.v1.Span", 0), schema)
		markRequiredFields(schemas)
		if got := mapKeys(schema); !reflect.DeepEqual(got, []string{"type", "required", "properties"}) {
			t.Errorf("key order is %v, want the existing slot kept", got)
		}
		if got := seqValues(findNode(schema, "required")); !reflect.DeepEqual(got, []string{"traceId", "spanId"}) {
			t.Errorf("required is %v, want [traceId spanId]", got)
		}
	})

	t.Run("a missing schema is skipped", func(t *testing.T) {
		schemas := mappingNode()
		markRequiredFields(schemas)
		if len(schemas.Content) != 0 {
			t.Errorf("markRequiredFields invented a schema: %v", mapKeys(schemas))
		}
	})
}

// TestFixIDFormats checks that the swap happens in place. `pattern` taking the slot
// `format` held keeps the diff to one line per field; appending instead would reorder the
// keys around every ID in the document.
func TestFixIDFormats(t *testing.T) {
	idField := func() *yaml.Node {
		return mappingNode(
			scalarNode("type", 0), scalarNode("string", 0),
			scalarNode("description", 0), scalarNode("A unique identifier for a trace.", 0),
			scalarNode("format", 0), scalarNode("bytes", 0),
		)
	}
	schemas := mappingNode(
		scalarNode("opentelemetry.proto.trace.v1.Span", 0), mappingNode(
			scalarNode("properties", 0), mappingNode(
				scalarNode("traceId", 0), idField(),
				scalarNode("flags", 0), mappingNode(
					scalarNode("type", 0), scalarNode("integer", 0),
					scalarNode("format", 0), scalarNode("uint32", 0),
				),
			),
		),
		// A genuine bytes field, and the one site that must keep format: bytes.
		scalarNode("opentelemetry.proto.common.v1.AnyValue", 0), mappingNode(
			scalarNode("properties", 0), mappingNode(
				scalarNode("bytesValue", 0), idField(),
			),
		),
	)

	fixIDFormats(schemas)

	traceID := descend(schemas, "opentelemetry.proto.trace.v1.Span", "properties", "traceId")
	if got := mapKeys(traceID); !reflect.DeepEqual(got, []string{"type", "description", "pattern"}) {
		t.Errorf("key order is %v, want pattern in the slot format held", got)
	}
	if got := findNode(traceID, "pattern").Value; got != "^[0-9a-f]{32}$" {
		t.Errorf("traceId pattern is %q", got)
	}

	flags := descend(schemas, "opentelemetry.proto.trace.v1.Span", "properties", "flags")
	if got := findNode(flags, "format").Value; got != "uint32" {
		t.Errorf("an unrelated format was rewritten: %q", got)
	}

	bytesValue := descend(schemas, "opentelemetry.proto.common.v1.AnyValue", "properties", "bytesValue")
	if got := findNode(bytesValue, "format"); got == nil || got.Value != "bytes" {
		t.Error("AnyValue.bytesValue lost format: bytes; it is a genuine bytes field")
	}
}

// TestIDPatternsMatchTheWireFormat is the point of the change: the patterns must accept what
// the gateway sends and reject the base64 that `format: bytes` implied. The values are taken
// from a jaegertracing/jaeger:2.21.0 capture.
func TestIDPatternsMatchTheWireFormat(t *testing.T) {
	patterns := make(map[string]string)
	for _, id := range idFields {
		patterns[id.schema+"."+id.field] = id.pattern
	}

	tests := []struct {
		field string
		value string
		want  bool
	}{
		{"opentelemetry.proto.trace.v1.Span.traceId", "0123456789abcdef0123456789abcdef", true},
		{"opentelemetry.proto.trace.v1.Span.traceId", "ASNFZ4mrze8BI0VniavN7w==", false},
		{"opentelemetry.proto.trace.v1.Span.traceId", "0123456789ABCDEF0123456789ABCDEF", false},
		{"opentelemetry.proto.trace.v1.Span.traceId", "0123456789abcdef", false},
		{"opentelemetry.proto.trace.v1.Span.spanId", "0123456789abcdef", true},
		{"opentelemetry.proto.trace.v1.Span.spanId", "", false},
		// A root span reports no parent, and the gateway omits or empties the field.
		{"opentelemetry.proto.trace.v1.Span.parentSpanId", "", true},
		{"opentelemetry.proto.trace.v1.Span.parentSpanId", "fedcba9876543210", true},
		{"opentelemetry.proto.trace.v1.Span_Link.traceId", "0123456789abcdef0123456789abcdef", true},
		{"opentelemetry.proto.trace.v1.Span_Link.spanId", "0123456789abcdef", true},
	}
	for _, tt := range tests {
		pattern, ok := patterns[tt.field]
		if !ok {
			t.Errorf("%s carries no pattern", tt.field)
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			t.Errorf("%s: pattern %q does not compile: %v", tt.field, pattern, err)
			continue
		}
		if got := re.MatchString(tt.value); got != tt.want {
			t.Errorf("%s: %q matched %v, want %v (pattern %s)", tt.field, tt.value, got, tt.want, pattern)
		}
	}
}
