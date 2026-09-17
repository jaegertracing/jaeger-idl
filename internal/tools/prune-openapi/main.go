// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("Usage: %s <openapi-file>", os.Args[0])
	}
	filename := os.Args[1]

	data, err := os.ReadFile(filename)
	if err != nil {
		log.Fatalf("Error reading file: %v", err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		log.Fatalf("Error unmarshaling YAML: %v", err)
	}

	// 1. Find the "paths" node and "components/schemas" node
	pathsNode := findNode(&root, "paths")
	if pathsNode == nil {
		log.Fatalf("Could not find 'paths' in OpenAPI spec")
	}

	componentsNode := findNode(&root, "components")
	if componentsNode == nil {
		log.Fatalf("Could not find 'components' in OpenAPI spec")
	}
	schemasNode := findNode(componentsNode, "schemas")
	if schemasNode == nil {
		log.Fatalf("Could not find 'schemas' in 'components'")
	}

	// 1.5 Fix duplicated operationId for POST /api/v3/traces
	// Iterate paths to find /api/v3/traces -> post -> operationId
	for i := 0; i < len(pathsNode.Content); i += 2 {
		pathKey := pathsNode.Content[i].Value
		if pathKey == "/api/v3/traces" {
			pathVal := pathsNode.Content[i+1]
			// Find "post"
			for j := 0; j < len(pathVal.Content); j += 2 {
				method := pathVal.Content[j].Value
				if method == "post" {
					methodVal := pathVal.Content[j+1]
					// Find "operationId"
					for k := 0; k < len(methodVal.Content); k += 2 {
						if methodVal.Content[k].Value == "operationId" {
							if methodVal.Content[k+1].Value == "QueryService_FindTraces" {
								methodVal.Content[k+1].Value = "QueryService_FindTracesPost"
							}
						}
					}
				}
			}
		}
	}

	// 1.6 Collapse the flattened `query.filter.*` GET parameters into a single
	// `query.filter` string parameter. A message-typed field in a GET binding is
	// expanded by field path, so `filter` (a Call) yields a useless `query.filter.op`
	// and drops the recursive `args`. The filter is actually passed as a URL-encoded
	// JSON object, so replace the flattened parameters with one string parameter.
	for i := 0; i < len(pathsNode.Content); i += 2 {
		pathVal := pathsNode.Content[i+1]
		for j := 0; j < len(pathVal.Content); j += 2 {
			collapseFilterParams(pathVal.Content[j+1])
		}
	}

	// 1.7 Point the GetTrace 200 response at the `{"result": ...}` envelope the HTTP
	// gateway actually sends. The operation description has always said the body is
	// wrapped while the $ref named a bare TracesData, so every generated client
	// unmarshals one level too shallow. GRPCGatewayWrapper is declared in the proto for
	// exactly this shape, but gnostic only emits schemas that something already
	// references, so the schema has to be supplied here alongside the rewritten ref.
	if envelopeGetTraceResponse(pathsNode) {
		insertSchema(schemasNode, envelopeSchemaName, envelopeSchema())
	}

	// 1.8 Declare the OTLP ID fields required. Their own descriptions say "This field is
	// required", but they are declared in the external opentelemetry-proto submodule, so
	// neither (google.api.field_behavior) nor (openapi.v3.schema) can reach them.
	markRequiredFields(schemasNode)

	// 1.9 Replace `format: bytes` on the ID fields with a hex pattern. `bytes` is the
	// protobuf type name emitted verbatim, not a registered OpenAPI format, and it points
	// readers and code generators at base64 while the gateway sends hex strings.
	fixIDFormats(schemasNode)

	// 2. Identify all reachable schemas starting from "paths"
	reachable := make(map[string]bool)

	// Start traversal from root to find initial references
	queue := make([]string, 0)
	initialRefFinder := func(n *yaml.Node) {
		findRefs(n, &reachable, &queue)
	}

	// Traverse root structure, intentionally skipping components/schemas definition scan
	if root.Kind == yaml.DocumentNode {
		rootMap := root.Content[0]
		for i := 0; i < len(rootMap.Content); i += 2 {
			key := rootMap.Content[i].Value
			val := rootMap.Content[i+1]

			if key == "components" {
				for j := 0; j < len(val.Content); j += 2 {
					compKey := val.Content[j].Value
					compVal := val.Content[j+1]
					if compKey == "schemas" {
						continue
					}
					traverse(compVal, initialRefFinder)
				}
			} else {
				traverse(val, initialRefFinder)
			}
		}
	}

	// Now process queue
	// For each schema in queue, validate it exists in schemasNode, then traverse it to find more refs
	schemaMap := make(map[string]*yaml.Node)
	for i := 0; i < len(schemasNode.Content); i += 2 {
		schemaMap[schemasNode.Content[i].Value] = schemasNode.Content[i+1]
	}

	processed := make(map[string]bool)

	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]

		if processed[name] {
			continue
		}
		processed[name] = true

		node, exists := schemaMap[name]
		if !exists {
			// Ref points to non-existent schema? Ignore or warn.
			log.Printf("Warning: reference to non-existent schema %q found. This may indicate a missing import or incorrectly generated reference. The schema will be skipped, but you may need to verify the proto definitions or regenerate the OpenAPI spec.", name)
			continue
		}

		// Traverse this schema definition to find downstream refs
		traverse(node, initialRefFinder)
	}

	// 3. Prune schemas
	// Rebuild schemasNode.Content keeping only those in 'reachable'
	newContent := make([]*yaml.Node, 0)
	for i := 0; i < len(schemasNode.Content); i += 2 {
		name := schemasNode.Content[i].Value
		val := schemasNode.Content[i+1]

		if reachable[name] {
			newContent = append(newContent, schemasNode.Content[i], val)
		} else {
			fmt.Printf("Pruning unused schema: %s\n", name)
		}
	}
	schemasNode.Content = newContent

	// 4. Write back
	outFile, err := os.Create(filename)
	if err != nil {
		log.Fatalf("Error creating output file: %v", err)
	}
	defer outFile.Close()

	encoder := yaml.NewEncoder(outFile)
	encoder.SetIndent(4)
	if err := encoder.Encode(&root); err != nil {
		log.Fatalf("Error encoding YAML: %v", err)
	}
}

