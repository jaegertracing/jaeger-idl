// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"gopkg.in/yaml.v3"
)

// vocabularyFile mirrors proto/expression/v1/vocabulary.proto closely enough for the reader:
// the definition message, its two enums, and the extension on FieldOptions.
func vocabularyFile() *descriptorpb.FileDescriptorProto {
	enumValue := func(name string, number int32) *descriptorpb.EnumValueDescriptorProto {
		return &descriptorpb.EnumValueDescriptorProto{Name: proto.String(name), Number: proto.Int32(number)}
	}
	return &descriptorpb.FileDescriptorProto{
		Name:       proto.String("expression/v1/vocabulary.proto"),
		Package:    proto.String("jaeger.expression.v1"),
		Syntax:     proto.String("proto3"),
		Dependency: []string{"google/protobuf/descriptor.proto"},
		EnumType: []*descriptorpb.EnumDescriptorProto{
			{Name: proto.String("Arity"), Value: []*descriptorpb.EnumValueDescriptorProto{
				enumValue("ARITY_UNSPECIFIED", 0), enumValue("ARITY_UNARY", 1), enumValue("ARITY_BINARY", 2), enumValue("ARITY_VARIADIC", 3),
			}},
			{Name: proto.String("Operand"), Value: []*descriptorpb.EnumValueDescriptorProto{
				enumValue("OPERAND_UNSPECIFIED", 0), enumValue("OPERAND_PREDICATE", 1), enumValue("OPERAND_REFERENCE", 2), enumValue("OPERAND_LIST", 7),
			}},
		},
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("OperatorDefinition"),
			Field: []*descriptorpb.FieldDescriptorProto{
				{Name: proto.String("name"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("name")},
				{Name: proto.String("description"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("description")},
				{Name: proto.String("arity"), Number: proto.Int32(3), Type: descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(), TypeName: proto.String(".jaeger.expression.v1.Arity"), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("arity")},
				{Name: proto.String("operands"), Number: proto.Int32(4), Type: descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(), TypeName: proto.String(".jaeger.expression.v1.Operand"), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(), JsonName: proto.String("operands")},
			},
		}},
		Extension: []*descriptorpb.FieldDescriptorProto{{
			Name:     proto.String("operators"),
			Number:   proto.Int32(51001),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
			TypeName: proto.String(".jaeger.expression.v1.OperatorDefinition"),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
			Extendee: proto.String(".google.protobuf.FieldOptions"),
			JsonName: proto.String("operators"),
		}},
	}
}

// expressionFile is a Call message whose op field carries the given definitions, each written
// as {name, arity, operands, description}.
func expressionFile(t *testing.T, vocabulary *descriptorpb.FileDescriptorProto, defs ...[]any) *descriptorpb.FileDescriptorProto {
	t.Helper()
	files, err := protodesc.NewFiles(&descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{
		protodesc.ToFileDescriptorProto(descriptorpb.File_google_protobuf_descriptor_proto), vocabulary,
	}})
	if err != nil {
		t.Fatal(err)
	}
	extDesc, err := files.FindDescriptorByName(operatorsExtName)
	if err != nil {
		t.Fatal(err)
	}
	ext := dynamicpb.NewExtensionType(extDesc.(protoreflect.ExtensionDescriptor))
	defDesc := ext.TypeDescriptor().Message()

	options := dynamicpb.NewMessage((&descriptorpb.FieldOptions{}).ProtoReflect().Descriptor())
	list := options.Mutable(ext.TypeDescriptor()).List()
	for _, def := range defs {
		m := dynamicpb.NewMessage(defDesc)
		fields := defDesc.Fields()
		m.Set(fields.ByName("name"), protoreflect.ValueOfString(def[0].(string)))
		m.Set(fields.ByName("arity"), protoreflect.ValueOfEnum(fields.ByName("arity").Enum().Values().ByName(protoreflect.Name(def[1].(string))).Number()))
		operands := m.Mutable(fields.ByName("operands")).List()
		for _, operand := range def[2].([]string) {
			operands.Append(protoreflect.ValueOfEnum(fields.ByName("operands").Enum().Values().ByName(protoreflect.Name(operand)).Number()))
		}
		m.Set(fields.ByName("description"), protoreflect.ValueOfString(def[3].(string)))
		list.Append(protoreflect.ValueOfMessage(m))
	}
	raw, err := proto.Marshal(options)
	if err != nil {
		t.Fatal(err)
	}
	// Decoding without the extension registered leaves it in the unknown fields, which is how
	// protoc's descriptor set carries a custom option too.
	var fieldOptions descriptorpb.FieldOptions
	if err := proto.Unmarshal(raw, &fieldOptions); err != nil {
		t.Fatal(err)
	}
	return &descriptorpb.FileDescriptorProto{
		Name:       proto.String("expression/v1/expression.proto"),
		Package:    proto.String("jaeger.expression.v1"),
		Syntax:     proto.String("proto3"),
		Dependency: []string{"expression/v1/vocabulary.proto"},
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Call"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name: proto.String("op"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("op"), Options: &fieldOptions,
			}},
		}},
	}
}

