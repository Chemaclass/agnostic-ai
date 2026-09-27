package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Codex runs `command` through a POSIX shell on macOS and Linux, and
// `commandWindows` on Windows, where PowerShell or cmd has no `export`.
func TestEmit_HookCommandCarriesTheTarget(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "plain", Meta: map[string]any{"event": "PreToolUse", "command": "guard.sh"}},
		{Kind: spec.KindHook, Name: "windows", Meta: map[string]any{"event": "Stop", "command": "done.sh", "commandWindows": "done.ps1"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".codex/hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Hooks map[string][]struct {
			Hooks []map[string]any `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	plain := doc.Hooks["PreToolUse"][0].Hooks[0]
	if plain["command"] != "export AGNOSTIC_AI_TARGET=codex; guard.sh" {
		t.Errorf("command = %v", plain["command"])
	}
	if plain["commandWindows"] != "guard.sh" {
		t.Errorf("commandWindows = %v, want the command as declared", plain["commandWindows"])
	}
	windows := doc.Hooks["Stop"][0].Hooks[0]
	if windows["command"] != "export AGNOSTIC_AI_TARGET=codex; done.sh" || windows["commandWindows"] != "done.ps1" {
		t.Errorf("entry = %v", windows)
	}
}
