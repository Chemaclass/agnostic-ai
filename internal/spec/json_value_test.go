package spec

import (
	"math"
	"strings"
	"testing"
)

func TestNonJSONValue_NamesTheFieldJSONCannotHold(t *testing.T) {
	for want, meta := range map[string]map[string]any{
		"timeout":           {"command": "npx", "timeout": math.NaN()},
		"x-amp.timeout":     {"x-amp": map[string]any{"timeout": math.Inf(1)}},
		"args[1]":           {"args": []any{"a", math.Inf(-1)}},
		"env.list[0].limit": {"env": map[string]any{"list": []any{map[string]any{"limit": math.NaN()}}}},
	} {
		if got, _, ok := NonJSONValue(meta); !ok || got != want {
			t.Errorf("NonJSONValue(%v) = %q, %v; want %q", meta, got, ok, want)
		}
	}
	if _, _, ok := NonJSONValue(map[string]any{"timeout": 30.5, "args": []any{"x"}}); ok {
		t.Error("a plain value was flagged")
	}
}

func TestCheckMCPJSONValues_NamesTheSpecAndServer(t *testing.T) {
	err := CheckMCPJSONValues([]Entry{{Kind: KindMCP, Name: "gh", Path: "mcps/gh.yaml", Meta: map[string]any{"timeout": math.NaN()}}}, "amp")
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{"mcps/gh.yaml", `"gh"`, "timeout"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %s", err, want)
		}
	}
}

// yaml.v3 decodes a map with a non-string key as map[any]any; a value
// JSON cannot hold under one is still found.
func TestNonJSONValue_LooksInsideMapsWithNonStringKeys(t *testing.T) {
	meta := map[string]any{"x-amp": map[string]any{"options": map[any]any{true: math.NaN()}}}
	if got, _, ok := NonJSONValue(meta); !ok || got != "x-amp.options.true" {
		t.Errorf("NonJSONValue = %q, %v; want x-amp.options.true", got, ok)
	}
}

// Another target's x- block does not reach this target, so it does not
// fail it.
func TestCheckMCPJSONValues_IgnoresOtherTargetsBlocks(t *testing.T) {
	mcps := []Entry{{Kind: KindMCP, Name: "gh", Path: "mcps/gh.yaml", Meta: map[string]any{"command": "npx", "x-amp": map[string]any{"timeout": math.NaN()}}}}
	if err := CheckMCPJSONValues(mcps, "claude"); err != nil {
		t.Errorf("claude failed on an x-amp value: %v", err)
	}
	if err := CheckMCPJSONValues(mcps, "amp"); err == nil {
		t.Error("amp passed with its own .nan")
	}
}