func descriptorSet(t *testing.T, files ...*descriptorpb.FileDescriptorProto) []byte {
	t.Helper()
	set := &descriptorpb.FileDescriptorSet{File: append([]*descriptorpb.FileDescriptorProto{
		protodesc.ToFileDescriptorProto(descriptorpb.File_google_protobuf_descriptor_proto),
	}, files...)}
	raw, err := proto.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestReadOperatorDefinitions(t *testing.T) {
	vocabulary := vocabularyFile()
	set := descriptorSet(t, vocabulary, expressionFile(t, vocabulary,
		[]any{"and", "ARITY_VARIADIC", []string{"OPERAND_PREDICATE"}, "Holds when every predicate holds."},
		[]any{"in", "ARITY_BINARY", []string{"OPERAND_REFERENCE", "OPERAND_LIST"}, "Holds when the value is listed."},
	))
	defs, err := readOperatorDefinitions(set)
	if err != nil {
		t.Fatal(err)
	}
	want := []operatorDefinition{
		{name: "and", description: "Holds when every predicate holds.", arity: "ARITY_VARIADIC", operands: []string{"OPERAND_PREDICATE"}},
		{name: "in", description: "Holds when the value is listed.", arity: "ARITY_BINARY", operands: []string{"OPERAND_REFERENCE", "OPERAND_LIST"}},
	}
	if len(defs) != len(want) {
		t.Fatalf("got %d definitions, want %d", len(defs), len(want))
	}
	for i := range want {
		if defs[i].name != want[i].name || defs[i].description != want[i].description || defs[i].arity != want[i].arity ||
			strings.Join(defs[i].operands, ",") != strings.Join(want[i].operands, ",") {
			t.Errorf("definition %d = %+v, want %+v", i, defs[i], want[i])
		}
	}
}

func TestReadOperatorDefinitions_Refuses(t *testing.T) {
	vocabulary := vocabularyFile()
	tests := []struct {
		name string
		set  []byte
		want string
	}{
		{name: "garbage", set: []byte{0xff, 0xff}, want: "decoding descriptor set"},
		{name: "no vocabulary", set: descriptorSet(t), want: "finding the " + operatorsExtName},
		{name: "no Call", set: descriptorSet(t, vocabulary), want: "finding the " + callMessageName},
		{name: "no definitions", set: descriptorSet(t, vocabulary, expressionFile(t, vocabulary)), want: "declares no operators"},
		{
			name: "nameless definition",
			set:  descriptorSet(t, vocabulary, expressionFile(t, vocabulary, []any{"", "ARITY_UNARY", []string{"OPERAND_PREDICATE"}, "x"})),
			want: "has no name",
		},
		{
			name: "unspecified arity",
			set:  descriptorSet(t, vocabulary, expressionFile(t, vocabulary, []any{"x", "ARITY_UNSPECIFIED", []string{"OPERAND_PREDICATE"}, "x"})),
			want: "has arity ARITY_UNSPECIFIED",
		},
		{
			name: "binary with one operand",
			set:  descriptorSet(t, vocabulary, expressionFile(t, vocabulary, []any{"x", "ARITY_BINARY", []string{"OPERAND_PREDICATE"}, "x"})),
			want: "lists 1 operand kinds, not 2",
		},
		{
			name: "variadic with two operands",
			set:  descriptorSet(t, vocabulary, expressionFile(t, vocabulary, []any{"x", "ARITY_VARIADIC", []string{"OPERAND_PREDICATE", "OPERAND_PREDICATE"}, "x"})),
			want: "lists 2 operand kinds, not 1",
		},
		{
			name: "operand kind without a noun",
			set:  descriptorSet(t, vocabulary, expressionFile(t, vocabulary, []any{"x", "ARITY_UNARY", []string{"OPERAND_UNSPECIFIED"}, "x"})),
			want: "operand kind OPERAND_UNSPECIFIED",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := readOperatorDefinitions(test.set)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want an error containing %q", err, test.want)
			}
		})
	}
}

