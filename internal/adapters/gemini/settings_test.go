package gemini

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_SettingsPreservesNativeModelSiblings(t *testing.T) {
	testutil.TempCwd(t)
	if err := os.MkdirAll(".gemini", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaultSettingsFile, []byte(`{"model":{"name":"old","maxSessionTurns":15},"ui":{"theme":"dark"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{"model": "gemini-2.5-pro", "x-gemini": map[string]any{"model": map[string]any{"name": "native-model", "compressionThreshold": 0.6}, "general": map[string]any{"previewFeatures": true}}}}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(defaultSettingsFile)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	model := doc["model"].(map[string]any)
	if model["name"] != "native-model" || model["maxSessionTurns"] != float64(15) || model["compressionThreshold"] != 0.6 {
		t.Errorf("model = %#v", model)
	}
	if doc["ui"] == nil || doc["general"] == nil {
		t.Errorf("missing settings: %s", raw)
	}
}

func TestEmit_SettingsModelAndPermissionsCoverage(t *testing.T) {
	testutil.TempCwd(t)
	notes := swapNoteWarner(t)
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{"model": "gemini-2.5-pro", "permissions": map[string]any{"deny": []any{"Bash(rm *)"}}}}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(defaultSettingsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"name": "gemini-2.5-pro"`) || strings.Contains(string(raw), `"permissions"`) {
		t.Errorf("settings: %s", raw)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(notes.String(), "permissions") {
		t.Errorf("missing coverage: %s", notes.String())
	}
}
