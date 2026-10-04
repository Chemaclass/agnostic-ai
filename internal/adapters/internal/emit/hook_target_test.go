package emit

import (
	"os/exec"
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestExportHookTarget_ReachesEveryCommandInAList(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no POSIX shell")
	}
	command := ExportHookTarget(`printf '%s ' "$AGNOSTIC_AI_TARGET" && printf '%s' "$AGNOSTIC_AI_TARGET"`, "codex")
	out, err := exec.Command("sh", "-c", command).Output()
	if err != nil {
		t.Fatalf("sh -c %q: %v", command, err)
	}
	if got := string(out); got != "codex codex" {
		t.Errorf("output = %q, want %q", got, "codex codex")
	}
}

func TestStripHookTargetExport_RestoresTheDeclaredCommand(t *testing.T) {
	t.Parallel()
	command := "git status --short"
	if got := StripHookTargetExport(ExportHookTarget(command, "goose"), "goose"); got != command {
		t.Errorf("round trip = %q, want %q", got, command)
	}
	other := ExportHookTarget(command, "crush")
	if got := StripHookTargetExport(other, "goose"); got != other {
		t.Errorf("another target's prefix was stripped: %q", got)
	}
}

func TestWithHookTarget_AddsTheTargetUnlessTheSpecSetsIt(t *testing.T) {
	t.Parallel()
	env := map[string]string{"FOO": "bar"}
	got := WithHookTarget(env, "qoder")
	if got[HookTargetEnv] != "qoder" || got["FOO"] != "bar" {
		t.Errorf("WithHookTarget = %v", got)
	}
	if _, ok := env[HookTargetEnv]; ok {
		t.Error("WithHookTarget changed its input")
	}
	pinned := WithHookTarget(map[string]string{HookTargetEnv: "mine"}, "qoder")
	if pinned[HookTargetEnv] != "mine" {
		t.Errorf("spec value lost: %v", pinned)
	}
	if got := WithHookTarget[any](nil, "gemini"); got[HookTargetEnv] != "gemini" {
		t.Errorf("WithHookTarget(nil) = %v", got)
	}
}

func TestWithoutHookTarget_DropsOnlyTheValueSyncAdded(t *testing.T) {
	t.Parallel()
	if got := WithoutHookTarget(map[string]string{HookTargetEnv: "qoder"}, "qoder"); got != nil {
		t.Errorf("only the target left = %v, want nil", got)
	}
	got := WithoutHookTarget(map[string]any{HookTargetEnv: "gemini", "FOO": "bar"}, any("gemini"))
	if _, ok := got[HookTargetEnv]; ok || got["FOO"] != "bar" {
		t.Errorf("WithoutHookTarget = %v", got)
	}
	pinned := map[string]string{HookTargetEnv: "mine"}
	if got := WithoutHookTarget(pinned, "qoder"); got[HookTargetEnv] != "mine" {
		t.Errorf("a pinned value was dropped: %v", got)
	}
}

func TestExecFormCommand_QuotesACommandOnlyWhenItNeedsIt(t *testing.T) {
	t.Parallel()
	if got := ExecFormCommand("node", []string{"guard.js"}); got != "node 'guard.js'" {
		t.Errorf("plain command = %q", got)
	}
	if got := ExecFormCommand("/opt/My Tools/guard", []string{"x"}); got != "'/opt/My Tools/guard' 'x'" {
		t.Errorf("command with a space = %q", got)
	}
	if got := ExecFormCommand("echo hi", nil); got != "echo hi" {
		t.Errorf("shell form = %q", got)
	}
}

func TestShellHookCommand_FoldsTheTargetsArgsAfterThePathRewrite(t *testing.T) {
	t.Parallel()
	meta := map[string]any{"args": []any{"two words"}}
	if got := ShellHookCommand(".agnostic-ai/scripts/guard.sh", "crush", meta); got != ".crush/hooks/guard.sh 'two words'" {
		t.Errorf("script path = %q", got)
	}
	meta["x-trae"] = map[string]any{"args": []any{"it's"}}
	if got := ShellHookCommand("echo", "trae", meta); got != `echo 'it'\''s'` {
		t.Errorf("x-trae args = %q", got)
	}
	if got := ShellHookCommand("echo hi", "trae", nil); got != "echo hi" {
		t.Errorf("shell form = %q", got)
	}
}

func TestTargetHook_ReadsCommandAndArgsAfterTheOverride(t *testing.T) {
	t.Parallel()
	h := spec.Entry{Kind: spec.KindHook, Meta: map[string]any{
		"command": "echo", "args": []any{"base"},
		"x-factory": map[string]any{"command": "printf", "args": []any{"target"}, "loop_limit": 2},
	}}
	got := TargetHook("factory", h)
	if got.Meta["command"] != "printf" || !reflect.DeepEqual(got.Meta["args"], []any{"target"}) {
		t.Errorf("factory meta = %v", got.Meta)
	}
	if _, ok := got.Meta["x-factory"].(map[string]any)["loop_limit"]; !ok {
		t.Errorf("the override lost its other keys: %v", got.Meta)
	}
	if h.Meta["command"] != "echo" {
		t.Errorf("the source spec changed: %v", h.Meta)
	}
	moved := spec.Entry{Kind: spec.KindHook, Meta: map[string]any{"event": "BeforeTool", "matcher": "a", "x-gemini": map[string]any{"event": "AfterTool", "matcher": "b"}}}
	if got := TargetHook("gemini", moved); got.Meta["event"] != "AfterTool" || got.Meta["matcher"] != "b" {
		t.Errorf("gemini event and matcher = %v", got.Meta)
	}
	if got := TargetHook("trae", h); got.Meta["command"] != "echo" {
		t.Errorf("another target read the override: %v", got.Meta)
	}
	unset := spec.Entry{Kind: spec.KindHook, Meta: map[string]any{"command": "echo", "args": []any{"a"}, "x-trae": map[string]any{"args": nil}}}
	if got := TargetHook("trae", unset); got.Meta["args"] != nil {
		t.Errorf("a null override kept args: %v", got.Meta)
	}
}
