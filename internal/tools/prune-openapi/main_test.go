// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func mkParam(name string) *yaml.Node {
	return mappingNode(scalarNode("name", 0), scalarNode(name, 0))
}

func paramNames(seq *yaml.Node) []string {
	var out []string
	for _, p := range seq.Content {
		out = append(out, paramName(p))
	}
	return out
}

func TestCollapseFilterParams(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			// The two query.filter.* entries collapse into one query.filter (appended
			// last); query.filter_mode is a sibling that starts with the same prefix and
			// must survive — the bug this test guards against.
			name: "collapse the query.filter.* expansion, keep siblings",
			in:   []string{"query.service_name", "query.filter.op", "query.filter.args", "query.filter_mode", "query.raw_traces"},
			want: []string{"query.service_name", "query.filter_mode", "query.raw_traces", "query.filter"},
		},
		{
			name: "no filter params: list unchanged",
			in:   []string{"query.service_name", "query.filter_mode", "query.raw_traces"},
			want: []string{"query.service_name", "query.filter_mode", "query.raw_traces"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seq := &yaml.Node{Kind: yaml.SequenceNode}
			for _, n := range tt.in {
				seq.Content = append(seq.Content, mkParam(n))
			}
			collapseFilterParams(mappingNode(scalarNode("parameters", 0), seq))
			if got := paramNames(seq); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFilterQueryParamExample checks the published example against the terms the expression proto
// actually defines. The example is the one part of the specification a reader is most likely to
// copy, and nothing else would catch it going stale when a term is renamed: the generator does not
// read it, and the pruning tool writes it as an opaque string.
func TestFilterQueryParamExample(t *testing.T) {
	arms := map[string]bool{
		"attr": true, "field": true, "nested": true, "scalar": true, "list": true, "call": true,
	}

	var example string
	traverse(filterQueryParam(), func(node *yaml.Node) {
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "example" {
				example = node.Content[i+1].Value
			}
		}
	})
	if example == "" {
		t.Fatal("the filter parameter publishes no example")
	}

	var call struct {
		Op   string                       `json:"op"`
		Args []map[string]json.RawMessage `json:"args"`
	}
	if err := json.Unmarshal([]byte(example), &call); err != nil {
		t.Fatalf("the example is not a JSON Call: %v", err)
	}
	if call.Op == "" {
		t.Errorf("the example names no operator: %s", example)
	}
	if len(call.Args) == 0 {
		t.Errorf("the example passes no arguments: %s", example)
	}
	for _, arg := range call.Args {
		for term := range arg {
			if !arms[term] {
				t.Errorf("the example uses %q, which is not a term of jaeger.expression.v1.Expression: %s",
					term, example)
			}
		}
	}
	if strings.Contains(example, `"ref"`) {
		t.Errorf("the example uses the removed single reference term: %s", example)
	}
}
