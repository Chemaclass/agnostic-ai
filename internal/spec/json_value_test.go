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
	err := CheckMCPJSONValues([]Entry{{Kind: KindMCP, Name: "gh", Path: "mcps/gh.yaml", Meta: map[string]any{"timeout": math.NaN()}}})
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{"mcps/gh.yaml", `"gh"`, "timeout"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %s", err, want)
		}
	}
}
