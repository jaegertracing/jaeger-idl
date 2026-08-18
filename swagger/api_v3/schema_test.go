// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3schema

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// schema reads the published specification. It is generated from the protos, so what these tests
// guard is the annotations that carry a rule the generator cannot infer: a default value that means
// something, and a list that cannot be empty. Losing one of those leaves a schema that rejects a
// request the server accepts, which nothing else here would notice.
func schema(t *testing.T) map[string]any {
	raw, err := os.ReadFile("query_service.openapi.yaml")
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	return doc
}

func property(t *testing.T, message, field string) map[string]any {
	schemas := schema(t)["components"].(map[string]any)["schemas"].(map[string]any)
	definition, ok := schemas[message].(map[string]any)
	require.True(t, ok, "the schema defines %s", message)
	properties := definition["properties"].(map[string]any)
	value, ok := properties[field].(map[string]any)
	require.True(t, ok, "%s defines %s", message, field)
	return value
}

func enum(t *testing.T, message, field string) []string {
	var out []string
	for _, value := range property(t, message, field)["enum"].([]any) {
		text, ok := value.(string)
		require.True(t, ok, "%s.%s enumerates strings, got %#v", message, field, value)
		out = append(out, text)
	}
	return out
}

// TestEmptyValuesAreInTheEnums pins the three places where an empty value is a value: an
// unqualified attribute reference, a constant of any type, and a list whose element type comes
// from the field it is compared against. A generic JSON client that writes default-valued fields
// rather than omitting them sends "" for each of these.
func TestEmptyValuesAreInTheEnums(t *testing.T) {
	assert.Contains(t, enum(t, "jaeger.expression.v1.AttributeReference", "level"), "",
		"an empty level is the unqualified span-or-resource search")
	assert.Contains(t, enum(t, "jaeger.expression.v1.Scalar", "type"), "",
		"an empty type matches a value of any type")
	assert.Contains(t, enum(t, "jaeger.expression.v1.List", "type"), "",
		"an empty list type is valid where a built-in field supplies one")
}

// TestRequiredLevelsHaveNoEmptyValue is the other half: the two references that must name a level
// do not accept one that names nothing.
func TestRequiredLevelsHaveNoEmptyValue(t *testing.T) {
	assert.NotContains(t, enum(t, "jaeger.expression.v1.FieldReference", "level"), "")
	assert.NotContains(t, enum(t, "jaeger.expression.v1.NestedReference", "level"), "")
}

func TestMembershipListIsNotEmpty(t *testing.T) {
	assert.Equal(t, 1, property(t, "jaeger.expression.v1.List", "values")["minItems"],
		"membership in nothing matches nothing, so the validator refuses an empty list")
}