func TestOperandPhrase(t *testing.T) {
	tests := []struct {
		def  operatorDefinition
		want string
	}{
		{operatorDefinition{arity: "ARITY_VARIADIC", operands: []string{"OPERAND_PREDICATE"}}, "two or more predicates"},
		{operatorDefinition{arity: "ARITY_UNARY", operands: []string{"OPERAND_PREDICATE"}}, "a predicate"},
		{operatorDefinition{arity: "ARITY_UNARY", operands: []string{"OPERAND_ATTRIBUTE_REFERENCE"}}, "an attribute reference"},
		{operatorDefinition{arity: "ARITY_BINARY", operands: []string{"OPERAND_VALUE", "OPERAND_VALUE"}}, "two values"},
		{operatorDefinition{arity: "ARITY_BINARY", operands: []string{"OPERAND_COLLECTION_REFERENCE", "OPERAND_PREDICATE"}}, "a collection reference and a predicate"},
	}
	for _, test := range tests {
		if got := operandPhrase(test.def); got != test.want {
			t.Errorf("operandPhrase(%+v) = %q, want %q", test.def, got, test.want)
		}
	}
}

func TestPublishOperators(t *testing.T) {
	doc := `
jaeger.expression.v1.Call:
    properties:
        op:
            type: string
            description: op names the operator.
        args:
            type: array
`
	var schemas yaml.Node
	if err := yaml.Unmarshal([]byte(doc), &schemas); err != nil {
		t.Fatal(err)
	}
	defs := []operatorDefinition{
		{name: "not", description: "Holds when the predicate does not.", arity: "ARITY_UNARY", operands: []string{"OPERAND_PREDICATE"}},
		{name: "exists", description: "Holds when the value is present.", arity: "ARITY_UNARY", operands: []string{"OPERAND_REFERENCE"}},
	}
	if err := publishOperators(&schemas, defs); err != nil {
		t.Fatal(err)
	}
	op := descend(&schemas, callMessageName, "properties", opFieldName)

	var names []string
	for _, n := range findNode(op, "enum").Content {
		names = append(names, n.Value)
	}
	if got := strings.Join(names, ","); got != "not,exists" {
		t.Errorf("enum = %q, want not,exists", got)
	}
	description := findNode(op, "description").Value
	for _, want := range []string{
		"op names the operator.\n\n",
		"- `not` (a predicate): Holds when the predicate does not.",
		"- `exists` (a reference): Holds when the value is present.",
	} {
		if !strings.Contains(description, want) {
			t.Errorf("description lacks %q:\n%s", want, description)
		}
	}

	// A second run replaces the enum and the rendered list rather than appending to either.
	if err := publishOperators(&schemas, defs[:1]); err != nil {
		t.Fatal(err)
	}
	if got := len(findNode(op, "enum").Content); got != 1 {
		t.Errorf("enum has %d entries after republishing one operator", got)
	}
	description = findNode(op, "description").Value
	if strings.Count(description, renderedHeading) != 1 || strings.Contains(description, "`exists`") {
		t.Errorf("republishing did not replace the rendered list:\n%s", description)
	}

	var noOp yaml.Node
	if err := yaml.Unmarshal([]byte("jaeger.expression.v1.Call:\n    properties: {}\n"), &noOp); err != nil {
		t.Fatal(err)
	}
	if err := publishOperators(&noOp, defs); err == nil {
		t.Error("publishing into a document without the op property should fail")
	}
}

func TestPublishOperators_WithoutDescription(t *testing.T) {
	var schemas yaml.Node
	if err := yaml.Unmarshal([]byte("jaeger.expression.v1.Call:\n    properties:\n        op:\n            type: string\n"), &schemas); err != nil {
		t.Fatal(err)
	}
	defs := []operatorDefinition{{name: "not", description: "Holds when the predicate does not.", arity: "ARITY_UNARY", operands: []string{"OPERAND_PREDICATE"}}}
	if err := publishOperators(&schemas, defs); err != nil {
		t.Fatal(err)
	}
	op := descend(&schemas, callMessageName, "properties", opFieldName)
	if description := findNode(op, "description"); description == nil || !strings.HasPrefix(description.Value, "Operators, each with") {
		t.Errorf("description was not created from the rendered definitions: %v", description)
	}
}