// collapseFilterParams rewrites a method node's `parameters` list, replacing any
// `query.filter.*` entries (the field-path expansion of the Call-typed filter)
// with a single `query.filter` string parameter carrying an example.
func collapseFilterParams(methodVal *yaml.Node) {
	if methodVal.Kind != yaml.MappingNode {
		return
	}
	for k := 0; k+1 < len(methodVal.Content); k += 2 {
		if methodVal.Content[k].Value != "parameters" {
			continue
		}
		params := methodVal.Content[k+1]
		kept := make([]*yaml.Node, 0, len(params.Content))
		collapsed := false
		for _, p := range params.Content {
			// Match the flattened `query.filter.*` expansion exactly — the bare
			// `query.filter` itself and its dotted children — without eating an
			// unrelated sibling like a future `query.filter_mode`.
			if name := paramName(p); name == "query.filter" || strings.HasPrefix(name, "query.filter.") {
				collapsed = true
				continue
			}
			kept = append(kept, p)
		}
		if collapsed {
			params.Content = append(kept, filterQueryParam())
		}
	}
}

func paramName(param *yaml.Node) string {
	for i := 0; i+1 < len(param.Content); i += 2 {
		if param.Content[i].Value == "name" {
			return param.Content[i+1].Value
		}
	}
	return ""
}

func scalarNode(value string, style yaml.Style) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value, Style: style}
}

func mappingNode(pairs ...*yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: pairs}
}

// filterQueryParam is the replacement parameter: a plain string, because a Call nests to no fixed
// depth and a GET binding cannot expand that by field path. It documents the shape with an example
// rather than an items schema, which would be invalid on a string.
func filterQueryParam() *yaml.Node {
	return mappingNode(
		scalarNode("name", 0), scalarNode("query.filter", 0),
		scalarNode("in", 0), scalarNode("query", 0),
		scalarNode("description", 0), scalarNode("Structured query filter as a URL-encoded JSON object (a Call expression). See RFC 0005 §6.", 0),
		scalarNode("schema", 0), mappingNode(
			scalarNode("type", 0), scalarNode("string", 0),
			scalarNode("example", 0), scalarNode(`{"op":"eq","args":[{"attr":{"key":"http.status_code"}},{"scalar":{"value":"500"}}]}`, yaml.SingleQuotedStyle),
		),
	)
}

