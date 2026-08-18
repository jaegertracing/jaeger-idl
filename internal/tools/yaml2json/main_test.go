// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	var out strings.Builder
	in := strings.NewReader("components:\n  schemas:\n    Call:\n      enum:\n        - ''\n        - eq\n")
	if err := run(in, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `{
  "components": {
    "schemas": {
      "Call": {
        "enum": [
          "",
          "eq"
        ]
      }
    }
  }
}
`
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestRunRefusesWhatIsNotYAML(t *testing.T) {
	var out strings.Builder
	if err := run(strings.NewReader("\tnot: yaml"), &out); err == nil {
		t.Error("expected an error for a document YAML cannot read")
	}
}

func TestRunRefusesAnUnreadableInput(t *testing.T) {
	var out strings.Builder
	if err := run(failingReader{}, &out); err == nil {
		t.Error("expected an error when the document cannot be read")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errRead
}

var errRead = &readError{}

type readError struct{}

func (*readError) Error() string { return "cannot read" }
