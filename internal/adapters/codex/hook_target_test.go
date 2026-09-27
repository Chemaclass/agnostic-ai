package codex

import (
	"encoding/json"
	"os"
	"os/exec"
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

// Codex has no exec form. A bare interpreter would read the event JSON
// on stdin as its program, so the args fold into the command, quoted
// for the POSIX shell the export prefix already needs.
func TestEmit_ExecFormHookFoldsQuotedArgs(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "a", Meta: map[string]any{"event": "PreToolUse", "command": "bash", "args": []any{"guard.sh"}}},
		{Kind: spec.KindHook, Name: "b", Meta: map[string]any{"event": "PreToolUse", "command": "bash", "args": []any{"audit.sh"}, "commandWindows": "audit.ps1"}},
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
	var handlers []map[string]any
	for _, group := range doc.Hooks["PreToolUse"] {
		handlers = append(handlers, group.Hooks...)
	}
	if len(handlers) != 2 {
		t.Fatalf("specs that differ only in args must stay two entries:\n%s", raw)
	}
	if handlers[0]["command"] != "export AGNOSTIC_AI_TARGET=codex; bash 'guard.sh'" || handlers[0]["commandWindows"] != "bash 'guard.sh'" || handlers[0]["args"] != nil {
		t.Errorf("first handler = %v", handlers[0])
	}
	if handlers[1]["command"] != "export AGNOSTIC_AI_TARGET=codex; bash 'audit.sh'" || handlers[1]["commandWindows"] != "audit.ps1" {
		t.Errorf("second handler = %v", handlers[1])
	}
}

func TestEmit_FoldedArgsRunVerbatimInSh(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no POSIX shell")
	}
	dir := testutil.TempCwd(t)
	args := []any{"%s|%s|%s", "it's", "a b", "$HOME `x`"}
	entries := []spec.Entry{{Kind: spec.KindHook, Name: "p", Meta: map[string]any{"event": "Stop", "command": "printf", "args": args}}}
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
	command, _ := doc.Hooks["Stop"][0].Hooks[0]["command"].(string)
	out, err := exec.Command("sh", "-c", command).Output()
	if err != nil {
		t.Fatalf("sh -c %q: %v", command, err)
	}
	if got, want := string(out), "it's|a b|$HOME `x`"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}
