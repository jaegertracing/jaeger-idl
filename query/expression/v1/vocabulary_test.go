// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The closed vocabularies are written down twice: as enum annotations on the proto fields, which
// the OpenAPI document publishes, and as the slices this package validates against. The tests here
// compare the two, so a level or an operator added to one and forgotten in the other cannot pass —
// it would otherwise be schema-valid and refused by the validator, or accepted by the validator and
// unrepresentable on the wire.
//
// They read the published document rather than the annotations, because the document is what a
// client generates from, and CI regenerates it from the annotations and diffs it. It is read as
// JSON, which is the same document in a form the standard library parses: a YAML parser is not a
// dependency this module takes, since every direct dependency of the IDL reaches whoever imports it
// (see the swagger-json target).
const publishedSchema = "../../../swagger/api_v3/query_service.openapi.json"

func schemas(t *testing.T) map[string]any {
	raw, err := os.ReadFile(publishedSchema)
	require.NoError(t, err, "run `make swagger-json` if the published document is missing")

	var document struct {
		Components struct {
			Schemas map[string]any `json:"schemas"`
		} `json:"components"`
	}
	require.NoError(t, json.Unmarshal(raw, &document))
	require.NotEmpty(t, document.Components.Schemas, "the document publishes its schemas")
	return document.Components.Schemas
}

func publishedProperty(t *testing.T, message, field string) map[string]any {
	definition, ok := schemas(t)[message].(map[string]any)
	require.True(t, ok, "the document defines %s", message)
	properties, ok := definition["properties"].(map[string]any)
	require.True(t, ok, "%s publishes properties", message)
	property, ok := properties[field].(map[string]any)
	require.True(t, ok, "%s publishes %s", message, field)
	return property
}

func publishedEnum(t *testing.T, message, field string) []string {
	values, ok := publishedProperty(t, message, field)["enum"].([]any)
	require.True(t, ok, "%s.%s is an enumeration", message, field)

	out := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		require.True(t, ok, "%s.%s enumerates strings, got %#v", message, field, value)
		out = append(out, text)
	}
	return out
}

// TestPublishedLevelsMatchTheDomain compares the three reference terms against the levels this
// package accepts. The three do not carry the same set: only an attribute reference may leave the
// level empty, and only a collection reference is restricted to the two levels a span holds many of.
func TestPublishedLevelsMatchTheDomain(t *testing.T) {
	var declared []string
	for _, level := range levels {
		declared = append(declared, string(level))
	}

	assert.ElementsMatch(t, declared, publishedEnum(t, "jaeger.expression.v1.FieldReference", "level"))
	assert.ElementsMatch(t, append([]string{""}, declared...),
		publishedEnum(t, "jaeger.expression.v1.AttributeReference", "level"),
		"an empty level is the unqualified span-or-resource search")

	nested := publishedEnum(t, "jaeger.expression.v1.NestedReference", "level")
	assert.ElementsMatch(t, []string{string(LevelEvent), string(LevelLink)}, nested,
		"only events and links are collections to quantify over")
	assert.Subset(t, declared, nested)
}

func TestPublishedOperatorsMatchTheDomain(t *testing.T) {
	var declared []string
	for _, op := range operators {
		declared = append(declared, string(op))
	}
	assert.ElementsMatch(t, declared, publishedEnum(t, "jaeger.expression.v1.Call", "op"))
}

// TestPublishedValueTypesMatchTheDomain covers both places a type is declared. Each also accepts
// the empty value, which means something in both: any type for a constant, and "the field opposite
// the list declares it" for a list.
func TestPublishedValueTypesMatchTheDomain(t *testing.T) {
	declared := []string{""}
	for _, valueType := range valueTypes {
		declared = append(declared, string(valueType))
	}

	assert.ElementsMatch(t, declared, publishedEnum(t, "jaeger.expression.v1.Scalar", "type"))
	assert.ElementsMatch(t, declared, publishedEnum(t, "jaeger.expression.v1.List", "type"))
}

// TestPublishedListIsNotEmpty pins the one cardinality rule the schema can state. The validator
// refuses an empty list too, because a protobuf or gRPC caller is not governed by this document.
func TestPublishedListIsNotEmpty(t *testing.T) {
	assert.EqualValues(t, 1, publishedProperty(t, "jaeger.expression.v1.List", "values")["minItems"])
}

// TestFieldsUseDeclaredLevels is the closest the field registry comes to a published check: a field
// name travels as a free string, so the document says nothing about which names exist, but the level
// a field is registered at has to be one the document declares.
func TestFieldsUseDeclaredLevels(t *testing.T) {
	declared := publishedEnum(t, "jaeger.expression.v1.FieldReference", "level")
	for _, field := range Fields() {
		assert.Contains(t, declared, string(field.Level), "field %s", field.Name)
	}
}
