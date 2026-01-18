// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	inputPath := flag.String("input", "", "Path to the input swagger.json file (required)")
	outputPath := flag.String("output", "", "Path to save the modified swagger.json file (defaults to input path)")
	flag.Parse()

	if *inputPath == "" {
		log.Fatal("--input flag is required")
	}

	if *outputPath == "" {
		*outputPath = *inputPath
	}

	if err := processSwagger(*inputPath, *outputPath); err != nil {
		log.Fatalf("Error processing swagger file: %v", err)
	}

	fmt.Printf("Successfully transformed %s -> %s\n", *inputPath, *outputPath)
}

func processSwagger(inputPath, outputPath string) error {
	// Read the swagger file
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Parse JSON
	var swagger map[string]interface{}
	if err := json.Unmarshal(data, &swagger); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Apply transformations
	transformSwagger(swagger)

	// Write back
	output, err := json.MarshalIndent(swagger, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	if err := os.WriteFile(outputPath, output, 0o600); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func transformSwagger(swagger map[string]interface{}) {
	definitions, ok := swagger["definitions"].(map[string]interface{})
	if !ok {
		return
	}

	// Process each definition
	for _, defValue := range definitions {
		defObj, ok := defValue.(map[string]interface{})
		if !ok {
			continue
		}

		// Handle enum-as-integer transformation
		if isEnumDefinition(defObj) {
			transformEnumToInteger(defObj)
			continue
		}

		// Handle properties within object definitions
		properties, ok := defObj["properties"].(map[string]interface{})
		if !ok {
			continue
		}

		for propName, propValue := range properties {
			propObj, ok := propValue.(map[string]interface{})
			if !ok {
				continue
			}

			// Transform TraceID and SpanID
			switch propName {
			case "trace_id", "traceId":
				transformTraceID(propObj)
			case "span_id", "spanId", "parent_span_id", "parentSpanId":
				transformSpanID(propObj)
			}

			// Transform 64-bit integer fields - already handled correctly in the swagger
			// (start_time_unix_nano, end_time_unix_nano, time_unix_nano are already type: string, format: uint64)

			// Handle enum references
			if ref, ok := propObj["$ref"].(string); ok {
				// Check if the referenced definition is an enum that should be transformed
				refName := strings.TrimPrefix(ref, "#/definitions/")
				if refDef, exists := definitions[refName]; exists {
					if refDefObj, ok := refDef.(map[string]interface{}); ok && isEnumDefinition(refDefObj) {
						// Replace the $ref with inline integer type
						transformEnumReference(propObj, refDefObj)
					}
				}
			}
		}
	}
}

func isEnumDefinition(def map[string]interface{}) bool {
	// Check if it's a string enum definition
	typeVal, hasType := def["type"].(string)
	_, hasEnum := def["enum"]
	return hasType && typeVal == "string" && hasEnum
}

func transformEnumToInteger(enumDef map[string]interface{}) {
	enumValues, ok := enumDef["enum"].([]interface{})
	if !ok {
		return
	}

	// Build the integer enum array and description mapping
	var intEnum []int
	var mappingParts []string

	for i, val := range enumValues {
		strVal, ok := val.(string)
		if !ok {
			continue
		}
		intEnum = append(intEnum, i)
		mappingParts = append(mappingParts, fmt.Sprintf("%d: %s", i, strVal))
	}

	// Update the definition
	enumDef["type"] = "integer"

	// Convert int slice to interface slice for JSON encoding
	intEnumInterface := make([]interface{}, len(intEnum))
	for i, v := range intEnum {
		intEnumInterface[i] = v
	}
	enumDef["enum"] = intEnumInterface

	// Update description with mapping
	description, _ := enumDef["description"].(string)
	mapping := strings.Join(mappingParts, ", ")
	if description != "" {
		enumDef["description"] = description + "\n\nValue mapping: " + mapping
	} else {
		enumDef["description"] = "Value mapping: " + mapping
	}

	// Remove default if it exists and is a string
	if defaultVal, exists := enumDef["default"].(string); exists {
		// Find the index of the default value
		for i, val := range enumValues {
			if strVal, ok := val.(string); ok && strVal == defaultVal {
				enumDef["default"] = i
				break
			}
		}
	}
}

func transformEnumReference(propObj map[string]interface{}, enumDef map[string]interface{}) {
	// Remove the $ref
	delete(propObj, "$ref")

	// Get enum values
	enumValues, ok := enumDef["enum"].([]interface{})
	if !ok {
		return
	}

	// Build integer enum
	var intEnum []interface{}
	var mappingParts []string

	for i, val := range enumValues {
		strVal, ok := val.(string)
		if !ok {
			continue
		}
		intEnum = append(intEnum, i)
		mappingParts = append(mappingParts, fmt.Sprintf("%d: %s", i, strVal))
	}

	// Set as integer type with enum values
	propObj["type"] = "integer"
	propObj["enum"] = intEnum

	// Preserve and update description
	description, _ := propObj["description"].(string)
	enumDescription, _ := enumDef["description"].(string)

	mapping := strings.Join(mappingParts, ", ")
	combinedDesc := description
	if enumDescription != "" {
		if combinedDesc != "" {
			combinedDesc += "\n\n" + enumDescription
		} else {
			combinedDesc = enumDescription
		}
	}
	if combinedDesc != "" {
		propObj["description"] = combinedDesc + "\n\nValue mapping: " + mapping
	} else {
		propObj["description"] = "Value mapping: " + mapping
	}
}

func transformTraceID(propObj map[string]interface{}) {
	// Check if it's already transformed (idempotency)
	if pattern, exists := propObj["pattern"].(string); exists && pattern == "^[0-9a-f]{32}$" {
		return
	}

	// Only transform if it's currently format: byte
	format, hasFormat := propObj["format"].(string)
	if !hasFormat || format != "byte" {
		return
	}

	// Set type to string (should already be string, but ensure it)
	propObj["type"] = "string"

	// Remove format: byte
	delete(propObj, "format")

	// Add pattern for 32-character hex string (16 bytes = 32 hex chars)
	propObj["pattern"] = "^[0-9a-f]{32}$"

	// Update description
	description, _ := propObj["description"].(string)
	if !strings.Contains(description, "hex-encoded") {
		if description != "" {
			propObj["description"] = description + " Byte array as hex-encoded string."
		} else {
			propObj["description"] = "Byte array as hex-encoded string."
		}
	}
}

func transformSpanID(propObj map[string]interface{}) {
	// Check if it's already transformed (idempotency)
	if pattern, exists := propObj["pattern"].(string); exists && pattern == "^[0-9a-f]{16}$" {
		return
	}

	// Only transform if it's currently format: byte
	format, hasFormat := propObj["format"].(string)
	if !hasFormat || format != "byte" {
		return
	}

	// Set type to string (should already be string, but ensure it)
	propObj["type"] = "string"

	// Remove format: byte
	delete(propObj, "format")

	// Add pattern for 16-character hex string (8 bytes = 16 hex chars)
	propObj["pattern"] = "^[0-9a-f]{16}$"

	// Update description
	description, _ := propObj["description"].(string)
	if !strings.Contains(description, "hex-encoded") {
		if description != "" {
			propObj["description"] = description + " Byte array as hex-encoded string."
		} else {
			propObj["description"] = "Byte array as hex-encoded string."
		}
	}
}
