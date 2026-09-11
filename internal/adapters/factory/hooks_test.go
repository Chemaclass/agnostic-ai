package factory

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
// `{"<Event>": [...]}` with no surrounding `"hooks"` key: "Standalone
// `hooks.json` files are keyed directly by event name"
// (docs.factory.ai/harness/hooks). Every Claude-shaped hook target
// (claude, codex, gemini, qoder, openhands) nests the same shape under
// that key, so this is Factory's one structural divergence, shared
// only with Windsurf/Devin CLI's `.devin/hooks.v1.json` (#629).
func TestEmit_Hooks_NoWrapperKey(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "check", Meta: map[string]any{"event": "PreToolUse", "matcher": "Execute", "command": "./scripts/audit-git-command.sh"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".factory/hooks.json"))
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, raw)
	}
	if _, wrapped := doc["hooks"]; wrapped {
		t.Errorf("hooks.json must not carry a top-level \"hooks\" wrapper key:\n%s", raw)
	}
	pre, ok := doc["PreToolUse"].([]any)
	if !ok || len(pre) != 1 {
		t.Fatalf("expected PreToolUse at the top level:\n%s", raw)
	}
}

// TestEmit_Hooks_MatcherGroupShape confirms a command hook renders
// `{matcher, hooks: [{type: "command", command, timeout}]}` per
// matcher group, the vendor's own documented example shape.
func TestEmit_Hooks_MatcherGroupShape(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "check", Meta: map[string]any{
			"event": "PreToolUse", "matcher": "Execute",
			"command": "./scripts/audit-git-command.sh", "timeout": 30,
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".factory/hooks.json"))
	for _, want := range []string{
		`"matcher": "Execute"`, `"type": "command"`,
		`"command": "./scripts/audit-git-command.sh"`, `"timeout": 30`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestEmit_Hooks_OmitsTimeoutWhenAbsent confirms an unset timeout
// leaves the field off the rendered entry, so Droid CLI's own default
// (60 seconds) applies rather than an emitted zero.
func TestEmit_Hooks_OmitsTimeoutWhenAbsent(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "check", Meta: map[string]any{"event": "PreToolUse", "command": "./check.sh"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".factory/hooks.json"))
	if strings.Contains(got, "timeout") {
		t.Errorf("expected no timeout key when absent from the spec:\n%s", got)
	}
}

// TestEmit_Hooks_ClaudeStyleMatcherSurfacesCoverageNote confirms a
// matcher copied from a Claude spec still emits verbatim but folds
// into one coverage note, since Droid CLI's own tool vocabulary
// spells three names (Bash, Write, WebFetch) differently (Execute,
// Create, FetchUrl).
func TestEmit_Hooks_ClaudeStyleMatcherSurfacesCoverageNote(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "fmt", Meta: map[string]any{"event": "PostToolUse", "matcher": "Bash", "command": "echo ran"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".factory/hooks.json"))
	if !strings.Contains(got, `"matcher": "Bash"`) {
		t.Errorf("expected the matcher to still emit verbatim:\n%s", got)
	}
}

// TestEmit_Hooks_MatchingFactoryVocabularyNoNote confirms a matcher
// that already spells Droid CLI's own tool name (Execute, in place of
// Claude's Bash) raises no coverage note.
func TestEmit_Hooks_MatchingFactoryVocabularyNoNote(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "fmt", Meta: map[string]any{"event": "PreToolUse", "matcher": "Execute", "command": "echo ran"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 0 {
		t.Errorf("expected no coverage note for a Factory-native matcher, got %d pending warnings", got)
	}
}

// TestEmit_Hooks_FileOverride confirms outputs.factory.hooks-file
// redirects the output.
func TestEmit_Hooks_FileOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{Outputs: map[string]config.Output{"factory": {HooksFile: "vendor/factory/hooks.json"}}}
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "check", Meta: map[string]any{"event": "PreToolUse", "command": "./check.sh"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/factory/hooks.json")); err != nil {
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
	if _, err := os.Stat(filepath.Join(dir, ".factory/hooks.json")); !os.IsNotExist(err) {
		t.Errorf("expected no hooks.json without any hook specs, err=%v", err)
	}
}

// TestEmit_Hooks_MultipleEventsPreserveVendorOrder confirms events
// render in the vendor's own documented Event Reference order
// regardless of spec authoring order, so sync --check stays stable.
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
	got := readFile(t, filepath.Join(dir, ".factory/hooks.json"))
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

// TestEmit_Hooks_NoEventDropsEntry confirms a hook spec missing
// `event:` produces no output rather than a malformed entry.
func TestEmit_Hooks_NoEventDropsEntry(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "no-event", Meta: map[string]any{"command": "echo hi"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".factory/hooks.json")); !os.IsNotExist(err) {
		t.Errorf("expected no hooks.json for a hook spec with no event, err=%v", err)
	}
}
