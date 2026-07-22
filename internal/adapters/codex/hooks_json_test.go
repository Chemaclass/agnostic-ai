package codex

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

// Two hook specs that share an event + command but declare different
// matcher pipe-sets must collapse into a single entry whose matcher is
// the deduplicated union. Codex CLI does not internally dedupe duplicate
// handlers, so two near-equivalent specs would otherwise fire the same
// script twice on overlapping events.
func TestEmit_HooksJSON_UnionsMatchersForSameEventCommand(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "h1",
			Meta: map[string]any{
				"event":   "PostToolUse",
				"matcher": "apply_patch|Edit|Write",
				"command": ".codex/hooks/format-php.sh",
			},
		},
		{
			Kind: spec.KindHook, Name: "h2",
			Meta: map[string]any{
				"event":   "PostToolUse",
				"matcher": "Edit|Write",
				"command": ".codex/hooks/format-php.sh",
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".codex/hooks.json"))
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, raw)
	}
	post, ok := doc["hooks"].(map[string]any)["PostToolUse"].([]any)
	if !ok {
		t.Fatalf("missing PostToolUse array in:\n%s", raw)
	}
	if len(post) != 1 {
		t.Fatalf("expected 1 dedup'd matcher group, got %d:\n%s", len(post), raw)
	}
	group := post[0].(map[string]any)
	matcher, _ := group["matcher"].(string)
	wantSegments := []string{"Edit", "Write", "apply_patch"}
	for _, seg := range wantSegments {
		if !strings.Contains(matcher, seg) {
			t.Errorf("union matcher missing %q in %q", seg, matcher)
		}
	}
	// Segments must appear exactly once each — no duplicates from the
	// two source specs.
	for _, seg := range wantSegments {
		if strings.Count(matcher, seg) != 1 {
			t.Errorf("matcher segment %q duplicated in union: %q", seg, matcher)
		}
	}
}

// Identical event + matcher + command triples collapse to a single
// entry (literal dedupe).
func TestEmit_HooksJSON_DropsIdenticalDuplicates(t *testing.T) {
	dir := testutil.TempCwd(t)

	dup := spec.Entry{
		Kind: spec.KindHook,
		Name: "fmt",
		Meta: map[string]any{
			"event":   "PostToolUse",
			"matcher": "Edit",
			"command": "echo dup",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{dup, dup}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, ".codex/hooks.json"))
	if strings.Count(string(raw), `"command": "echo dup"`) != 1 {
		t.Errorf("expected single command entry after dedupe, got:\n%s", raw)
	}
}

// Per-hook timeout + statusMessage land in the emitted entry so the
// metadata Claude carries survives a cross-tool sync to codex.
func TestEmit_HooksJSON_PreservesTimeoutAndStatusMessage(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "h",
			Meta: map[string]any{
				"event":         "PostToolUse",
				"matcher":       "Edit",
				"command":       "echo go",
				"timeout":       45,
				"statusMessage": "running",
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, ".codex/hooks.json"))
	for _, want := range []string{`"timeout": 45`, `"statusMessage": "running"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("missing %q in:\n%s", want, raw)
		}
	}
}

// config.toml no longer carries hook sections; they live in hooks.json.
// A bundle of hooks + no MCPs + no first-class config produces no
// config.toml file at all.
func TestEmit_HooksJSON_HooksMoveOutOfConfigToml(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "h",
			Meta: map[string]any{
				"event":   "PostToolUse",
				"matcher": "Edit",
				"command": "echo go",
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/hooks.json")); err != nil {
		t.Errorf("expected hooks.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/config.toml")); !os.IsNotExist(err) {
		t.Errorf("config.toml should not be emitted for hook-only bundles, got: %v", err)
	}
}

// Regression for #266: matcher pipe-segment order is preserved as
// authored, not alphabetized. `Bash|apply_patch|Edit|Write` (the form
// shipped by codex out of the box) must round-trip byte-stable.
func TestEmit_HooksJSON_PreservesMatcherTokenOrder(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "h",
			Meta: map[string]any{
				"event":   "PreToolUse",
				"matcher": "Bash|apply_patch|Edit|Write",
				"command": "echo go",
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, ".codex/hooks.json"))
	if !strings.Contains(string(raw), `"matcher": "Bash|apply_patch|Edit|Write"`) {
		t.Errorf("expected matcher order preserved, got:\n%s", raw)
	}
}

// outputs.codex.hooks-file relocates the emitted file.
func TestEmit_HooksJSON_HonorsHooksFileOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{Outputs: map[string]config.Output{
		"codex": {HooksFile: "vendor/codex.hooks.json"},
	}}
	entries := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "h",
			Meta: map[string]any{
				"event": "PostToolUse", "matcher": "Edit", "command": "echo go",
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/codex.hooks.json")); err != nil {
		t.Errorf("expected override path written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/hooks.json")); !os.IsNotExist(err) {
		t.Errorf("expected default path skipped when override set, got: %v", err)
	}
}

// commandWindows is Codex's optional Windows command override; it must
// land on the emitted hook entry.
func TestEmit_HookCommandWindowsPropagates(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "h1", Meta: map[string]any{
			"event":          "PreToolUse",
			"command":        "check.sh",
			"commandWindows": "check.ps1",
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".codex/hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"commandWindows": "check.ps1"`) {
		t.Errorf("commandWindows missing in hooks.json:\n%s", got)
	}
}
