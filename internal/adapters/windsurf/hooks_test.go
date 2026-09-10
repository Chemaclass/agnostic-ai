package windsurf

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

// TestEmit_Hooks_NoWrapperKey confirms the rendered document is
// `{"<Event>": [...]}` with no surrounding `"hooks"` key: "the hooks
// object is the entire file (no wrapper key needed)"
// (docs.devin.ai/cli/extensibility/hooks/overview). Every other native
// hook target (claude, codex, gemini, qoder, openhands) nests the same
// shape under that key, so this is windsurf's one structural
// divergence (#629).
func TestEmit_Hooks_NoWrapperKey(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "check", Meta: map[string]any{"event": "PreToolUse", "matcher": "exec", "command": "./scripts/check-command.sh"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".devin/hooks.v1.json"))
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, raw)
	}
	if _, wrapped := doc["hooks"]; wrapped {
		t.Errorf("hooks.v1.json must not carry a top-level \"hooks\" wrapper key:\n%s", raw)
	}
	pre, ok := doc["PreToolUse"].([]any)
	if !ok || len(pre) != 1 {
		t.Fatalf("expected PreToolUse at the top level:\n%s", raw)
	}
}

// TestEmit_Hooks_CommandEntryShape confirms a command hook renders
// `{type: "command", command, timeout}` per matcher group, the
// vendor's documented "Quick Example" shape.
func TestEmit_Hooks_CommandEntryShape(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "check", Meta: map[string]any{
			"event": "PreToolUse", "matcher": "exec",
			"command": "./scripts/validate.sh", "timeout": 10,
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".devin/hooks.v1.json"))
	for _, want := range []string{`"matcher": "exec"`, `"type": "command"`, `"command": "./scripts/validate.sh"`, `"timeout": 10`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestEmit_Hooks_PromptTypeCarriesPromptField confirms `type: prompt`
// renders `{type: "prompt", prompt}` instead of a `command` field.
// agnostic-ai's generic hook spec has no `prompt` field, so this is
// only reachable through a hand-authored `type`/`prompt` pair on the
// spec's own Meta.
func TestEmit_Hooks_PromptTypeCarriesPromptField(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "confirm", Meta: map[string]any{
			"event": "UserPromptSubmit", "type": "prompt",
			"prompt": "Does this look like a destructive command?",
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".devin/hooks.v1.json"))
	if !strings.Contains(got, `"type": "prompt"`) {
		t.Errorf("expected type: prompt:\n%s", got)
	}
	if !strings.Contains(got, `"prompt": "Does this look like a destructive command?"`) {
		t.Errorf("expected the prompt field to carry the prompt text:\n%s", got)
	}
	if strings.Contains(got, `"command"`) {
		t.Errorf("a prompt hook must not also carry a command key:\n%s", got)
	}
}

// TestEmit_Hooks_ClaudeStyleMatcherSurfacesCoverageNote confirms a
// matcher copied from a Claude spec (PascalCase tool name) still
// emits verbatim but folds into one coverage note, since Devin CLI's
// own tool vocabulary is lowercase and snake_case.
func TestEmit_Hooks_ClaudeStyleMatcherSurfacesCoverageNote(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "fmt", Meta: map[string]any{"event": "PostToolUse", "matcher": "Edit", "command": "gofmt -w"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".devin/hooks.v1.json"))
	if !strings.Contains(got, `"matcher": "Edit"`) {
		t.Errorf("expected the matcher to still emit verbatim:\n%s", got)
	}
}

// TestEmit_Hooks_FileOverride confirms outputs.windsurf.hooks-file
// redirects the output.
func TestEmit_Hooks_FileOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{Outputs: map[string]config.Output{"windsurf": {HooksFile: "vendor/devin/hooks.v1.json"}}}
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "check", Meta: map[string]any{"event": "PreToolUse", "command": "./check.sh"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/devin/hooks.v1.json")); err != nil {
		t.Errorf("expected override path written: %v", err)
	}
}

// TestEmit_Hooks_NoHooksWritesNothing confirms no file emits when the
// bundle carries no hooks.
func TestEmit_Hooks_NoHooksWritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{Kind: spec.KindRule, Name: "r1", Body: "rule body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".devin/hooks.v1.json")); !os.IsNotExist(err) {
		t.Errorf("expected no hooks.v1.json without any hook specs, err=%v", err)
	}
}

// TestEmit_Hooks_MultipleEventsPreserveVendorOrder confirms events
// render in the vendor's own documented lifecycle order regardless of
// spec authoring order, so sync --check stays stable.
func TestEmit_Hooks_MultipleEventsPreserveVendorOrder(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "on-stop", Meta: map[string]any{"event": "Stop", "command": "echo stop"}},
		{Kind: spec.KindHook, Name: "on-pre", Meta: map[string]any{"event": "PreToolUse", "command": "echo pre"}},
		{Kind: spec.KindHook, Name: "on-session-start", Meta: map[string]any{"event": "SessionStart", "command": "echo start"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".devin/hooks.v1.json"))
	preIdx := strings.Index(got, "PreToolUse")
	stopIdx := strings.Index(got, "Stop")
	startIdx := strings.Index(got, "SessionStart")
	if preIdx == -1 || stopIdx == -1 || startIdx == -1 {
		t.Fatalf("missing an expected event key:\n%s", got)
	}
	if preIdx >= stopIdx || stopIdx >= startIdx {
		t.Errorf("expected PreToolUse, then Stop, then SessionStart, got order in:\n%s", got)
	}
}
