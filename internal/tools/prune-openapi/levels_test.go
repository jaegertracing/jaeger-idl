// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"gopkg.in/yaml.v3"
)

// fieldReferenceFile is a FieldReference message whose level field carries the given
// definitions, each written as {name, description}.
func fieldReferenceFile(t *testing.T, vocabulary *descriptorpb.FileDescriptorProto, defs ...[2]string) *descriptorpb.FileDescriptorProto {
	t.Helper()
	files, err := protodesc.NewFiles(&descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{
		protodesc.ToFileDescriptorProto(descriptorpb.File_google_protobuf_descriptor_proto), vocabulary,
	}})
	if err != nil {
		t.Fatal(err)
	}
	ext, err := dynamicpb.NewTypes(files).FindExtensionByName(levelsExtName)
	if err != nil {
		t.Fatal(err)
	}
	defDesc := ext.TypeDescriptor().Message()

	options := dynamicpb.NewMessage((&descriptorpb.FieldOptions{}).ProtoReflect().Descriptor())
	list := options.Mutable(ext.TypeDescriptor()).List()
	for _, def := range defs {
		m := dynamicpb.NewMessage(defDesc)
		fields := defDesc.Fields()
		m.Set(fields.ByName("name"), protoreflect.ValueOfString(def[0]))
		m.Set(fields.ByName("description"), protoreflect.ValueOfString(def[1]))
		list.Append(protoreflect.ValueOfMessage(m))
	}
	raw, err := proto.Marshal(options)
	if err != nil {
		t.Fatal(err)
	}
	var fieldOptions descriptorpb.FieldOptions
	if err := proto.Unmarshal(raw, &fieldOptions); err != nil {
		t.Fatal(err)
	}
	return &descriptorpb.FileDescriptorProto{
		Name:       proto.String("expression/v1/references.proto"),
		Package:    proto.String("jaeger.expression.v1"),
		Syntax:     proto.String("proto3"),
		Dependency: []string{"expression/v1/vocabulary.proto"},
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("FieldReference"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name: proto.String("level"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("level"), Options: &fieldOptions,
			}},
		}},
	}
}

func TestReadLevelDefinitions(t *testing.T) {
	vocabulary := vocabularyFile()
	set := descriptorSet(t, vocabulary, fieldReferenceFile(t, vocabulary,
		[2]string{"span", "The span's own attributes."},
		[2]string{"event", "The span's events."},
	))
	defs, err := readLevelDefinitions(set)
	if err != nil {
		t.Fatal(err)
	}
	want := []levelDefinition{
		{name: "span", description: "The span's own attributes."},
		{name: "event", description: "The span's events."},
	}
	if !reflect.DeepEqual(defs, want) {
		t.Errorf("got %+v, want %+v", defs, want)
	}
}

func TestReadLevelDefinitions_Refuses(t *testing.T) {
	vocabulary := vocabularyFile()
	tests := []struct {
		name string
		set  []byte
		want string
	}{
		{name: "no vocabulary", set: descriptorSet(t), want: "finding the " + levelsExtName},
		{name: "no FieldReference", set: descriptorSet(t, vocabulary), want: "finding the " + fieldReferenceMessageName},
		{name: "no definitions", set: descriptorSet(t, vocabulary, fieldReferenceFile(t, vocabulary)), want: "declares no levels"},
		{name: "nameless", set: descriptorSet(t, vocabulary, fieldReferenceFile(t, vocabulary, [2]string{"", "x"})), want: "has no name"},
		{name: "undescribed", set: descriptorSet(t, vocabulary, fieldReferenceFile(t, vocabulary, [2]string{"span", ""})), want: `"span" has no description`},
		{
			name: "duplicate",
			set:  descriptorSet(t, vocabulary, fieldReferenceFile(t, vocabulary, [2]string{"span", "x"}, [2]string{"span", "y"})),
			want: `"span" is defined twice`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := readLevelDefinitions(test.set)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want an error containing %q", err, test.want)
			}
		})
	}
}

