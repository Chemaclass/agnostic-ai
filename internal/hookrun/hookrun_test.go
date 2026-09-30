package hookrun

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func decode(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("payload is no JSON object: %v\n%s", err, body)
	}
	return doc
}

func TestBuild_ClaudeEditNamesTheAbsoluteFileOfTheFirstMatchingTool(t *testing.T) {
	root := t.TempDir()
	p, err := Build("claude", "PreToolUse", "Edit|MultiEdit", root, Input{Edit: "src/Foo.php"})
	if err != nil {
		t.Fatal(err)
	}
	doc := decode(t, p.Body)
	input, _ := doc["tool_input"].(map[string]any)
	if !p.Fires || p.Trigger != "Edit" || doc["tool_name"] != "Edit" {
		t.Errorf("fires=%v trigger=%q tool_name=%v, want the Edit tool", p.Fires, p.Trigger, doc["tool_name"])
	}
	if want := filepath.Join(root, "src", "Foo.php"); input["file_path"] != want {
		t.Errorf("file_path = %v, want %q", input["file_path"], want)
	}
	if doc["hook_event_name"] != "PreToolUse" || doc["cwd"] != root {
		t.Errorf("hook_event_name=%v cwd=%v", doc["hook_event_name"], doc["cwd"])
	}
}

func TestBuild_CodexEditSendsAnApplyPatchThatAddsOrUpdates(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "old.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, header string }{
		{"old.txt", "*** Update File: old.txt\n"},
		{filepath.Join("src", "new.txt"), "*** Add File: src/new.txt\n"},
	} {
		p, err := Build("codex", "PostToolUse", "Write", root, Input{Edit: tc.path})
		if err != nil {
			t.Fatal(err)
		}
		doc := decode(t, p.Body)
		input, _ := doc["tool_input"].(map[string]any)
		patch, _ := input["command"].(string)
		if !p.Fires || doc["tool_name"] != "apply_patch" {
			t.Errorf("fires=%v tool_name=%v, want apply_patch through the Write alias", p.Fires, doc["tool_name"])
		}
		if !strings.HasPrefix(patch, "*** Begin Patch\n"+tc.header) || !strings.HasSuffix(patch, "*** End Patch\n") {
			t.Errorf("patch = %q, want header %q", patch, tc.header)
		}
		if _, ok := doc["tool_response"]; !ok {
			t.Error("PostToolUse payload has no tool_response")
		}
	}
}

func TestBuild_MatcherThatMissesTheToolDoesNotFire(t *testing.T) {
	for _, tc := range []struct{ target, matcher string }{
		{"claude", "Bash"},
		{"codex", "MultiEdit"},
	} {
		p, err := Build(tc.target, "PreToolUse", tc.matcher, t.TempDir(), Input{Edit: "a.go"})
		if err != nil {
			t.Fatal(err)
		}
		if p.Fires {
			t.Errorf("%s matcher %q fired on an edit", tc.target, tc.matcher)
		}
	}
}

func TestBuild_BashAndPromptAndSessionSource(t *testing.T) {
	root := t.TempDir()
	bash, err := Build("codex", "PreToolUse", "Bash", root, Input{Bash: "git push --force"})
	if err != nil {
		t.Fatal(err)
	}
	if input, _ := decode(t, bash.Body)["tool_input"].(map[string]any); input["command"] != "git push --force" {
		t.Errorf("tool_input = %v", input)
	}
	prompt, err := Build("claude", "UserPromptSubmit", "", root, Input{Prompt: "ship it"})
	if err != nil {
		t.Fatal(err)
	}
	if decode(t, prompt.Body)["prompt"] != "ship it" {
		t.Errorf("prompt payload = %s", prompt.Body)
	}
	start, err := Build("claude", "SessionStart", "compact", root, Input{})
	if err != nil {
		t.Fatal(err)
	}
	if !start.Fires || decode(t, start.Body)["source"] != "compact" {
		t.Errorf("SessionStart payload = %s, want source compact", start.Body)
	}
}

func TestBuild_RejectsAnEventWithoutABuilderAndAMismatchedInput(t *testing.T) {
	for _, tc := range []struct {
		event string
		in    Input
		want  string
	}{
		{"Stop", Input{}, "--payload"},
		{"PreToolUse", Input{}, "--edit"},
		{"SessionStart", Input{Edit: "a.go"}, "--edit"},
		{"PreToolUse", Input{Prompt: "x"}, "--prompt"},
	} {
		if _, err := Build("claude", tc.event, "", t.TempDir(), tc.in); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s %+v: err = %v, want it to name %s", tc.event, tc.in, err, tc.want)
		}
	}
}

func TestBuild_RawPayloadPassesThrough(t *testing.T) {
	raw := []byte(`{"tool_name":"Bash"}`)
	p, err := Build("codex", "Stop", "", t.TempDir(), Input{Raw: raw})
	if err != nil {
		t.Fatal(err)
	}
	if !p.Fires || string(p.Body) != string(raw) {
		t.Errorf("payload = %s fires=%v", p.Body, p.Fires)
	}
}

