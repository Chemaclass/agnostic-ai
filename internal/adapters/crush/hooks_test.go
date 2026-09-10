package crush

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

func swapNoteWarner(t *testing.T) *strings.Builder {
	t.Helper()
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	return buf
}

// A PreToolUse hook is Crush's one supported event: it merges into
// crush.json's `hooks.PreToolUse` array with `matcher`, `command`, and
// `timeout` (docs/hooks/README.md, schema.json $defs.HookConfig).
func TestEmit_Hook_PreToolUseWritesCrushJSON(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "no-rm-rf",
			Meta: map[string]any{
				"event": "PreToolUse", "matcher": "^bash$",
				"command": "./hooks/no-rm-rf.sh", "timeout": 10,
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "crush.json"))
	for _, want := range []string{
		`"hooks"`, `"PreToolUse"`,
		`"name": "no-rm-rf"`,
		`"matcher": "^bash$"`,
		`"command": "./hooks/no-rm-rf.sh"`,
		`"timeout": 10`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// timeout is optional on crush's own HookConfig (default 30 seconds
// when absent); a hook spec that omits it must not write a `timeout`
// key at all rather than a guessed 0.
func TestEmit_Hook_OmitsTimeoutWhenUnset(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "log-all",
			Meta: map[string]any{"event": "PreToolUse", "command": "echo hi"},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "crush.json"))
	if strings.Contains(got, `"timeout"`) {
		t.Errorf("expected no timeout key when unset: %s", got)
	}
}

// A `command` list becomes one hooks.PreToolUse entry per command,
// the same fan-out claude/codex/openhands/qoder already apply.
func TestEmit_Hook_CommandListFansOutToMultipleEntries(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "multi",
			Meta: map[string]any{
				"event":   "PreToolUse",
				"command": []any{"echo one", "echo two"},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "crush.json"))
	for _, want := range []string{`"echo one"`, `"echo two"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// Crush "currently supports just one hook, PreToolUse" (docs/hooks/
// README.md, re-verified 2026-09-10). Any other event has no crush
// surface: the entry does not reach crush.json, and a coverage note
// fires instead of a silent drop.
func TestEmit_Hook_NonPreToolUseEventDropsWithCoverageNote(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "fmt-go",
			Meta: map[string]any{"event": "PostToolUse", "command": "gofmt -w"},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "crush.json")); !os.IsNotExist(err) {
		t.Errorf("expected no crush.json for a PostToolUse-only hook, err=%v", err)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "PreToolUse hook event") {
		t.Errorf("expected a coverage note naming the PreToolUse restriction, got: %s", buf.String())
	}
}

// A matcher carried over from a Claude/Codex spec (Bash, Edit, Write,
// ...) parses as a valid regex against Crush's own lowercase tool
// names and then matches nothing. The entry still emits (unlike the
// wrong-event case above); only the matcher's effectiveness is in
// question, so this is a field no-op note, not a coverage gap.
func TestEmit_Hook_ClaudeStyleMatcherNoOpNote(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "block-bash",
			Meta: map[string]any{"event": "PreToolUse", "matcher": "Bash", "command": "echo no"},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "matcher") {
		t.Errorf("expected a matcher no-op note, got: %s", buf.String())
	}
}

// Crush's own lowercase matcher (its worked examples use `^bash$`)
// must not trip the Claude-style no-op note.
func TestEmit_Hook_LowercaseMatcherNoNote(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "block-bash",
			Meta: map[string]any{"event": "PreToolUse", "matcher": "^bash$", "command": "echo no"},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if strings.Contains(buf.String(), "matcher") {
		t.Errorf("lowercase matcher must not trip the Claude-style note, got: %s", buf.String())
	}
}

// mcp and hooks share one crush.json and must merge in a single write:
// an MCP entry's presence must not push out a hook entry, or vice
// versa, and both keys must land in the same sync (#629, PR #718's
// finding on separate MergeJSONFile calls to one path).
func TestEmit_MCPAndHookMergeInOneWrite(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
		{
			Kind: spec.KindHook, Name: "no-rm-rf",
			Meta: map[string]any{"event": "PreToolUse", "command": "./hooks/no-rm-rf.sh"},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "crush.json"))
	for _, want := range []string{`"mcp"`, `"fs"`, `"hooks"`, `"PreToolUse"`, `no-rm-rf.sh`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// Merging hooks into an existing hand-authored crush.json must not
// clobber unrelated user keys, the same guarantee
// TestEmit_MCP_PreservesExistingUserKeys already proves for mcp.
func TestEmit_Hook_PreservesExistingUserKeys(t *testing.T) {
	dir := testutil.TempCwd(t)

	existing := `{"models": {"large": "x"}, "options": {"debug": true}}`
	if err := os.WriteFile(filepath.Join(dir, "crush.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "no-rm-rf",
			Meta: map[string]any{"event": "PreToolUse", "command": "./hooks/no-rm-rf.sh"},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "crush.json"))
	for _, want := range []string{`"large": "x"`, `"debug": true`, `"hooks"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestEmit_NoHookNoteWhenNoHooks(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "x"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if got := emit.PendingCoverageNotesCount(); got != 0 {
		t.Errorf("no hook specs must buffer no note, count=%d", got)
	}
	emit.FlushCoverageNotes()
	if buf.Len() != 0 {
		t.Errorf("expected no output, got: %s", buf.String())
	}
}
