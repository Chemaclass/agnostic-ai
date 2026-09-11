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

// Event keys land in the vendor's documented lifecycle order first,
// then any unlisted event in first-seen order. The four events
// docs.qoder.com/cli/hooks-reference carries beyond the grouped
// `/cli/hooks` table (`TaskCreated`, `TaskCompleted`, `TeammateIdle`,
// `Setup`) now sort among the lifecycle instead of behind it, so a
// spec that names one alongside an unlisted event writes different
// bytes than it did before #744. This pins that order.
func TestEmit_Hook_EventOrderIsLifecycleThenFirstSeen(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "custom", Meta: map[string]any{"event": "CustomThing", "command": "echo custom"}},
		{Kind: spec.KindHook, Name: "setup", Meta: map[string]any{"event": "Setup", "command": "echo setup"}},
		{Kind: spec.KindHook, Name: "created", Meta: map[string]any{"event": "TaskCreated", "command": "echo created"}},
		{Kind: spec.KindHook, Name: "start", Meta: map[string]any{"event": "SessionStart", "command": "echo start"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readSettings(t, filepath.Join(dir, ".qoder/settings.json"))

	want := []string{"SessionStart", "TaskCreated", "Setup", "CustomThing"}
	prev := -1
	for _, event := range want {
		at := strings.Index(got, `"`+event+`"`)
		if at < 0 {
			t.Fatalf("missing event %q in %s", event, got)
		}
		if at < prev {
			t.Errorf("event %q emitted out of order, want %v in:\n%s", event, want, got)
		}
		prev = at
	}
}

// `args` switches a command hook to exec form: "`command` is the
// path/name of a single executable, and each element of `args` is one
// literal argv entry. The CLI runs the binary directly without a shell"
// (docs.qoder.com/cli/hooks, "Exec form vs Shell form", #746).
func TestEmit_Hook_ArgsEmitExecForm(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "check", Meta: map[string]any{
			"event": "PreToolUse", "command": "/usr/bin/python3",
			"args": []any{"/opt/my scripts/check.py", "--strict"},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readSettings(t, filepath.Join(dir, ".qoder/settings.json"))
	for _, want := range []string{
		`"command": "/usr/bin/python3"`, `"args"`,
		`"/opt/my scripts/check.py"`, `"--strict"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// A hook without `args` stays shell form. An empty array would flip
// qoder into exec form and stop the command from being shell-parsed.
func TestEmit_Hook_NoArgsKeyWhenUnset(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
			"event": "PreToolUse", "command": "echo hi",
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readSettings(t, filepath.Join(dir, ".qoder/settings.json"))
	if strings.Contains(got, `"args"`) {
		t.Errorf("expected no args key when unset:\n%s", got)
	}
}

// "The `shell` field is ignored when `args` is set"
// (docs.qoder.com/cli/hooks). The value still emits, since the user
// authored it and qoder accepts the key, but the note says it will not
// run (#746).
func TestEmit_Hook_ShellWithArgsNotesFieldNoOp(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "check", Meta: map[string]any{
			"event": "Stop", "command": "/usr/bin/python3",
			"args": []any{"check.py"}, "shell": "bash",
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readSettings(t, filepath.Join(dir, ".qoder/settings.json"))
	if !strings.Contains(got, `"shell": "bash"`) {
		t.Errorf("expected shell to emit verbatim, got %s", got)
	}

	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "shell") {
		t.Errorf("expected a shell field no-op note for an exec-form hook, got: %q", buf.String())
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
