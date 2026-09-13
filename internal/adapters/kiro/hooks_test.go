package kiro

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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

// A hook spec with neither a command nor a native action produces no file.
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

func TestEmit_Hook_DistinguishesZeroTimeoutFromAbsent(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "unlimited", Meta: map[string]any{
			"event": "Stop", "command": "echo done", "timeout": 0,
		}},
		{Kind: spec.KindHook, Name: "default", Meta: map[string]any{
			"event": "Stop", "command": "echo done",
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	unlimited := readKiroHookEntries(t, filepath.Join(dir, ".kiro/hooks/unlimited.json"))
	if unlimited[0]["timeout"] != float64(0) {
		t.Errorf("timeout = %v, want explicit zero", unlimited[0]["timeout"])
	}
	defaults := readKiroHookEntries(t, filepath.Join(dir, ".kiro/hooks/default.json"))
	if _, ok := defaults[0]["timeout"]; ok {
		t.Errorf("absent timeout emitted as %v", defaults[0]["timeout"])
	}
}

func TestEmit_Hook_PreservesDecimalStringTimeout(t *testing.T) {
	for _, tt := range []struct {
		value   string
		want    float64
		present bool
	}{
		{"30", 30, true},
		{"0", 0, true},
		{" 30 ", 30, true},
		{"invalid", 0, false},
		{"0seconds", 0, false},
		{"", 0, false},
	} {
		t.Run(tt.value, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			entry := spec.Entry{
				Kind: spec.KindHook, Name: "verify",
				Meta: map[string]any{
					"event": "Stop", "command": "echo done", "timeout": tt.value,
				},
			}
			if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false); err != nil {
				t.Fatal(err)
			}
			hooks := readKiroHookEntries(t, filepath.Join(dir, ".kiro/hooks/verify.json"))
			got, present := hooks[0]["timeout"]
			if present != tt.present || present && got != tt.want {
				t.Errorf("timeout = %v (present %v), want %v (present %v)", got, present, tt.want, tt.present)
			}
		})
	}
}

func TestEmit_Hook_AgentActionNeedsNoCommand(t *testing.T) {
	dir := testutil.TempCwd(t)
	action := map[string]any{"type": "agent", "prompt": "Check the result."}
	entry := spec.Entry{
		Kind: spec.KindHook, Name: "review",
		Meta: map[string]any{
			"event": "Stop", "description": "Review the completed work.",
			"x-kiro": map[string]any{"action": action},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	hooks := readKiroHookEntries(t, filepath.Join(dir, ".kiro/hooks/review.json"))
	if len(hooks) != 1 {
		t.Fatalf("got %d hooks, want one native agent action", len(hooks))
	}
	if !reflect.DeepEqual(hooks[0]["action"], action) {
		t.Errorf("action = %#v, want %#v", hooks[0]["action"], action)
	}
	if hooks[0]["description"] != "Review the completed work." {
		t.Errorf("description = %v", hooks[0]["description"])
	}
}

func TestEmit_Hook_NativeActionReplacesCommandList(t *testing.T) {
	dir := testutil.TempCwd(t)
	action := map[string]any{"type": "agent", "prompt": "Check the result."}
	entry := spec.Entry{
		Kind: spec.KindHook, Name: "review",
		Meta: map[string]any{
			"event": "Stop", "command": []any{"echo one", "echo two"},
			"x-kiro": map[string]any{"action": action},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	hooks := readKiroHookEntries(t, filepath.Join(dir, ".kiro/hooks/review.json"))
	if len(hooks) != 1 {
		t.Fatalf("got %d hooks, want one native action replacing the command list", len(hooks))
	}
	if !reflect.DeepEqual(hooks[0]["action"], action) {
		t.Errorf("action = %#v, want %#v", hooks[0]["action"], action)
	}
}

func TestEmit_Hook_NativeCommandActionNeedsNoGenericCommand(t *testing.T) {
	dir := testutil.TempCwd(t)
	action := map[string]any{"type": "command", "command": "./scripts/verify.sh"}
	entry := spec.Entry{
		Kind: spec.KindHook, Name: "verify",
		Meta: map[string]any{
			"event": "Stop", "timeout": 0,
			"x-kiro": map[string]any{"action": action},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	hooks := readKiroHookEntries(t, filepath.Join(dir, ".kiro/hooks/verify.json"))
	if len(hooks) != 1 {
		t.Fatalf("got %d hooks, want one native command action", len(hooks))
	}
	if !reflect.DeepEqual(hooks[0]["action"], action) || hooks[0]["timeout"] != float64(0) {
		t.Errorf("native command or zero timeout lost: %#v", hooks[0])
	}
}

func TestEmit_Hook_RejectsInvalidNativeAction(t *testing.T) {
	for _, tt := range []struct {
		name   string
		action any
	}{
		{"not an object", "echo hi"},
		{"null", nil},
		{"missing type", map[string]any{"prompt": "Check the result."}},
		{"unknown type", map[string]any{"type": "other", "prompt": "Check the result."}},
		{"missing prompt", map[string]any{"type": "agent"}},
		{"non-string prompt", map[string]any{"type": "agent", "prompt": 12}},
		{"blank prompt", map[string]any{"type": "agent", "prompt": " \n"}},
		{"missing command", map[string]any{"type": "command"}},
		{"command list", map[string]any{"type": "command", "command": []any{"echo one", "echo two"}}},
		{"blank command", map[string]any{"type": "command", "command": " \n"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			entry := spec.Entry{
				Kind: spec.KindHook, Name: "invalid",
				Meta: map[string]any{
					"event": "Stop", "command": "echo fallback",
					"x-kiro": map[string]any{"action": tt.action},
				},
			}
			err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false)
			if err == nil || !strings.Contains(err.Error(), "x-kiro.action") || !strings.Contains(err.Error(), "invalid") {
				t.Errorf("error = %v, want a native-action error identifying the hook", err)
			}
			if _, err := os.Stat(filepath.Join(dir, ".kiro/hooks/invalid.json")); !os.IsNotExist(err) {
				t.Errorf("invalid native action emitted a fallback hook: %v", err)
			}
		})
	}
}

func readKiroHookEntries(t *testing.T, path string) []map[string]any {
	t.Helper()
	var doc hooksFile
	if err := json.Unmarshal([]byte(readFile(t, path)), &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(doc.Hooks) == 0 {
		t.Fatalf("%s contains no hooks", path)
	}
	return doc.Hooks
}
