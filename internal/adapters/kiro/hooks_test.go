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
	if h.Name != "fmt-go" || h.Trigger != "PostToolUse" || h.Matcher != "Edit" || h.Timeout != 30 {
		t.Errorf("unexpected entry: %+v", h)
	}
	if h.Action.Type != "command" || h.Action.Command != "gofmt -w" {
		t.Errorf("unexpected action: %+v", h.Action)
	}
	if h.Enabled != nil {
		t.Errorf("expected no enabled key for a hook that is not disabled, got %v", *h.Enabled)
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
	if doc.Hooks[0].Name != "multi" || doc.Hooks[0].Action.Command != "echo one" {
		t.Errorf("unexpected first entry: %+v", doc.Hooks[0])
	}
	if doc.Hooks[1].Name != "multi-2" || doc.Hooks[1].Action.Command != "echo two" {
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
