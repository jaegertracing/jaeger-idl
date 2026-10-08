// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const filterCapabilitiesDoc = `
jaeger.api_v3.FilterCapabilities:
    properties:
        levels:
            type: array
            items:
                type: string
        operators:
            type: array
            items:
                type: string
`

func TestPublishFilterCapabilityEnums(t *testing.T) {
	var schemas yaml.Node
	if err := yaml.Unmarshal([]byte(filterCapabilitiesDoc), &schemas); err != nil {
		t.Fatal(err)
	}
	opDefs := []operatorDefinition{{name: "and"}, {name: "eq"}}
	if err := publishFilterCapabilityEnums(&schemas, levelDefs, opDefs); err != nil {
		t.Fatal(err)
	}
	for property, want := range map[string]string{levelsPropertyName: "span,event", operatorsPropertyName: "and,eq"} {
		enum := descend(&schemas, filterCapabilitiesMessageName, "properties", property, "items", "enum")
		if enum == nil {
			t.Fatalf("%s.items has no enum", property)
		}
		var names []string
		for _, n := range enum.Content {
			names = append(names, n.Value)
		}
		if got := strings.Join(names, ","); got != want {
			t.Errorf("%s enum = %q, want %q", property, got, want)
		}
	}
}

func TestPublishFilterCapabilityEnums_Refuses(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want string
	}{
		{
			name: "no FilterCapabilities",
			doc:  "jaeger.api_v3.Other:\n    properties: {}\n",
			want: "no " + filterCapabilitiesMessageName + "." + levelsPropertyName,
		},
		{
			name: "operators without items",
			doc:  strings.Replace(filterCapabilitiesDoc, "        operators:\n            type: array\n            items:\n                type: string\n", "        operators:\n            type: string\n", 1),
			want: "no " + filterCapabilitiesMessageName + "." + operatorsPropertyName,
		},
		{
			name: "hand-written enum",
			doc:  strings.Replace(filterCapabilitiesDoc, "items:\n                type: string\n        operators", "items:\n                type: string\n                enum: [span]\n        operators", 1),
			want: "already publishes an enum",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var schemas yaml.Node
			if err := yaml.Unmarshal([]byte(test.doc), &schemas); err != nil {
				t.Fatal(err)
			}
			err := publishFilterCapabilityEnums(&schemas, levelDefs, []operatorDefinition{{name: "and"}})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want an error containing %q", err, test.want)
			}
		})
	}
}
