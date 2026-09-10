package augment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestEmit_Hook_WritesAugmentSettings confirms a hook spec merges into
// `.augment/settings.json` under the vendor's documented
// `{"hooks": {"<Event>": [{matcher, hooks: [...]}]}}` shape
// (docs.augmentcode.com/cli/hooks, "Structure"), with `timeout`
// converted from the shared spec's seconds to Augment's milliseconds.
func TestEmit_Hook_WritesAugmentSettings(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
			"event": "PreToolUse", "matcher": "launch-process", "command": "hooks/guard.sh", "timeout": 30,
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment/settings.json"))
	for _, want := range []string{
		`"hooks"`, `"PreToolUse"`, `"matcher": "launch-process"`,
		`"type": "command"`, `"command": "hooks/guard.sh"`, `"timeout": 30000`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// timeout is milliseconds on Augment; the shared hook spec's own field
// is seconds, so an absent timeout must not fall back to writing 0 (a
// real, near-instant Augment timeout) and must instead stay unset so
// the vendor's own 60000ms default applies.
func TestEmit_Hook_NoTimeoutOmitsKeyRatherThanZero(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
			"event": "SessionStart", "command": "hooks/setup.sh",
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment/settings.json"))
	if strings.Contains(got, `"timeout"`) {
		t.Errorf("expected no timeout key when the spec never sets one, got %s", got)
	}
}

// Session events (Stop, SessionStart, SessionEnd) document matcher as
// "Not used", and the vendor's own SessionStart example omits the key
// outright. A matcher set on one of these three must not reach the
// file at all, even accidentally.
func TestEmit_Hook_SessionEventsOmitMatcherKey(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "notify", Meta: map[string]any{
			"event": "SessionStart", "matcher": "should-be-dropped", "command": "hooks/setup.sh",
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment/settings.json"))
	if strings.Contains(got, `"matcher"`) {
		t.Errorf("expected no matcher key on a session event, got %s", got)
	}
}

// A command that is a path ending in one of Augment's four supported
// script extensions runs; docs.augmentcode.com/cli/hooks, "Script
// Requirements".
func TestEmit_Hook_ScriptExtensionCommandNoCoverageNote(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
			"event": "PreToolUse", "command": "hooks/guard.ps1",
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if n := emit.PendingCoverageNotesCount(); n != 0 {
		t.Errorf("expected no coverage note for a .ps1 command, got %d", n)
	}
}

// A command with no recognized script extension still emits verbatim
// (no guessed rename) but surfaces a coverage note: Augment silently
// never runs it (docs.augmentcode.com/cli/hooks, "Script Requirements").
func TestEmit_Hook_InlineCommandSurfacesCoverageNote(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
			"event": "PostToolUse", "command": "gofmt -w",
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment/settings.json"))
	if !strings.Contains(got, `"command": "gofmt -w"`) {
		t.Errorf("expected the command to still emit verbatim, got %s", got)
	}
	if n := emit.PendingCoverageNotesCount(); n != 1 {
		t.Errorf("expected one field no-op note for the missing script extension, got %d", n)
	}
}

// Augment's own PreToolUse/PostToolUse matcher vocabulary is its tool
// names (launch-process, str-replace-editor, save-file, ...), not
// Claude Code's, so a Claude-style matcher parses and then matches
// nothing; that case surfaces a coverage note rather than a silent
// pass-through, the same treatment openhands and windsurf give their
// own mismatched tool vocabularies (#629).
func TestEmit_Hook_ClaudeStyleMatcherSurfacesCoverageNote(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
			"event": "PreToolUse", "matcher": "Bash", "command": "hooks/guard.sh",
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment/settings.json"))
	if !strings.Contains(got, `"matcher": "Bash"`) {
		t.Errorf("expected matcher to still emit verbatim, got %s", got)
	}
	if n := emit.PendingCoverageNotesCount(); n != 1 {
		t.Errorf("expected one field no-op note for a Claude-style matcher, got %d", n)
	}
}

// A `command` list produces one hook entry per command, matching
// Claude Code's, Codex's, and Qoder's documented behavior for the same
// field (docs/user/spec-format.md, "When command is a list...").
func TestEmit_Hook_CommandListProducesMultipleEntries(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
			"event": "PostToolUse", "command": []any{"hooks/one.sh", "hooks/two.sh"},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment/settings.json"))
	for _, want := range []string{`"command": "hooks/one.sh"`, `"command": "hooks/two.sh"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// mcpServers and hooks share `.augment/settings.json`; both keys must
// land in one write. Two separate MergeJSONFile calls would each read
// the on-disk file before the other's write lands, which is exactly
// the shape that fooled sync's collision check into flagging qoder
// against itself (#629, #718): see emitSettings' doc in augment.go.
func TestEmit_HookAndMCP_MergeIntoOneSettingsFile(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
			"event": "PreToolUse", "matcher": "launch-process", "command": "hooks/guard.sh",
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment/settings.json"))
	for _, want := range []string{`"mcpServers"`, `"fs"`, `"hooks"`, `"PreToolUse"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// Same file, same guarantee TestEmit_MCP_PreservesExistingSettingsKeys
// pins for mcpServers: a hook-only sync must not clobber sibling keys
// already on disk (shell, startupScript, theme, tool permissions, ...).
func TestEmit_Hook_PreservesExistingSettingsKeys(t *testing.T) {
	dir := testutil.TempCwd(t)

	if err := os.MkdirAll(filepath.Join(dir, ".augment"), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"shell": "zsh", "theme": "ansi"}`
	if err := os.WriteFile(filepath.Join(dir, ".augment/settings.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "Stop", "command": "hooks/done.sh"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment/settings.json"))
	for _, want := range []string{`"shell": "zsh"`, `"theme": "ansi"`, `"hooks"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestEmit_NoSettingsJSONWhenNoHooksOrMCPs(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "x"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".augment/settings.json")); !os.IsNotExist(err) {
		t.Errorf("expected no .augment/settings.json when no hook or MCP entries, err=%v", err)
	}
}
