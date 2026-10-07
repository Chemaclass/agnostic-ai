package opencode

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func readInstructions(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile("opencode.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Instructions []string `json:"instructions"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Instructions
}

func TestEmit_ListsTheMemoryIndexesAndKeepsUserInstructions(t *testing.T) {
	testutil.TempCwd(t)
	if err := os.WriteFile("opencode.json", []byte(`{"instructions": ["CONTRIBUTING.md"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Builtins: []string{"memory"}}

	for range 2 {
		if err := New().Emit(emit.NewSession(), spec.Bundle{}, cfg, false); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"CONTRIBUTING.md", ".agnostic-ai/local/memory/MEMORY.md", ".agnostic-ai/memory/MEMORY.md"}
	if got := readInstructions(t); !slices.Equal(got, want) {
		t.Errorf("instructions = %v, want %v", got, want)
	}
}

func TestEmit_NoMemoryInstructionsWithoutTheBuiltin(t *testing.T) {
	testutil.TempCwd(t)

	if err := New().Emit(emit.NewSession(), spec.Bundle{}, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("opencode.json"); !os.IsNotExist(err) {
		t.Errorf("opencode.json written without the built-in: %v", err)
	}
}

func TestEmit_KeepsSettingsInstructionsBesideTheMemoryIndexes(t *testing.T) {
	testutil.TempCwd(t)
	settings := spec.Entry{Kind: spec.KindSettings, Name: "s", Meta: map[string]any{"x-opencode": map[string]any{"instructions": []any{"docs/custom.md"}}}}

	if err := New().Emit(emit.NewSession(), spec.Bundle{Settings: []spec.Entry{settings}}, &config.Config{Builtins: []string{"memory"}}, false); err != nil {
		t.Fatal(err)
	}
	got := readInstructions(t)
	for _, want := range []string{"docs/custom.md", ".agnostic-ai/local/memory/MEMORY.md", ".agnostic-ai/memory/MEMORY.md"} {
		if !slices.Contains(got, want) {
			t.Errorf("instructions = %v, missing %s", got, want)
		}
	}
}
