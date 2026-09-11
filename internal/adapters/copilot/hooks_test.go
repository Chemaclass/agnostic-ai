package copilot

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

func readHooksFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestEmit_Hook_WritesGithubHooksFile confirms a hook spec writes
// `.github/hooks/agnostic-ai.json` under the vendor's documented
// `{"version": 1, "hooks": {"<event>": [{"type": "command", ...}]}}`
// shape (docs.github.com/en/copilot/reference/hooks-reference, "Hook
// configuration format" and "Command hooks").
func TestEmit_Hook_WritesGithubHooksFile(t *testing.T) {
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
	got := readHooksFile(t, filepath.Join(dir, ".github/hooks/agnostic-ai.json"))
	for _, want := range []string{
		`"version": 1`, `"hooks"`, `"PreToolUse"`, `"matcher": "Bash"`,
		`"type": "command"`, `"command": "hooks/guard.sh"`, `"timeoutSec": 30`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, `"timeout":`) {
		t.Errorf("expected timeoutSec, not the deprecated timeout alias, got %s", got)
	}
}

// TestEmit_Hook_VersionIsIntegerOne pins the wrapper's `version` field
// as the bare integer `1`, not the quoted string `"1"` or `"v1"` —
// Kiro's own wrapper needed a fix for exactly that string-vs-int
// mistake (#626). docs.github.com/en/copilot/reference/hooks-reference's
// own example is unambiguous: `{ "version": 1, "hooks": { ... } }`.
func TestEmit_Hook_VersionIsIntegerOne(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "Stop", "command": "echo done"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readHooksFile(t, filepath.Join(dir, ".github/hooks/agnostic-ai.json"))
	if !strings.Contains(got, `"version": 1,`) {
		t.Errorf("expected bare integer version 1, got %s", got)
	}
	if strings.Contains(got, `"version": "1"`) || strings.Contains(got, `"version": "v1"`) {
		t.Errorf("version must not be a quoted string, got %s", got)
	}
}

