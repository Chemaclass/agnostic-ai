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

// learn.chatgpt.com/docs/hooks documents additionalContextLimit (token
// threshold for model visibility) alongside type/command/timeout/
// statusMessage/commandWindows/matcher. See #533.
func TestEmit_HookAdditionalContextLimitPropagates(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "h1", Meta: map[string]any{
			"event":                  "PostToolUse",
			"command":                "lint.sh",
			"additionalContextLimit": 2000,
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".codex/hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"additionalContextLimit": 2000`) {
		t.Errorf("additionalContextLimit missing in hooks.json:\n%s", got)
	}
}

func TestEmit_HookAdditionalContextLimitZeroPropagates(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{Kind: spec.KindHook, Name: "h1", Meta: map[string]any{
		"event":                  "PostToolUse",
		"command":                "lint.sh",
		"additionalContextLimit": 0,
	}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".codex/hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"additionalContextLimit": 0`) {
		t.Errorf("explicit zero additionalContextLimit missing in hooks.json:\n%s", got)
	}
}

// learn.chatgpt.com/docs/hooks documents async ("Set async to true to
// run a command hook in the background while Codex continues") as the
// seventh command-hook field, alongside type/command/timeout/
// statusMessage/commandWindows/additionalContextLimit. See #636.
func TestEmit_HookAsyncPropagates(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "h1", Meta: map[string]any{
			"event":   "PostToolUse",
			"command": "lint.sh",
			"async":   true,
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".codex/hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"async": true`) {
		t.Errorf("async missing in hooks.json:\n%s", got)
	}
}

// learn.chatgpt.com/docs/hooks documents an MCP tool hook as
// `{type: "mcp_tool", server, tool, input, timeout, statusMessage}`.
// Before this, buildHooksJSON wrote `Type: "command"` unconditionally,
// so such a hook carried no `command` and hookCommands returned
// nothing: the entry silently disappeared from hooks.json rather than
// emitting the wrong shape (target-audit 2026-09-08, #693).
func TestEmit_HooksJSON_MCPToolHook(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "scan-patch",
			Meta: map[string]any{
				"event":         "PostToolUse",
				"matcher":       "Write|Edit",
				"type":          "mcp_tool",
				"server":        "scanner",
				"tool":          "scan_patch",
				"input":         map[string]any{"patch": "${tool_input.command}"},
				"timeout":       30,
				"statusMessage": "Scanning edited files",
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
	if !ok || len(post) != 1 {
		t.Fatalf("expected 1 PostToolUse matcher group, got:\n%s", raw)
	}
	group := post[0].(map[string]any)
	hooks, _ := group["hooks"].([]any)
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook entry, got %d:\n%s", len(hooks), raw)
	}
	entry := hooks[0].(map[string]any)
	if entry["type"] != "mcp_tool" {
		t.Errorf("type = %v, want mcp_tool", entry["type"])
	}
	if entry["server"] != "scanner" || entry["tool"] != "scan_patch" {
		t.Errorf("server/tool mismatch: %v", entry)
	}
	if _, hasCommand := entry["command"]; hasCommand {
		t.Errorf("mcp_tool entry must not carry command: %v", entry)
	}
	input, ok := entry["input"].(map[string]any)
	if !ok || input["patch"] != "${tool_input.command}" {
		t.Errorf("input mismatch: %v", entry["input"])
	}
	if entry["timeout"] != float64(30) {
		t.Errorf("timeout = %v, want 30", entry["timeout"])
	}
	if entry["statusMessage"] != "Scanning edited files" {
		t.Errorf("statusMessage = %v", entry["statusMessage"])
	}
}

// A command hook and an mcp_tool hook sharing an event must not
// collapse into one entry just because both have empty/matching
// identity fields; each keeps its own shape in the output.
func TestEmit_HooksJSON_MCPToolAndCommandHooksCoexist(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "fmt", Meta: map[string]any{
			"event": "PostToolUse", "matcher": "Write", "command": "gofmt -w",
		}},
		{Kind: spec.KindHook, Name: "scan", Meta: map[string]any{
			"event": "PostToolUse", "matcher": "Write", "type": "mcp_tool",
			"server": "scanner", "tool": "scan_patch",
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".codex/hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"command": "gofmt -w"`, `"type": "mcp_tool"`, `"server": "scanner"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("missing %q in:\n%s", want, raw)
		}
	}
}

// An mcp_tool hook with no server or tool has no identity to key on
// and no command to fall back to, so it is dropped rather than
// emitting a malformed entry.
func TestEmit_HooksJSON_MCPToolMissingServerOrToolIsSkipped(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "bad", Meta: map[string]any{
			"event": "PostToolUse", "type": "mcp_tool", "tool": "scan_patch",
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/hooks.json")); !os.IsNotExist(err) {
		t.Errorf("expected no hooks.json for a hook with no usable identity, err=%v", err)
	}
}
