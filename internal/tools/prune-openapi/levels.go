// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// The level vocabulary is declared as data on the `level` field of FieldReference, through the
// (jaeger.expression.v1.levels) option that proto/expression/v1/vocabulary.proto defines. That
// field accepts every level, so it carries the definitions; the `level` fields of
// AttributeReference and NestedReference accept a subset each and publish it as their own enum,
// which this tool checks against the definitions and documents from them.
const (
	fieldReferenceMessageName     = "jaeger.expression.v1.FieldReference"
	attributeReferenceMessageName = "jaeger.expression.v1.AttributeReference"
	nestedReferenceMessageName    = "jaeger.expression.v1.NestedReference"
	levelFieldName                = "level"
	levelsExtName                 = "jaeger.expression.v1.levels"
)

// levelDefinition is one entry of the vocabulary, read from the option.
type levelDefinition struct {
	name        string
	description string
}

// readLevelDefinitions returns the level definitions on FieldReference.level from a serialized
// FileDescriptorSet, in the order the proto lists them.
func readLevelDefinitions(descriptorSet []byte) ([]levelDefinition, error) {
	list, err := readFieldOption(descriptorSet, fieldReferenceMessageName, levelFieldName, levelsExtName)
	if err != nil {
		return nil, err
	}
	if list.Len() == 0 {
		return nil, fmt.Errorf("%s.%s declares no levels", fieldReferenceMessageName, levelFieldName)
	}

	defs := make([]levelDefinition, 0, list.Len())
	seen := make(map[string]bool, list.Len())
	for i := range list.Len() {
		m := list.Get(i).Message()
		fields := m.Descriptor().Fields()
		def := levelDefinition{
			name:        m.Get(fields.ByName("name")).String(),
			description: m.Get(fields.ByName("description")).String(),
		}
		if def.name == "" {
			return nil, fmt.Errorf("level definition %d: has no name", i)
		}
		if def.description == "" {
			return nil, fmt.Errorf("level definition %d: %q has no description", i, def.name)
		}
		if seen[def.name] {
			return nil, fmt.Errorf("level definition %d: %q is defined twice", i, def.name)
		}
		seen[def.name] = true
		defs = append(defs, def)
	}
	return defs, nil
}

// renderLevelDefinitions renders the given levels as the Markdown list appended to a `level`
// description, one line per level.
func renderLevelDefinitions(defs []levelDefinition) string {
	var b strings.Builder
	b.WriteString("Levels:\n")
	for _, def := range defs {
		fmt.Fprintf(&b, "\n- `%s`: %s", def.name, def.description)
	}
	return b.String()
}

// publishLevels writes the vocabulary into the three `level` properties. FieldReference.level
// gets its enum from the definitions, the way Call.op does. AttributeReference.level and
// NestedReference.level keep the enum each declares, since each accepts a subset, and every
// value in it has to be a defined level or, for an attribute reference, the empty level.
func publishLevels(schemasNode *yaml.Node, defs []levelDefinition) error {
	byName := make(map[string]levelDefinition, len(defs))
	names := make([]*yaml.Node, 0, len(defs))
	for _, def := range defs {
		byName[def.name] = def
		names = append(names, scalarNode(def.name, 0))
	}

	level := descend(schemasNode, fieldReferenceMessageName, "properties", levelFieldName)
	if level == nil {
		return fmt.Errorf("the document has no %s.%s property", fieldReferenceMessageName, levelFieldName)
	}
	if findNode(level, "enum") != nil {
		return fmt.Errorf("%s.%s already publishes an enum; the vocabulary is declared through the levels option alone", fieldReferenceMessageName, levelFieldName)
	}
	level.Content = append(level.Content, scalarNode("enum", 0), seqNode(names...))
	appendDescription(level, renderLevelDefinitions(defs))

	for _, message := range []string{attributeReferenceMessageName, nestedReferenceMessageName} {
		level := descend(schemasNode, message, "properties", levelFieldName)
		if level == nil {
			return fmt.Errorf("the document has no %s.%s property", message, levelFieldName)
		}
		enum := findNode(level, "enum")
		if enum == nil {
			return fmt.Errorf("%s.%s publishes no enum; a level field that accepts a subset declares it", message, levelFieldName)
		}
		var listed []levelDefinition
		for _, value := range enum.Content {
			if value.Value == "" {
				continue
			}
			def, ok := byName[value.Value]
			if !ok {
				return fmt.Errorf("%s.%s lists %q, which the levels option does not define", message, levelFieldName, value.Value)
			}
			listed = append(listed, def)
		}
		if len(listed) == 0 {
			return errors.New(message + "." + levelFieldName + " lists no defined level")
		}
		appendDescription(level, renderLevelDefinitions(listed))
	}
	return nil
}

// appendDescription adds rendered to the node's description, after a blank line when the node
// already has one.
func appendDescription(node *yaml.Node, rendered string) {
	if description := findNode(node, "description"); description != nil {
		description.Value = description.Value + "\n\n" + rendered
		description.Style = yaml.LiteralStyle
		return
	}
	node.Content = append(node.Content, scalarNode("description", 0), scalarNode(rendered, yaml.LiteralStyle))
}
