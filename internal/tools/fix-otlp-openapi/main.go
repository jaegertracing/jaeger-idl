// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func main() {
	inputPath := flag.String("input", "", "Path to the input OpenAPI YAML file (required)")
	outputPath := flag.String("output", "", "Path to save the modified OpenAPI YAML file (defaults to input path)")
	flag.Parse()

	if *inputPath == "" {
		log.Fatal("--input flag is required")
	}

	if *outputPath == "" {
		*outputPath = *inputPath
	}

	if err := processOpenAPI(*inputPath, *outputPath); err != nil {
		log.Fatalf("Error processing OpenAPI file: %v", err)
	}

	fmt.Printf("Successfully transformed %s -> %s\n", *inputPath, *outputPath)
}

func processOpenAPI(inputPath, outputPath string) error {
	// Read the OpenAPI file
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Parse YAML into Node to preserve order
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Apply transformations
	transformOpenAPI(&root)

	// Write back
	//nolint:gosec // G306: Generated OpenAPI file needs to be readable by other tools and users
	f, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open file for writing: %w", err)
	}
	defer f.Close()

	encoder := yaml.NewEncoder(f)
	encoder.SetIndent(4)
	if err := encoder.Encode(&root); err != nil {
		return fmt.Errorf("failed to encode YAML: %w", err)
	}

	return nil
}

func transformOpenAPI(root *yaml.Node) {
	// Navigate to components.schemas
	schemas := findNodeByPath(root, []string{"components", "schemas"})
	if schemas == nil || schemas.Kind != yaml.MappingNode {
		return
	}

	// Process each schema definition
	for i := 0; i < len(schemas.Content); i += 2 {
		schemaNode := schemas.Content[i+1]

		if schemaNode.Kind != yaml.MappingNode {
			continue
		}

		// Process properties within the schema
		properties := findNodeByKey(schemaNode, "properties")
		if properties != nil && properties.Kind == yaml.MappingNode {
			for j := 0; j < len(properties.Content); j += 2 {
				propName := properties.Content[j].Value
				propNode := properties.Content[j+1]

				if propNode.Kind != yaml.MappingNode {
					continue
				}

				// Transform TraceID and SpanID
				switch propName {
				case "traceId":
					transformTraceID(propNode)
				case "spanId", "parentSpanId":
					transformSpanID(propNode)
				}

				// Transform enum fields
				format := getNodeValue(propNode, "format")
				if format == "enum" {
					transformEnum(propNode, propName)
				}
			}
		}
	}
}

func findNodeByPath(node *yaml.Node, path []string) *yaml.Node {
	current := node
	for _, key := range path {
		if current.Kind == yaml.DocumentNode && len(current.Content) > 0 {
			current = current.Content[0]
		}
		if current.Kind != yaml.MappingNode {
			return nil
		}
		found := false
		for i := 0; i < len(current.Content); i += 2 {
			if current.Content[i].Value == key {
				current = current.Content[i+1]
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return current
}

func findNodeByKey(node *yaml.Node, key string) *yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func getNodeValue(node *yaml.Node, key string) string {
	valueNode := findNodeByKey(node, key)
	if valueNode != nil {
		return valueNode.Value
	}
	return ""
}

func setNodeValue(node *yaml.Node, key, value string) {
	if node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content[i+1].Value = value
			return
		}
	}
	// Key doesn't exist, add it
	keyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: key,
	}
	valueNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: value,
	}
	node.Content = append(node.Content, keyNode, valueNode)
}

func removeNodeKey(node *yaml.Node, key string) {
	if node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			// Remove both key and value nodes
			node.Content = append(node.Content[:i], node.Content[i+2:]...)
			return
		}
	}
}

func updateNodeDescription(node *yaml.Node, suffix string) {
	description := getNodeValue(node, "description")
	if !strings.Contains(description, "hex-encoded") {
		if description != "" {
			setNodeValue(node, "description", description+" "+suffix)
		} else {
			setNodeValue(node, "description", suffix)
		}
	}
}

func transformTraceID(propNode *yaml.Node) {
	// Check if it's already transformed (idempotency)
	pattern := getNodeValue(propNode, "pattern")
	if pattern == "^[0-9a-f]{32}$" {
		return
	}

	// Only transform if it's currently format: bytes
	format := getNodeValue(propNode, "format")
	if format != "bytes" {
		return
	}

	// Remove format: bytes
	removeNodeKey(propNode, "format")

	// Add pattern for 32-character hex string (16 bytes = 32 hex chars)
	setNodeValue(propNode, "pattern", "^[0-9a-f]{32}$")

	// Update description
	updateNodeDescription(propNode, "Byte array as hex-encoded string.")
}

func transformSpanID(propNode *yaml.Node) {
	// Check if it's already transformed (idempotency)
	pattern := getNodeValue(propNode, "pattern")
	if pattern == "^[0-9a-f]{16}$" {
		return
	}

	// Only transform if it's currently format: bytes
	format := getNodeValue(propNode, "format")
	if format != "bytes" {
		return
	}

	// Remove format: bytes
	removeNodeKey(propNode, "format")

	// Add pattern for 16-character hex string (8 bytes = 16 hex chars)
	setNodeValue(propNode, "pattern", "^[0-9a-f]{16}$")

	// Update description
	updateNodeDescription(propNode, "Byte array as hex-encoded string.")
}

func transformEnum(propNode *yaml.Node, propName string) {
	// Check if already has enum values (idempotency)
	if findNodeByKey(propNode, "enum") != nil {
		return
	}

	// Only transform if it has format: enum
	format := getNodeValue(propNode, "format")
	if format != "enum" {
		return
	}

	// Remove format: enum
	removeNodeKey(propNode, "format")

	// Get enum values and descriptions based on property name
	var enumValues []int
	var enumDescriptions []string

	// SpanKind enum - based on OpenTelemetry proto definitions
	if propName == "kind" {
		enumValues = []int{0, 1, 2, 3, 4, 5}
		enumDescriptions = []string{
			"0: SPAN_KIND_UNSPECIFIED - Unspecified. Do NOT use as default",
			"1: SPAN_KIND_INTERNAL - Internal operation within an application (default)",
			"2: SPAN_KIND_SERVER - Server-side handling of RPC or remote request",
			"3: SPAN_KIND_CLIENT - Request to some remote service",
			"4: SPAN_KIND_PRODUCER - Producer sending a message to a broker",
			"5: SPAN_KIND_CONSUMER - Consumer receiving a message from a broker",
		}
	} else {
		// For other enum fields, we can't determine values without proto definitions
		return
	}

	// Add enum array
	addEnumArray(propNode, enumValues)

	// Update description with enum mapping
	description := getNodeValue(propNode, "description")
	mapping := strings.Join(enumDescriptions, "; ")
	if description != "" {
		setNodeValue(propNode, "description", description+"\n\nEnum values: "+mapping)
	} else {
		setNodeValue(propNode, "description", "Enum values: "+mapping)
	}
}

func addEnumArray(propNode *yaml.Node, values []int) {
	if propNode.Kind != yaml.MappingNode {
		return
	}

	// Create enum array node
	enumKey := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: "enum",
	}

	enumArray := &yaml.Node{
		Kind: yaml.SequenceNode,
	}

	for _, val := range values {
		enumArray.Content = append(enumArray.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: fmt.Sprintf("%d", val),
		})
	}

	// Add to propNode
	propNode.Content = append(propNode.Content, enumKey, enumArray)
}
