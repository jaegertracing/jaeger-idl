// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The closed vocabularies are written down twice: as enum annotations on the proto fields, which
// are published in the OpenAPI schema, and as the slices this package validates against. The tests
// here compare the two, so a level or an operator added to one and forgotten in the other cannot
// pass — it would otherwise be schema-valid and refused by the validator, or accepted by the
// validator and unrepresentable on the wire.
//
// They read the annotations rather than the published document because the annotations are the
// authority, and because CI regenerates the document from them and diffs it, which ties the
// published schema to whatever these tests just read.
const wireProtoPath = "../../../proto/expression/v1/expression.proto"

var (
	messageLine = regexp.MustCompile(`^message (\w+) \{`)
	valueInEnum = regexp.MustCompile(`\{yaml: "([^"]*)"\}`)
)

// wireEnum reads the values a proto field's OpenAPI annotation enumerates. The annotation carries
// each value as the YAML text that produces it, so the empty value is written as a pair of quotes;
// this reads it back as the empty string it publishes.
func wireEnum(t *testing.T, message, field string) []string {
	annotation := wireAnnotation(t, message, field)

	var values []string
	for _, match := range valueInEnum.FindAllStringSubmatch(annotation, -1) {
		if match[1] == `''` {
			values = append(values, "")
			continue
		}
		values = append(values, match[1])
	}
	require.NotEmpty(t, values, "%s.%s enumerates its values", message, field)
	return values
}

// wireAnnotation returns the text of the annotation on one field of one message, from the opening
// bracket to the closing one.
func wireAnnotation(t *testing.T, message, field string) string {
	raw, err := os.ReadFile(wireProtoPath)
	require.NoError(t, err)

	var (
		current    string
		annotation []string
		collecting bool
	)
	for _, line := range strings.Split(string(raw), "\n") {
		if match := messageLine.FindStringSubmatch(line); match != nil {
			current = match[1]
			continue
		}
		if collecting {
			annotation = append(annotation, line)
			if strings.Contains(line, "];") {
				break
			}
			continue
		}
		if current != message || !strings.Contains(line, " "+field+" = ") {
			continue
		}
		annotation = append(annotation, line)
		collecting = !strings.Contains(line, "];")
	}
	require.NotEmpty(t, annotation, "%s declares %s with an annotation", message, field)
	return strings.Join(annotation, "\n")
}

// TestWireLevelsMatchTheDomain compares the three reference terms against the levels this package
// accepts. The three do not carry the same set: only an attribute reference may leave the level
// empty, and only a collection reference is restricted to the two levels a span holds many of.
func TestWireLevelsMatchTheDomain(t *testing.T) {
	var declared []string
	for _, level := range levels {
		declared = append(declared, string(level))
	}

	assert.ElementsMatch(t, declared, wireEnum(t, "FieldReference", "level"))
	assert.ElementsMatch(t, append([]string{""}, declared...), wireEnum(t, "AttributeReference", "level"),
		"an empty level is the unqualified span-or-resource search")

	nested := wireEnum(t, "NestedReference", "level")
	assert.ElementsMatch(t, []string{string(LevelEvent), string(LevelLink)}, nested,
		"only events and links are collections to quantify over")
	assert.Subset(t, declared, nested)
}

func TestWireOperatorsMatchTheDomain(t *testing.T) {
	var declared []string
	for _, op := range operators {
		declared = append(declared, string(op))
	}
	assert.ElementsMatch(t, declared, wireEnum(t, "Call", "op"))
}

// TestWireValueTypesMatchTheDomain covers both places a type is declared. Each also accepts the
// empty value, which means something in both: any type for a constant, and "the field opposite the
// list declares it" for a list.
func TestWireValueTypesMatchTheDomain(t *testing.T) {
	declared := []string{""}
	for _, valueType := range valueTypes {
		declared = append(declared, string(valueType))
	}

	assert.ElementsMatch(t, declared, wireEnum(t, "Scalar", "type"))
	assert.ElementsMatch(t, declared, wireEnum(t, "List", "type"))
}

// TestWireListIsNotEmpty pins the one cardinality rule the schema can state. The validator refuses
// an empty list too, because a protobuf or gRPC caller is not governed by that schema.
func TestWireListIsNotEmpty(t *testing.T) {
	assert.Contains(t, wireAnnotation(t, "List", "values"), "min_items: 1")
}

// TestFieldsUseDeclaredLevels is the closest the field registry comes to a wire check: a field name
// travels as a free string, so the wire says nothing about which names exist, but the level a field
// is registered at has to be one the wire declares.
func TestFieldsUseDeclaredLevels(t *testing.T) {
	declared := wireEnum(t, "FieldReference", "level")
	for _, field := range Fields() {
		assert.Contains(t, declared, string(field.Level), "field %s", field.Name)
	}
}