const referencesDoc = `
jaeger.expression.v1.FieldReference:
    properties:
        level:
            type: string
            description: level names the level.
jaeger.expression.v1.AttributeReference:
    properties:
        level:
            type: string
            enum: ["", span, event]
jaeger.expression.v1.NestedReference:
    properties:
        level:
            type: string
            enum: [event]
`

var levelDefs = []levelDefinition{
	{name: "span", description: "The span's own attributes."},
	{name: "event", description: "The span's events."},
}

func TestPublishLevels(t *testing.T) {
	var schemas yaml.Node
	if err := yaml.Unmarshal([]byte(referencesDoc), &schemas); err != nil {
		t.Fatal(err)
	}
	if err := publishLevels(&schemas, levelDefs); err != nil {
		t.Fatal(err)
	}

	field := descend(&schemas, fieldReferenceMessageName, "properties", levelFieldName)
	var names []string
	for _, n := range findNode(field, "enum").Content {
		names = append(names, n.Value)
	}
	if got := strings.Join(names, ","); got != "span,event" {
		t.Errorf("FieldReference enum = %q, want span,event", got)
	}
	for _, want := range []string{"level names the level.\n\n", "- `span`: The span's own attributes.", "- `event`: The span's events."} {
		if description := findNode(field, "description").Value; !strings.Contains(description, want) {
			t.Errorf("FieldReference description lacks %q:\n%s", want, description)
		}
	}

	// AttributeReference keeps its own enum, including the empty level, and documents the
	// defined ones it lists.
	attr := descend(&schemas, attributeReferenceMessageName, "properties", levelFieldName)
	names = names[:0]
	for _, n := range findNode(attr, "enum").Content {
		names = append(names, n.Value)
	}
	if got := strings.Join(names, ","); got != ",span,event" {
		t.Errorf("AttributeReference enum = %q, want the \"\",span,event it declared", got)
	}
	if description := findNode(attr, "description").Value; !strings.HasPrefix(description, "Levels:") || !strings.Contains(description, "- `span`:") {
		t.Errorf("AttributeReference description was not rendered from the listed levels:\n%s", description)
	}

	// NestedReference lists one level and is documented for that one only.
	nested := descend(&schemas, nestedReferenceMessageName, "properties", levelFieldName)
	if description := findNode(nested, "description").Value; strings.Contains(description, "`span`") || !strings.Contains(description, "- `event`:") {
		t.Errorf("NestedReference description should cover event only:\n%s", description)
	}
}

func TestPublishLevels_Refuses(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want string
	}{
		{
			name: "FieldReference with a hand-written enum",
			doc:  strings.Replace(referencesDoc, "description: level names the level.", "enum: [span]", 1),
			want: "already publishes an enum",
		},
		{
			name: "no FieldReference.level",
			doc:  "jaeger.expression.v1.FieldReference:\n    properties: {}\n",
			want: "no " + fieldReferenceMessageName,
		},
		{
			name: "subset without an enum",
			doc:  strings.Replace(referencesDoc, "enum: [event]", "description: x", 1),
			want: "publishes no enum",
		},
		{
			name: "subset naming an undefined level",
			doc:  strings.Replace(referencesDoc, "enum: [event]", "enum: [trace]", 1),
			want: `lists "trace", which the levels option does not define`,
		},
		{
			name: "subset listing a level twice",
			doc:  strings.Replace(referencesDoc, "enum: [event]", "enum: [event, event]", 1),
			want: `lists "event" twice`,
		},
		{
			name: "subset listing only the empty level",
			doc:  strings.Replace(referencesDoc, "enum: [event]", `enum: [""]`, 1),
			want: "lists no defined level",
		},
		{
			name: "no AttributeReference.level",
			doc:  strings.Replace(referencesDoc, "jaeger.expression.v1.AttributeReference:", "jaeger.expression.v1.Other:", 1),
			want: "no " + attributeReferenceMessageName,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var schemas yaml.Node
			if err := yaml.Unmarshal([]byte(test.doc), &schemas); err != nil {
				t.Fatal(err)
			}
			err := publishLevels(&schemas, levelDefs)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want an error containing %q", err, test.want)
			}
		})
	}
}
