// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"reflect"
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