func TestArgv_PicksEachTargetsRunnerPerPlatform(t *testing.T) {
	for _, tc := range []struct {
		name, target, goos string
		h                  Handler
		want               []string
	}{
		{"claude shell form", "claude", "linux", Handler{Command: "echo hi"}, []string{"bash", "-c", "echo hi"}},
		{"claude on Windows uses Git Bash", "claude", "windows", Handler{Command: "echo hi"}, []string{"bash", "-c", "echo hi"}},
		{"claude exec form", "claude", "windows", Handler{Command: "guard.exe", Args: []string{"--deny", "a b"}}, []string{"guard.exe", "--deny", "a b"}},
		{"claude PowerShell", "claude", "windows", Handler{Command: "Write-Output hi", Shell: "powershell"}, []string{"powershell.exe", "-NoProfile", "-Command", "Write-Output hi"}},
		{"codex POSIX", "codex", "darwin", Handler{Command: "export AGNOSTIC_AI_TARGET=codex; echo hi", CommandWindows: "echo win"}, []string{"sh", "-c", "export AGNOSTIC_AI_TARGET=codex; echo hi"}},
		{"codex Windows runs commandWindows", "codex", "windows", Handler{Command: "export AGNOSTIC_AI_TARGET=codex; echo hi", CommandWindows: "echo win"}, []string{"powershell.exe", "-NoProfile", "-Command", "echo win"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Argv(tc.target, tc.goos, tc.h); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Argv = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecide_ReadsExitCodesAndJSONReplies(t *testing.T) {
	for _, tc := range []struct {
		name, event string
		r           Result
		want        Decision
	}{
		{"exit 0", "PreToolUse", Result{Exit: 0}, Allow},
		{"exit 2 blocks a tool call", "PreToolUse", Result{Exit: 2}, Block},
		{"exit 2 at session start only warns", "SessionStart", Result{Exit: 2}, Error},
		{"exit 1 is a non-blocking error", "PostToolUse", Result{Exit: 1}, Error},
		{"deny reply", "PreToolUse", Result{Stdout: `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny"}}`}, Block},
		{"block reply", "UserPromptSubmit", Result{Stdout: `{"decision":"block","reason":"no"}`}, Block},
		{"stop reply", "PostToolUse", Result{Stdout: `{"continue":false}`}, Block},
		{"allow reply", "PreToolUse", Result{Stdout: `{"hookSpecificOutput":{"permissionDecision":"allow"}}`}, Allow},
		{"timeout", "PreToolUse", Result{TimedOut: true}, Timeout},
		{"start failure", "PreToolUse", Result{StartErr: os.ErrNotExist}, Error},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Decide(tc.event, tc.r); got != tc.want {
				t.Errorf("Decide = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestAddsContext_PlainStdoutOnContextEventsAndJSONAdditionalContext(t *testing.T) {
	for _, tc := range []struct {
		name, event string
		r           Result
		want        bool
	}{
		{"session start text", "SessionStart", Result{Stdout: "branch main\n"}, true},
		{"prompt text", "UserPromptSubmit", Result{Stdout: "note"}, true},
		{"tool call text", "PreToolUse", Result{Stdout: "note"}, false},
		{"JSON context", "PostToolUse", Result{Stdout: `{"hookSpecificOutput":{"additionalContext":"lint passed"}}`}, true},
		{"JSON without context", "SessionStart", Result{Stdout: `{"continue":true}`}, false},
		{"failed hook", "SessionStart", Result{Exit: 1, Stdout: "x"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := AddsContext(tc.event, tc.r); got != tc.want {
				t.Errorf("AddsContext = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRun_KillsAHookAtItsTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	start := time.Now()
	r := Run([]string{"sh", "-c", "sleep 5; echo late"}, t.TempDir(), os.Environ(), nil, 200*time.Millisecond)
	if !r.TimedOut || strings.Contains(r.Stdout, "late") {
		t.Errorf("result = %+v, want a timeout before the output", r)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("Run took %s, want it to stop at the timeout", elapsed)
	}
}

func TestRun_ABackgroundChildDoesNotTurnAnExitIntoAStartFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	r := Run([]string{"sh", "-c", "sleep 3 & echo started"}, t.TempDir(), os.Environ(), nil, time.Minute)
	if r.StartErr != nil || r.Exit != 0 || r.TimedOut || !strings.Contains(r.Stdout, "started") {
		t.Errorf("result = %+v, want exit 0", r)
	}
}

func TestRun_FeedsThePayloadAndReportsTheExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	r := Run([]string{"sh", "-c", "cat; echo oops >&2; exit 3"}, t.TempDir(), os.Environ(), []byte(`{"a":1}`), time.Minute)
	if r.Exit != 3 || r.Stdout != `{"a":1}` || r.Stderr != "oops\n" || r.StartErr != nil {
		t.Errorf("result = %+v", r)
	}
}