func traverse(node *yaml.Node, visitor func(*yaml.Node)) {
	visitor(node)
	for _, child := range node.Content {
		traverse(child, visitor)
	}
}

func findRefs(node *yaml.Node, reachable *map[string]bool, queue *[]string) {
	// Check if current node is a ref
	// A ref in yaml.v3 is somewhat structural.
	// Usually it looks like Key: $ref, Value: "#/components/schemas/Foo" in a map
	// OR simply scanning scalar values checking for prefix.

	// Standard $ref is a key-value pair in a mapping.
	// However, the traverse function visits all nodes.
	// If we are at a Scalar node with Value starting with "#/components/schemas/", it's likely a value of a $ref key.
	// But to be precise we should check parent?
	// Actually, scanning all scalars for the string pattern is safe enough for this specific domain.

	if node.Kind == yaml.ScalarNode {
		val := node.Value
		prefix := "#/components/schemas/"
		if len(val) > len(prefix) && val[:len(prefix)] == prefix {
			schemaName := val[len(prefix):]
			if !(*reachable)[schemaName] {
				(*reachable)[schemaName] = true
				*queue = append(*queue, schemaName)
			}
		}
	}
}

func findNode(root *yaml.Node, key string) *yaml.Node {
	// Assuming root is Document -> Mapping
	var mapNode *yaml.Node
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) > 0 {
			mapNode = root.Content[0]
		}
	} else {
		mapNode = root
	}

	if mapNode == nil || mapNode.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i < len(mapNode.Content); i += 2 {
		if mapNode.Content[i].Value == key {
			return mapNode.Content[i+1]
		}
	}
	return nil
}

// Names of the nodes patched by hand below. GRPCGatewayWrapper is declared in
// proto/api_v3/query_service.proto for the {"result": ...} document the gateway sends.
const (
	schemaRefPrefix    = "#/components/schemas/"
	getTracePath       = "/api/v3/traces/{traceId}"
	tracesDataSchema   = "opentelemetry.proto.trace.v1.TracesData"
	envelopeSchemaName = "jaeger.api_v3.GRPCGatewayWrapper"
)

// requiredFields lists the fields whose own description in the generated document already
// states they are required. Only fields that exist under `properties` are declared, so a
// rename upstream drops the name instead of publishing a required field that is not there.
var requiredFields = []struct {
	schema string
	fields []string
}{
	{"opentelemetry.proto.trace.v1.Span", []string{"traceId", "spanId"}},
}

// idFields lists the ID fields that carry `format: bytes`, with the hex shape the gateway
// actually sends. parentSpanId is empty on a root span, so its pattern admits the empty
// string. opentelemetry.proto.common.v1.AnyValue.bytesValue is deliberately absent: it is a
// genuine bytes field and base64 is the correct reading of it.
var idFields = []struct {
	schema  string
	field   string
	pattern string
}{
	{"opentelemetry.proto.trace.v1.Span", "traceId", "^[0-9a-f]{32}$"},
	{"opentelemetry.proto.trace.v1.Span", "spanId", "^[0-9a-f]{16}$"},
	{"opentelemetry.proto.trace.v1.Span", "parentSpanId", "^([0-9a-f]{16})?$"},
	{"opentelemetry.proto.trace.v1.Span_Link", "traceId", "^[0-9a-f]{32}$"},
	{"opentelemetry.proto.trace.v1.Span_Link", "spanId", "^[0-9a-f]{16}$"},
}

// descend walks a chain of mapping keys, returning nil as soon as one is missing.
func descend(node *yaml.Node, keys ...string) *yaml.Node {
	for _, key := range keys {
		if node == nil {
			return nil
		}
		node = findNode(node, key)
	}
	return node
}

func seqNode(items ...*yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: items}
}

// setPair sets key to val in a mapping, replacing an existing entry in place so the
// surrounding key order is preserved, and otherwise appending.
func setPair(mapping *yaml.Node, key string, val *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = val
			return
		}
	}
	mapping.Content = append(mapping.Content, scalarNode(key, 0), val)
}

