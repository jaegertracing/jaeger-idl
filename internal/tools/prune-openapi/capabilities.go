// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// jaeger.api_v3.FilterCapabilities reports which levels and operators a backend serves as two
// lists of strings. Each is a subset of a vocabulary the document already defines, so its items
// publish that vocabulary as an enum: a client then reads the lists with the same closed sets
// it writes a filter with, and the definitions stay on the fields that own them.
const (
	filterCapabilitiesMessageName = "jaeger.api_v3.FilterCapabilities"
	levelsPropertyName            = "levels"
	operatorsPropertyName         = "operators"
)

// publishFilterCapabilityEnums writes the level and operator names as the item enums of the two
// FilterCapabilities lists.
func publishFilterCapabilityEnums(schemasNode *yaml.Node, levelDefs []levelDefinition, opDefs []operatorDefinition) error {
	levels := make([]*yaml.Node, 0, len(levelDefs))
	for _, def := range levelDefs {
		levels = append(levels, scalarNode(def.name, 0))
	}
	operators := make([]*yaml.Node, 0, len(opDefs))
	for _, def := range opDefs {
		operators = append(operators, scalarNode(def.name, 0))
	}
	if err := publishItemsEnum(schemasNode, levelsPropertyName, levels); err != nil {
		return err
	}
	return publishItemsEnum(schemasNode, operatorsPropertyName, operators)
}

func publishItemsEnum(schemasNode *yaml.Node, property string, names []*yaml.Node) error {
	items := descend(schemasNode, filterCapabilitiesMessageName, "properties", property, "items")
	if items == nil {
		return fmt.Errorf("the document has no %s.%s array", filterCapabilitiesMessageName, property)
	}
	if findNode(items, "enum") != nil {
		return fmt.Errorf("%s.%s already publishes an enum; it is derived from the vocabulary alone", filterCapabilitiesMessageName, property)
	}
	items.Content = append(items.Content, scalarNode("enum", 0), seqNode(names...))
	return nil
}
