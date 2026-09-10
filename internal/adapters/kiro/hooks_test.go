package kiro

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

// Hooks emit one JSON file per hook spec at `.kiro/hooks/<name>.json`
// (kiro.dev/docs/hooks/), each wrapping a `{version, hooks: [...]}`
// document. `event` becomes `trigger`; `command` becomes a `{"type":
// "command", "command": ...}` action.
func TestEmit_Hook_WritesOneFilePerHook(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "fmt-go",
			Meta: map[string]any{"event": "PostToolUse", "matcher": "Edit", "command": "gofmt -w", "timeout": 30},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, filepath.Join(dir, ".kiro/hooks/fmt-go.json"))
	var doc hooksFile
	if err := json.Unmarshal([]byte(got), &doc); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, got)
	}
	if doc.Version != "v1" {
		t.Errorf("expected version \"v1\", got %q", doc.Version)
	}
	if len(doc.Hooks) != 1 {
		t.Fatalf("expected one hook entry, got %d", len(doc.Hooks))
	}
	h := doc.Hooks[0]
	if h["name"] != "fmt-go" || h["trigger"] != "PostToolUse" || h["matcher"] != "Edit" || h["timeout"] != float64(30) {
		t.Errorf("unexpected entry: %+v", h)
	}
	action, _ := h["action"].(map[string]any)
	if action["type"] != "command" || action["command"] != "gofmt -w" {
		t.Errorf("unexpected action: %+v", action)
	}
	if _, ok := h["enabled"]; ok {
		t.Errorf("expected no enabled key for a hook that is not disabled, got %v", h["enabled"])
	}
}

// disabled: true maps to Kiro's own `enabled: false`, mirroring the
// disabled/enabled convention this adapter already uses for MCP
// entries. The vendor default (enabled) needs no explicit key.
func TestEmit_Hook_DisabledWritesEnabledFalse(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "off",
			Meta: map[string]any{"event": "Stop", "command": "echo done", "disabled": true},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".kiro/hooks/off.json"))
	if !strings.Contains(got, `"enabled": false`) {
		t.Errorf("expected \"enabled\": false, got:\n%s", got)
	}
}

// A `command:` list produces one hooks[] entry per command in the same
// file, `name` suffixed `-2`, `-3`, ... past the first so entries
// sharing a file stay unique.
func TestEmit_Hook_MultipleCommandsShareOneFile(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "multi",
			Meta: map[string]any{"event": "PreToolUse", "command": []any{"echo one", "echo two"}},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".kiro/hooks/multi.json"))
	var doc hooksFile
	if err := json.Unmarshal([]byte(got), &doc); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, got)
	}
	if len(doc.Hooks) != 2 {
		t.Fatalf("expected two hook entries, got %d", len(doc.Hooks))
	}
	firstAction, _ := doc.Hooks[0]["action"].(map[string]any)
	if doc.Hooks[0]["name"] != "multi" || firstAction["command"] != "echo one" {
		t.Errorf("unexpected first entry: %+v", doc.Hooks[0])
	}
	secondAction, _ := doc.Hooks[1]["action"].(map[string]any)
	if doc.Hooks[1]["name"] != "multi-2" || secondAction["command"] != "echo two" {
		t.Errorf("unexpected second entry: %+v", doc.Hooks[1])
	}
}

// A hook spec with no `event` produces no file: there is no trigger to
// register it against.
func TestEmit_Hook_NoEventNoOutput(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "no-event", Meta: map[string]any{"command": "echo hi"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".kiro/hooks/no-event.json")); !os.IsNotExist(err) {
		t.Errorf("expected no file for a hook with no event, err=%v", err)
	}
}

// A hook spec with no `command` produces no file: there is nothing for
// the action to run.
func TestEmit_Hook_NoCommandNoOutput(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "no-command", Meta: map[string]any{"event": "Stop"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".kiro/hooks/no-command.json")); !os.IsNotExist(err) {
		t.Errorf("expected no file for a hook with no command, err=%v", err)
	}
}

func TestEmit_HooksDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{Outputs: map[string]config.Output{"kiro": {HooksDir: "custom/hooks"}}}
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "h1", Meta: map[string]any{"event": "Stop", "command": "echo done"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/hooks/h1.json")); err != nil {
		t.Errorf("expected override dir to hold the hook file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".kiro/hooks/h1.json")); !os.IsNotExist(err) {
		t.Errorf("expected no output at the default hooks dir, err=%v", err)
	}
}

// `description` is a generic spec field (docs/user/spec-format.md's
// Hooks table: "Free-form documentation"); Kiro documents the matching
// `hooks[].description` as "Documentation only". It reaches the file
// now that entries build as a map instead of a fixed struct (#642).
func TestEmit_Hook_DescriptionReachesFile(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "fmt-go",
			Meta: map[string]any{
				"event": "PostToolUse", "command": "gofmt -w",
				"description": "Format Go files after an edit.",
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".kiro/hooks/fmt-go.json"))
	if !strings.Contains(got, `"description": "Format Go files after an edit."`) {
		t.Errorf("expected description to reach the file, got:\n%s", got)
	}
}

// `confirm` (kiro.dev/docs/hooks/: "Ask for confirmation before a Stop
// command hook runs") has no agnostic-ai spec equivalent, so it is only
// reachable through `x-kiro`. Before #642, hookEntry was a fixed Go
// struct with no route for any unknown key at any layer, including
// x-kiro, so this was unreachable by construction.
func TestEmit_Hook_XKiroConfirmPassesThrough(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "release",
			Meta: map[string]any{
				"event": "Stop", "command": "./release.sh",
				"x-kiro": map[string]any{
					"confirm": map[string]any{
						"question": "Ship the release?",
						"options": []any{
							map[string]any{"id": "yes", "label": "Ship it", "run": true},
							map[string]any{"id": "no", "label": "Cancel", "run": false},
						},
					},
				},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".kiro/hooks/release.json"))
	for _, want := range []string{`"confirm"`, `"question": "Ship the release?"`, `"id": "yes"`, `"label": "Ship it"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// A hook scoped away from kiro via `targets:` never reaches
// `.kiro/hooks/`: b.HooksFor(target) filters it out before emitHooks
// sees it.
func TestEmit_Hook_TargetScopingExcludesKiro(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "codex-only",
			Meta: map[string]any{"event": "Stop", "command": "echo done", "targets": []any{"codex"}},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".kiro/hooks/codex-only.json")); !os.IsNotExist(err) {
		t.Errorf("expected no output for a hook scoped to another target, err=%v", err)
	}
}
