//go:build js && wasm

package main

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestParseEntry_ReadsYAMLKindsWithoutFrontmatter(t *testing.T) {
	for _, kind := range []spec.Kind{spec.KindHook, spec.KindMCP, spec.KindSettings, spec.KindEnvironment} {
		entry, err := parseEntry(kind, "name: sample\nmodel: opus\n")
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if entry.Name != "sample" || entry.Meta["model"] != "opus" {
			t.Errorf("%s spec must parse as YAML, got name %q meta %v", kind, entry.Name, entry.Meta)
		}
	}
}