// envelopeGetTraceResponse repoints the GetTrace 200 response at the envelope schema. It
// changes nothing and reports false unless the ref is the bare TracesData, so neither a
// missing path nor an already enveloped ref can cause a second copy of the schema to be
// injected.
func envelopeGetTraceResponse(pathsNode *yaml.Node) bool {
	schema := descend(pathsNode, getTracePath, "get", "responses", "200", "content", "application/json", "schema")
	ref := descend(schema, "$ref")
	if ref == nil || ref.Value != schemaRefPrefix+tracesDataSchema {
		return false
	}
	ref.Value = schemaRefPrefix + envelopeSchemaName
	return true
}

// envelopeSchema is what gnostic would have emitted for GRPCGatewayWrapper had anything
// referenced it. The description paraphrases that message's own comment, naming
// google.rpc.Status for the error case because that is the schema this document's default
// response refs, and leaving out the note about a possible future chunked multi-response,
// which describes where the server may go rather than the body this schema describes.
func envelopeSchema() *yaml.Node {
	description := "GRPCGatewayWrapper wraps streaming responses from GetTrace for HTTP.\n" +
		"Today there is always only one response because internally the HTTP server gets\n" +
		"data from QueryService that does not support multiple responses. In case of errors,\n" +
		"google.rpc.Status is returned instead.\n\n" +
		"See https://github.com/grpc-ecosystem/grpc-gateway/issues/2189"
	return mappingNode(
		scalarNode("required", 0), seqNode(scalarNode("result", 0)),
		scalarNode("type", 0), scalarNode("object", 0),
		scalarNode("properties", 0), mappingNode(
			scalarNode("result", 0), mappingNode(
				scalarNode("allOf", 0), seqNode(mappingNode(
					scalarNode("$ref", 0), scalarNode(schemaRefPrefix+tracesDataSchema, yaml.SingleQuotedStyle),
				)),
				scalarNode("description", 0), scalarNode("The trace data, always present on a 200 response.", 0),
			),
		),
		scalarNode("description", 0), scalarNode(description, yaml.LiteralStyle),
	)
}

// insertSchema adds a schema to components/schemas, or replaces one already there under the
// same name, keeping the byte order the generator emits so the document reads as though it
// had been generated with the schema in place.
func insertSchema(schemasNode *yaml.Node, name string, schema *yaml.Node) {
	for i := 0; i+1 < len(schemasNode.Content); i += 2 {
		if schemasNode.Content[i].Value == name {
			schemasNode.Content[i+1] = schema
			return
		}
		if schemasNode.Content[i].Value > name {
			tail := append([]*yaml.Node{scalarNode(name, 0), schema}, schemasNode.Content[i:]...)
			schemasNode.Content = append(schemasNode.Content[:i:i], tail...)
			return
		}
	}
	schemasNode.Content = append(schemasNode.Content, scalarNode(name, 0), schema)
}

// markRequiredFields declares the fields listed in requiredFields on their own schema. The
// list is prepended, where the generator puts it on the schemas that have one, or replaces
// an existing list in its current slot.
func markRequiredFields(schemasNode *yaml.Node) {
	for _, want := range requiredFields {
		schema := findNode(schemasNode, want.schema)
		properties := descend(schema, "properties")
		if properties == nil {
			continue
		}
		names := seqNode()
		for _, field := range want.fields {
			if findNode(properties, field) != nil {
				names.Content = append(names.Content, scalarNode(field, 0))
			}
		}
		if len(names.Content) == 0 {
			continue
		}
		if findNode(schema, "required") != nil {
			setPair(schema, "required", names)
			continue
		}
		schema.Content = append([]*yaml.Node{scalarNode("required", 0), names}, schema.Content...)
	}
}

// fixIDFormats swaps `format: bytes` for a hex `pattern` on the ID fields, in place, so the
// surrounding keys keep the order the generator emitted.
func fixIDFormats(schemasNode *yaml.Node) {
	for _, id := range idFields {
		field := descend(schemasNode, id.schema, "properties", id.field)
		if field == nil {
			continue
		}
		for i := 0; i+1 < len(field.Content); i += 2 {
			if field.Content[i].Value == "format" && field.Content[i+1].Value == "bytes" {
				field.Content[i] = scalarNode("pattern", 0)
				field.Content[i+1] = scalarNode(id.pattern, yaml.SingleQuotedStyle)
				break
			}
		}
	}
}
