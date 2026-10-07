// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// The operator vocabulary is declared as data on the `op` field of Call, through the
// (jaeger.expression.v1.operators) option that proto/expression/v1/vocabulary.proto defines.
// gnostic copies a field's comment into the OpenAPI document but knows nothing of a custom
// option, so the enum and the per-operator descriptions are written here from the compiled
// descriptors instead.
const (
	callMessageName  = "jaeger.expression.v1.Call"
	opFieldName      = "op"
	operatorsExtName = "jaeger.expression.v1.operators"
	callSchemaName   = callMessageName
)

// operatorDefinition is one entry of the vocabulary, read from the option.
type operatorDefinition struct {
	name        string
	description string
	arity       string
	operands    []string
}

// readOperatorDefinitions returns the operator definitions on Call.op from a serialized
// FileDescriptorSet, in the order the proto lists them.
func readOperatorDefinitions(descriptorSet []byte) ([]operatorDefinition, error) {
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(descriptorSet, &set); err != nil {
		return nil, fmt.Errorf("decoding descriptor set: %w", err)
	}
	files, err := protodesc.NewFiles(&set)
	if err != nil {
		return nil, fmt.Errorf("resolving descriptor set: %w", err)
	}
	extDesc, err := files.FindDescriptorByName(operatorsExtName)
	if err != nil {
		return nil, fmt.Errorf("finding the %s option: %w", operatorsExtName, err)
	}
	ext := dynamicpb.NewExtensionType(extDesc.(protoreflect.ExtensionDescriptor))
	callDesc, err := files.FindDescriptorByName(callMessageName)
	if err != nil {
		return nil, fmt.Errorf("finding the %s message: %w", callMessageName, err)
	}
	opField := callDesc.(protoreflect.MessageDescriptor).Fields().ByName(opFieldName)
	if opField == nil {
		return nil, fmt.Errorf("%s has no %s field", callMessageName, opFieldName)
	}

	// The descriptor set was decoded without the extension registered, so the option sits in
	// the unknown fields of the FieldOptions. Re-decoding it with the extension type known is
	// what turns those bytes into definitions.
	raw, err := proto.Marshal(opField.Options())
	if err != nil {
		return nil, fmt.Errorf("re-encoding the %s options: %w", opFieldName, err)
	}
	var types protoregistry.Types
	if err := types.RegisterExtension(ext); err != nil {
		return nil, err
	}
	options := dynamicpb.NewMessage((&descriptorpb.FieldOptions{}).ProtoReflect().Descriptor())
	if err := (proto.UnmarshalOptions{Resolver: &types}).Unmarshal(raw, options); err != nil {
		return nil, fmt.Errorf("decoding the %s options: %w", opFieldName, err)
	}
	list := options.Get(ext.TypeDescriptor()).List()
	if list.Len() == 0 {
		return nil, fmt.Errorf("%s.%s declares no operators", callMessageName, opFieldName)
	}

	defs := make([]operatorDefinition, 0, list.Len())
	for i := range list.Len() {
		m := list.Get(i).Message()
		fields := m.Descriptor().Fields()
		def := operatorDefinition{
			name:        m.Get(fields.ByName("name")).String(),
			description: m.Get(fields.ByName("description")).String(),
			arity:       enumName(fields.ByName("arity"), m.Get(fields.ByName("arity")).Enum()),
		}
		operands := m.Get(fields.ByName("operands")).List()
		for j := range operands.Len() {
			def.operands = append(def.operands, enumName(fields.ByName("operands"), operands.Get(j).Enum()))
		}
		if def.name == "" {
			return nil, fmt.Errorf("operator definition %d has no name", i)
		}
		defs = append(defs, def)
	}
	return defs, nil
}

func enumName(field protoreflect.FieldDescriptor, number protoreflect.EnumNumber) string {
	value := field.Enum().Values().ByNumber(number)
	if value == nil {
		return fmt.Sprintf("%d", number)
	}
	return string(value.Name())
}

// operandNouns maps each Operand enum value to the phrase the rendered definition uses.
var operandNouns = map[string]string{
	"OPERAND_PREDICATE":            "predicate",
	"OPERAND_REFERENCE":            "reference",
	"OPERAND_ATTRIBUTE_REFERENCE":  "attribute reference",
	"OPERAND_COLLECTION_REFERENCE": "collection reference",
	"OPERAND_VALUE":                "value",
	"OPERAND_CONSTANT":             "constant",
	"OPERAND_LIST":                 "list",
}

// operandPhrase renders an operator's operands as the start of its definition, such as
// "two or more predicates" or "a reference and a list".
func operandPhrase(def operatorDefinition) string {
	nouns := make([]string, 0, len(def.operands))
	for _, operand := range def.operands {
		noun, ok := operandNouns[operand]
		if !ok {
			noun = strings.ToLower(strings.TrimPrefix(operand, "OPERAND_"))
		}
		nouns = append(nouns, noun)
	}
	if def.arity == "ARITY_VARIADIC" && len(nouns) == 1 {
		return "two or more " + plural(nouns[0])
	}
	if len(nouns) == 2 && nouns[0] == nouns[1] {
		return "two " + plural(nouns[0])
	}
	for i, noun := range nouns {
		nouns[i] = article(noun) + " " + noun
	}
	return strings.Join(nouns, " and ")
}

func plural(noun string) string {
	return noun + "s"
}

func article(noun string) string {
	if strings.ContainsRune("aeiou", rune(noun[0])) {
		return "an"
	}
	return "a"
}

// renderOperatorDefinitions renders the vocabulary as the Markdown list appended to the `op`
// description, one line per operator.
func renderOperatorDefinitions(defs []operatorDefinition) string {
	var b strings.Builder
	b.WriteString("Operators, each with the operands it takes:\n")
	for _, def := range defs {
		fmt.Fprintf(&b, "\n- `%s` (%s): %s", def.name, operandPhrase(def), def.description)
	}
	return b.String()
}
