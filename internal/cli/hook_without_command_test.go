package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestHookCommandIssues_FlagsAHookWithNothingToRun(t *testing.T) {
	for _, tc := range []struct {
		meta  map[string]any
		valid bool
		want  string
	}{
		{map[string]any{"on": "after-edit", "command": "fmt.sh"}, true, ""},
		{map[string]any{"on": "after-edit", "command": []any{"a.sh", "b.sh"}}, true, ""},
		{map[string]any{"event": "PreToolUse", "type": "http", "url": "https://example.test/hook"}, true, ""},
		{map[string]any{"event": "Stop", "type": "prompt", "prompt": "Check the work."}, true, ""},
		{map[string]any{"event": "PreToolUse", "type": "mcp_tool", "server": "s", "tool": "t"}, true, ""},
		{map[string]any{"event": "Stop", "commandWindows": "fmt.ps1"}, true, ""},
		{map[string]any{"event": "Stop", "x-kiro": map[string]any{"action": map[string]any{"type": "askAgent", "prompt": "Review."}}}, true, ""},
		{map[string]any{"event": "BeforeTool", "x-gemini": map[string]any{"hooks": []any{map[string]any{"command": "a.sh"}}}}, true, ""},
		{map[string]any{"event": "Stop", "x-claude": map[string]any{"command": "echo xc"}}, true, ""},
		{map[string]any{"event": "Stop", "x-qwen": map[string]any{"hooks": []any{map[string]any{"command": "a.sh"}}}}, true, ""},
		{map[string]any{"x-grok": map[string]any{"hooks": map[string]any{"Stop": []any{}}}}, true, ""},
		{map[string]any{"event": "Stop", "x-claude": map[string]any{"timeout": 5}}, false, ""},
		{map[string]any{"event": "Stop", "x-zed": map[string]any{"hooks": []any{"task-name"}}}, false, ""},
		{map[string]any{"event": "Stop", "x-gemini": map[string]any{"action": map[string]any{"type": "askAgent"}}}, false, ""},
		{map[string]any{"event": "Stop", "x-foo": map[string]any{"command": "echo foo"}}, false, ""},
		{map[string]any{"event": "Stop", "target": "claude", "x-codex": map[string]any{"command": "echo codex"}}, false, ""},
		{map[string]any{"event": "Stop", "target": "codex", "x-codex": map[string]any{"command": "echo codex"}}, true, ""},
		{map[string]any{"on": "after-edit", "run": "echo hi"}, false, "`run:`"},
		{map[string]any{"on": "after-edit", "comand": "echo hi"}, false, "`comand:`"},
		{map[string]any{"on": "after-edit", "command": ""}, false, ""},
		{map[string]any{"on": "after-edit", "command": []any{""}}, false, ""},
		{map[string]any{"on": "after-edit"}, false, ""},
	} {
		got := hookCommandIssues([]spec.Entry{{Kind: spec.KindHook, Name: "fmt", Path: ".agnostic-ai/hooks/fmt.yaml", Meta: tc.meta}})
		if (len(got) == 0) != tc.valid {
			t.Errorf("%v: valid=%t, got %v", tc.meta, tc.valid, got)
			continue
		}
		for _, issue := range got {
			if !strings.Contains(issue.Message, "no `command`") || !strings.Contains(issue.Message, tc.want) {
				t.Errorf("%v: message %q must say the hook has no command and name %s", tc.meta, issue.Message, tc.want)
			}
			if tc.want != "" && !strings.Contains(issue.Message, "did you mean `command:`") {
				t.Errorf("%v: message %q must suggest command", tc.meta, issue.Message)
			}
		}
	}
}

func hookWithoutCommandProject(t *testing.T) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	silence(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "fmt.yaml"), "name: fmt\non: after-edit\nrun: echo hi\n")
	return dir
}

func TestValidate_FailsOnAHookWithoutCommand(t *testing.T) {
	hookWithoutCommandProject(t)
	log := captureLog(t)
	out, err := runCLI(t, "validate")
	text := out + log.String()
	if err == nil {
		t.Fatalf("validate must fail:\n%s", text)
	}
	if !strings.Contains(text, "fmt.yaml") || !strings.Contains(text, "no `command`") || !strings.Contains(text, "did you mean `command:` instead of `run:`") {
		t.Errorf("validate must name the file, the missing command, and suggest it:\n%s", text)
	}
}

func TestLint_ReportsAHookWithoutCommandAsAnErrorAndNotAsEmpty(t *testing.T) {
	hookWithoutCommandProject(t)
	log := captureLog(t)
	out, err := runCLI(t, "lint")
	text := out + log.String()
	if err == nil {
		t.Fatalf("lint must fail:\n%s", text)
	}
	if !strings.Contains(text, "LINT052") || !strings.Contains(text, "instead of `run:`") {
		t.Errorf("lint must report LINT052 with the suggestion:\n%s", text)
	}
	if strings.Contains(text, "LINT001") {
		t.Errorf("a hook needs no body or description, so LINT001 does not apply:\n%s", text)
	}
}

func TestLint_ValidHookGetsNoNewFinding(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "fmt.yaml"), "name: fmt\non: after-edit\ncommand: echo hi\n")
	log := captureLog(t)
	out, err := runCLI(t, "lint")
	if text := out + log.String(); err != nil || strings.Contains(text, "LINT052") || strings.Contains(text, "LINT001") {
		t.Errorf("a valid hook must lint clean: %v\n%s", err, text)
	}
}

func TestSync_SummaryLeavesOutAHookNoToolWrote(t *testing.T) {
	hookWithoutCommandProject(t)
	log := captureLog(t)
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	text := log.String()
	if !strings.Contains(text, "codex now reads, from .agnostic-ai/:") {
		t.Fatalf("first sync must print what each tool reads:\n%s", text)
	}
	if strings.Contains(text, "hook") {
		t.Errorf("the summary must not list a hook no tool wrote:\n%s", text)
	}
}

func TestExplainGlobal_ListsNoOutputForAHookWithoutCommand(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "fmt.yaml"), "name: fmt\non: after-edit\nrun: echo hi\n")
	out, err := runRoot(t, "explain", "--global", "hooks/fmt.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no target sync --global writes takes anything") {
		t.Errorf("explain must list no output for a hook with nothing to run:\n%s", out)
	}
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "fmt.yaml"), "name: fmt\non: after-edit\ncommand: echo hi\n")
	if out, err = runRoot(t, "explain", "--global", "hooks/fmt.yaml"); err != nil || !strings.Contains(out, "settings.json") {
		t.Errorf("a hook with a command still lists its outputs: %v\n%s", err, out)
	}
}
