package trae

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

func readHooks(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// TestEmit_Hook_WritesProjectHooksFile pins the documented shape of
// `.trae/hooks.json`: an integer `version` wrapper around a `hooks` map
// keyed by event, each event holding `{matcher, hooks: [{type, command,
// timeout}]}` groups (docs.trae.ai/ide/hook-configuration-reference).
func TestEmit_Hook_WritesProjectHooksFile(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
			"event": "PreToolUse", "matcher": "RunCommand",
			"command": "python3 ./validate_command.py", "timeout": 10,
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readHooks(t, filepath.Join(dir, ".trae/hooks.json"))
	for _, want := range []string{
		`"version": 1`, `"PreToolUse"`, `"matcher": "RunCommand"`,
		`"type": "command"`, `"command": "python3 ./validate_command.py"`, `"timeout": 10`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// TestEmit_Hook_EventsFollowVendorLifecycleOrder asserts the six
// documented events render in the vendor's own table order rather than
// the map order Go would otherwise pick, and that an event Trae does
// not document still passes through verbatim, after the known ones.
func TestEmit_Hook_EventsFollowVendorLifecycleOrder(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "n", Meta: map[string]any{"event": "Notification", "command": "echo n"}},
		{Kind: spec.KindHook, Name: "x", Meta: map[string]any{"event": "SomethingNew", "command": "echo x"}},
		{Kind: spec.KindHook, Name: "p", Meta: map[string]any{"event": "PreToolUse", "command": "echo p"}},
		{Kind: spec.KindHook, Name: "s", Meta: map[string]any{"event": "SessionStart", "command": "echo s"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readHooks(t, filepath.Join(dir, ".trae/hooks.json"))
	var order []int
	for _, event := range []string{"SessionStart", "PreToolUse", "Notification", "SomethingNew"} {
		i := strings.Index(got, `"`+event+`"`)
		if i < 0 {
			t.Fatalf("missing event %q in %s", event, got)
		}
		order = append(order, i)
	}
	for i := 1; i < len(order); i++ {
		if order[i-1] > order[i] {
			t.Errorf("events out of lifecycle order in %s", got)
		}
	}
}

// TestEmit_Hook_LoopLimitEmitsOnlyWhenSet covers Trae's own group-level
// field: an unset spec writes no key, so the vendor default of 5
// applies, and an explicit value emits as written.
func TestEmit_Hook_LoopLimitEmitsOnlyWhenSet(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "tests", Meta: map[string]any{
			"event": "Stop", "command": "python3 ./check_tests.py", "loop_limit": 3,
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if got := readHooks(t, filepath.Join(dir, ".trae/hooks.json")); !strings.Contains(got, `"loop_limit": 3`) {
		t.Errorf("expected loop_limit to emit, got %s", got)
	}

	other := t.TempDir()
	testutil.Chdir(t, other)
	bare := []spec.Entry{
		{Kind: spec.KindHook, Name: "tests", Meta: map[string]any{"event": "Stop", "command": "echo done"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(bare), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if got := readHooks(t, filepath.Join(other, ".trae/hooks.json")); strings.Contains(got, "loop_limit") {
		t.Errorf("expected no loop_limit key when the spec sets none, got %s", got)
	}
}

// TestEmit_Hook_ClaudeStyleMatcherNotesCoverage is the two-vocabulary
// trap: Trae's hook tool names spell the terminal tool `RunCommand`, so
// a Claude-authored `matcher: Bash` parses as a valid regex and matches
// nothing. The value still emits verbatim (guessing a rename would be
// worse), with one coverage note per sync naming the real vocabulary.
func TestEmit_Hook_ClaudeStyleMatcherNotesCoverage(t *testing.T) {
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
	if got := readHooks(t, filepath.Join(dir, ".trae/hooks.json")); !strings.Contains(got, `"matcher": "Bash"`) {
		t.Errorf("expected the matcher to emit verbatim, got %s", got)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "RunCommand") {
		t.Errorf("expected a coverage note naming Trae's own tool vocabulary, got: %s", buf.String())
	}
}

// TestEmit_Hook_NativeMatchersRaiseNoNote guards the other side of that
// check: a name from Trae's own table, an MCP tool name, a regex, and a
// matcher on an event that matches notification types rather than tools
// must all stay quiet, or the note becomes noise the user learns to
// ignore.
func TestEmit_Hook_NativeMatchersRaiseNoNote(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "a", Meta: map[string]any{"event": "PreToolUse", "matcher": "RunCommand", "command": "echo a"}},
		{Kind: spec.KindHook, Name: "b", Meta: map[string]any{"event": "PostToolUse", "matcher": "Edit|Write", "command": "echo b"}},
		{Kind: spec.KindHook, Name: "c", Meta: map[string]any{"event": "PreToolUse", "matcher": "mcp__Git__git_status", "command": "echo c"}},
		{Kind: spec.KindHook, Name: "d", Meta: map[string]any{"event": "Notification", "matcher": "idle_prompt", "command": "echo d"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if strings.Contains(buf.String(), "RunCommand") {
		t.Errorf("expected no matcher coverage note, got: %s", buf.String())
	}
}

// TestEmit_Hook_NoSpecsWritesNoFile keeps the adapter from dropping an
// empty `{"version": 1, "hooks": {}}` on a project with no hook specs.
func TestEmit_Hook_NoSpecsWritesNoFile(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".trae/hooks.json")); !os.IsNotExist(err) {
		t.Errorf("expected no hooks file for a bundle with no hooks, stat err: %v", err)
	}
}

// TestEmit_Hook_HonorsOutputsOverride confirms the path resolves through
// the shared outputs.trae.hooks-file key rather than a hard-coded one.
func TestEmit_Hook_HonorsOutputsOverride(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	cfg := &config.Config{Outputs: map[string]config.Output{"trae": {HooksFile: "custom/hooks.json"}}}
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "h", Meta: map[string]any{"event": "Stop", "command": "echo done"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/hooks.json")); err != nil {
		t.Errorf("expected the override path to be written: %v", err)
	}
}