// TestEmit_Hook_EventPassesThroughVerbatim confirms `event:` is used
// as the literal JSON key with no case translation, matching
// docs/user/spec-format.md's stated policy and every other hook
// emitter in this repo. Both a PascalCase event (this repo's dominant
// vocabulary) and Copilot's own camelCase form are independently
// valid per the vendor's "Hook event input payloads" section, so both
// must reach the file unchanged.
func TestEmit_Hook_EventPassesThroughVerbatim(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "pascal", Meta: map[string]any{"event": "SessionStart", "command": "echo pascal"}},
		{Kind: spec.KindHook, Name: "camel", Meta: map[string]any{"event": "sessionEnd", "command": "echo camel"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readHooksFile(t, filepath.Join(dir, ".github/hooks/agnostic-ai.json"))
	for _, want := range []string{`"SessionStart"`, `"sessionEnd"`, `"echo pascal"`, `"echo camel"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// TestEmit_Hook_PascalCaseClaudeMatcherPassesThroughNoCoverageNote
// confirms the vendor's own claim: a PascalCase event name applies
// "Claude's matcher semantics instead of the native regex rule", so a
// Claude-style matcher (`Bash`) reaches Copilot unchanged when paired
// with a PascalCase event, with no coverage note — this repo's
// dominant convention for authoring a hook spec (#629).
func TestEmit_Hook_PascalCaseClaudeMatcherPassesThroughNoCoverageNote(t *testing.T) {
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
	got := readHooksFile(t, filepath.Join(dir, ".github/hooks/agnostic-ai.json"))
	if !strings.Contains(got, `"matcher": "Bash"`) {
		t.Errorf("expected matcher to pass through, got %s", got)
	}

	emit.FlushCoverageNotes()
	if buf.Len() != 0 {
		t.Errorf("expected no coverage note for a PascalCase event with a Claude-style matcher, got: %s", buf.String())
	}
}

// TestEmit_Hook_CamelCaseClaudeMatcherNotesNoOp is the companion case:
// pairing Copilot's own camelCase event form with a Claude-style
// PascalCase matcher earns a coverage note, because that combination
// selects the native regex rule tested against Copilot's own
// lowercase tool names ("bash", not "Bash") rather than Claude's
// matcher semantics (#629).
func TestEmit_Hook_CamelCaseClaudeMatcherNotesNoOp(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
			"event": "preToolUse", "matcher": "Bash", "command": "echo hi",
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readHooksFile(t, filepath.Join(dir, ".github/hooks/agnostic-ai.json"))
	if !strings.Contains(got, `"matcher": "Bash"`) {
		t.Errorf("expected matcher to still emit (entry itself is not downgraded), got %s", got)
	}

	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "`matcher` on 1 hook has no effect on copilot") {
		t.Errorf("expected a field no-op note for the camelCase/Claude-matcher combination, got: %s", buf.String())
	}
}

// A `command` list produces one hook entry per command, same
// documented behavior as Claude Code, Codex, and Qoder for the same
// field (docs/user/spec-format.md, "When command is a list...").
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
	got := readHooksFile(t, filepath.Join(dir, ".github/hooks/agnostic-ai.json"))
	for _, want := range []string{`"command": "echo one"`, `"command": "echo two"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// A spec that sets `args` switches to Copilot's own shell-free form.
// The field table on docs.github.com/en/copilot/reference/hooks-reference
// reads "`exec` | string | Instead of `bash`, `powershell`, and
// `command` | Executable name or path. Runs the executable directly
// without a shell." and "`args` | array of strings | No | Arguments
// passed directly to `exec`." The executable moves out of `command`,
// which Claude Code's own exec form does not do, and the prose forbids
// carrying both: "Do not combine `exec` with `bash`, `powershell`, or
// `command`" (#755). The command below carries a space so the test
// distinguishes exec form from the shell form that would tokenize it.
func TestEmit_Hook_ArgsEmitExecFormWithoutCommand(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "fmt", Meta: map[string]any{
			"event": "PreToolUse", "matcher": "Edit",
			"command": "/usr/local/bin/my formatter",
			"args":    []any{"--fix", "$CLAUDE_FILE_PATHS"},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readHooksFile(t, filepath.Join(dir, ".github/hooks/agnostic-ai.json"))
	for _, want := range []string{
		`"exec": "/usr/local/bin/my formatter"`, `"args"`, `"--fix"`, `"$CLAUDE_FILE_PATHS"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, `"command":`) {
		t.Errorf("exec must not be combined with command, got %s", got)
	}
}

// The exec form is Copilot CLI only. The same page's "Hooks locations"
// section says a cloud agent job fires "a subset of events ... and only
// `bash` (or `command`) entries are honored", and loads its hooks from
// "`.github/hooks/*.json` files in the cloned repository" — the file
// this adapter writes. Emitting `exec` buys shell-free arguments and
// costs cloud-agent execution, so the note names the surface that
// drops the entry rather than claiming the whole target ignores it.
func TestEmit_Hook_ExecFormNotesCloudAgentSurfaceGap(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "fmt", Meta: map[string]any{
			"event": "PreToolUse", "command": "fmt.sh", "args": []any{"--fix"},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	out := buf.String()
	if !strings.Contains(out, "1 hook reaches copilot but not Copilot cloud agent") {
		t.Errorf("expected a surface gap note for the exec form, got: %s", out)
	}
	if !strings.Contains(out, "honors `bash` or `command` entries only") {
		t.Errorf("note must name the cloud-agent reason, got: %s", out)
	}
}

// No `args` means no exec form and no note: the cross-platform
// `command` field is the one a cloud agent job honors, so the default
// shape stays the portable one.
func TestEmit_Hook_NoArgsKeepsCommandFormAndRaisesNoNote(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
			"event": "PreToolUse", "command": "hooks/guard.sh",
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readHooksFile(t, filepath.Join(dir, ".github/hooks/agnostic-ai.json"))
	if !strings.Contains(got, `"command": "hooks/guard.sh"`) {
		t.Errorf("expected command form, got %s", got)
	}
	for _, unwanted := range []string{`"exec"`, `"args"`} {
		if strings.Contains(got, unwanted) {
			t.Errorf("unexpected %q in %s", unwanted, got)
		}
	}

	emit.FlushCoverageNotes()
	if buf.Len() != 0 {
		t.Errorf("expected no coverage note without args, got: %s", buf.String())
	}
}

func TestEmit_NoHooksFileWhenNoHookEntries(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{{Kind: spec.KindRule, Name: "r1", Body: "x"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".github/hooks/agnostic-ai.json")); !os.IsNotExist(err) {
		t.Errorf("expected no .github/hooks/agnostic-ai.json when no hook entries, err=%v", err)
	}
}
