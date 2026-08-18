// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// yaml2json republishes a YAML document as JSON, reading stdin and writing stdout.
//
// It exists so that the OpenAPI document generated for api_v3 is available in a form the standard
// library reads. The tests that compare the published closed value sets against the domain
// package live in the module that defines that package, and that module takes no dependency on a
// YAML parser: every direct dependency of the IDL reaches whoever imports it.
//
// Keys are sorted, because encoding/json sorts a map's keys, so the output is stable across runs
// and the generated file can be diffed in CI like any other generated artifact.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "yaml2json: %v\n", err)
		os.Exit(1)
	}
}

func run(in io.Reader, out io.Writer) error {
	raw, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("cannot read the document: %w", err)
	}
	var document any
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return fmt.Errorf("cannot read the document as YAML: %w", err)
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(document); err != nil {
		return fmt.Errorf("cannot write the document as JSON: %w", err)
	}
	return nil
}
