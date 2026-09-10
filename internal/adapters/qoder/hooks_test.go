package qoder

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

// TestEmit_Hook_WritesQoderSettings confirms a hook spec merges into
// `.qoder/settings.json` under the vendor's documented
// `{"hooks": {"<Event>": [{matcher, hooks: [...]}]}}` shape
// (docs.qoder.com/cli/hooks, "Configuration Format").
func TestEmit_Hook_WritesQoderSettings(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
			"event": "PreToolUse", "matcher": "Bash", "command": "hooks/guard.sh", "timeout": 30,
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readSettings(t, filepath.Join(dir, ".qoder/settings.json"))
	for _, want := range []string{
		`"hooks"`, `"PreToolUse"`, `"matcher": "Bash"`,
		`"type": "command"`, `"command": "hooks/guard.sh"`, `"timeout": 30`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// Qoder's own PreToolUse/PostToolUse matcher vocabulary is Claude
// Code's tool names ("e.g. Bash, Write, Edit, Read, Glob, Grep"), so a
// Claude-authored matcher passes straight through with no coverage
// note, unlike openhands and windsurf whose own tool vocabularies
// diverge (#629).
func TestEmit_Hook_ClaudeStyleMatcherPassesThroughNoCoverageNote(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
			"event": "PreToolUse", "matcher": "Bash", "command": "echo hi",
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readSettings(t, filepath.Join(dir, ".qoder/settings.json"))
	if !strings.Contains(got, `"matcher": "Bash"`) {
		t.Errorf("expected matcher to pass through, got %s", got)
	}

	emit.FlushCoverageNotes()
	if buf.Len() != 0 {
		t.Errorf("expected no coverage note for a Claude-style matcher, got: %s", buf.String())
	}
}

// docs.qoder.com/cli/hooks documents timeout, statusMessage, async,
// asyncRewake, shell, if, and once on a `command` hook entry with the
// same semantics claudehooks.CommandEntry already models for Claude
// Code (#629).
func TestEmit_Hook_AllSharedFieldsEmit(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
			"event": "Stop", "command": "hooks/check.sh",
			"timeout": 45, "statusMessage": "Checking...",
			"async": true, "asyncRewake": true,
			"shell": "bash", "if": "Bash(git *)", "once": true,
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readSettings(t, filepath.Join(dir, ".qoder/settings.json"))
	for _, want := range []string{
		`"timeout": 45`, `"statusMessage": "Checking..."`,
		`"async": true`, `"asyncRewake": true`,
		`"shell": "bash"`, `"if": "Bash(git *)"`, `"once": true`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// A `command` list produces one hook entry per command, matching
// Claude Code's and Codex's documented behavior for the same field
// (docs/user/spec-format.md, "When command is a list...").
func TestEmit_Hook_CommandListProducesMultipleEntries(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
			"event": "PostToolUse", "command": []any{"echo one", "echo two"},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readSettings(t, filepath.Join(dir, ".qoder/settings.json"))
	for _, want := range []string{`"command": "echo one"`, `"command": "echo two"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// mcpServers and hooks share `.qoder/settings.json`; both keys must
// land in one write. Two separate MergeJSONFile calls would each read
// the on-disk file before the other's write lands, which is exactly
// the shape that fooled sync's collision check into flagging qoder
// against itself (#629): see mcp.go's emitSettings doc.
func TestEmit_HookAndMCP_MergeIntoOneSettingsFile(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
			"event": "PreToolUse", "matcher": "Bash", "command": "echo hi",
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readSettings(t, filepath.Join(dir, ".qoder/settings.json"))
	for _, want := range []string{`"mcpServers"`, `"fs"`, `"hooks"`, `"PreToolUse"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// Same file, same guarantee `TestEmit_MCP_PreservesUnrelatedSettingsKeys`
// pins for mcpServers: a hook-only sync must not clobber sibling keys
// already on disk (mcp.enableAllProjectMcpServers, permissions, custom
// models, ...).
func TestEmit_Hook_PreservesUnrelatedSettingsKeys(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	if err := os.MkdirAll(filepath.Join(dir, ".qoder"), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"mcp":{"enableAllProjectMcpServers":true},"outputStyle":"concise"}`
	if err := os.WriteFile(filepath.Join(dir, ".qoder/settings.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "Stop", "command": "echo done"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readSettings(t, filepath.Join(dir, ".qoder/settings.json"))
	for _, want := range []string{`"enableAllProjectMcpServers"`, `"outputStyle"`, `"hooks"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestEmit_NoSettingsJSONWhenNoHooksOrMCPs(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{{Kind: spec.KindRule, Name: "r1", Body: "x"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".qoder/settings.json")); !os.IsNotExist(err) {
		t.Errorf("expected no .qoder/settings.json when no hook or MCP entries, err=%v", err)
	}
}
