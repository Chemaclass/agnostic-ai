package goose

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_HooksWritesOpenPlugin(t *testing.T) {
	dir := testutil.TempCwd(t)
	hooks := []spec.Entry{
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
			"event": "PreToolUse", "matcher": "^developer__shell$", "command": "./scripts/guard.sh", "timeout": 12,
			"x-goose": map[string]any{"on_failure": "block"},
		}},
		{Kind: spec.KindHook, Name: "observe", Meta: map[string]any{
			"event": "PreToolUseResult", "command": "./scripts/observe.sh",
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(hooks), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(dir, ".agents/plugins/agnostic-ai/plugin.json")
	var manifest map[string]any
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["name"] != "agnostic-ai" {
		t.Errorf("manifest name = %#v", manifest["name"])
	}

	hooksPath := filepath.Join(dir, defaultHooksFile)
	var doc struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type      string `json:"type"`
				Command   string `json:"command"`
				Timeout   int    `json:"timeout"`
				OnFailure string `json:"on_failure"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	data, err = os.ReadFile(hooksPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	guard := doc.Hooks["PreToolUse"][0]
	if guard.Matcher != "^developer__shell$" || guard.Hooks[0].Type != "command" || guard.Hooks[0].Timeout != 12 || guard.Hooks[0].OnFailure != "block" {
		t.Errorf("guard hook = %#v", guard)
	}
	if _, ok := doc.Hooks["PreToolUseResult"]; !ok {
		t.Errorf("PreToolUseResult missing: %#v", doc.Hooks)
	}
}

func TestEmit_HooksFileOverrideMovesManifestWithPlugin(t *testing.T) {
	dir := testutil.TempCwd(t)
	cfg := &config.Config{Outputs: map[string]config.Output{"goose": {HooksFile: "custom/checks/hooks/hooks.json"}}}
	bundle := spec.NewBundle([]spec.Entry{{Kind: spec.KindHook, Name: "stop", Meta: map[string]any{"event": "Stop", "command": "true"}}})
	if err := New().Emit(emit.NewSession(), bundle, cfg, false); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"custom/checks/plugin.json", "custom/checks/hooks/hooks.json"} {
		if _, err := os.Stat(filepath.Join(dir, path)); err != nil {
			t.Errorf("missing %s: %v", path, err)
		}
	}
}

func TestEmit_HooksFileOverrideRequiresPluginLayout(t *testing.T) {
	testutil.TempCwd(t)
	cfg := &config.Config{Outputs: map[string]config.Output{"goose": {HooksFile: "custom/hooks.json"}}}
	bundle := spec.NewBundle([]spec.Entry{{Kind: spec.KindHook, Name: "stop", Meta: map[string]any{"event": "Stop", "command": "true"}}})
	err := New().Emit(emit.NewSession(), bundle, cfg, false)
	if err == nil || !strings.Contains(err.Error(), "must end in hooks/hooks.json") {
		t.Fatalf("Emit() error = %v, want plugin layout error", err)
	}
}

func TestEmit_OnFailureIsLimitedToPreToolUse(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	bundle := spec.NewBundle([]spec.Entry{{Kind: spec.KindHook, Name: "stop", Meta: map[string]any{
		"event": "Stop", "command": "true", "x-goose": map[string]any{"on_failure": "block"},
	}}})
	if err := New().Emit(emit.NewSession(), bundle, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, defaultHooksFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "on_failure") {
		t.Errorf("Stop hook must omit PreToolUse-only on_failure: %s", data)
	}
	if emit.PendingCoverageNotesCount() != 1 {
		t.Fatalf("expected one coverage note for misplaced on_failure")
	}
}
