package hookrun

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestBuild_TraeOpenHandsGooseAndAugmentShellPayloads(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		target, event, tool string
		eventKey, inputKey  string
	}{
		{"trae", "PreToolUse", "RunCommand", "hook_event_name", "command"},
		{"openhands", "PreToolUse", "terminal", "event_type", "command"},
		{"goose", "PreToolUse", "shell", "event", "command"},
		{"augment", "PreToolUse", "launch-process", "hook_event_name", "command"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			p, err := Build(tc.target, tc.event, "", root, Input{Bash: "git push"})
			if err != nil {
				t.Fatal(err)
			}
			doc := decode(t, p.Body)
			input, _ := doc["tool_input"].(map[string]any)
			if !p.Fires || doc["tool_name"] != tc.tool || doc[tc.eventKey] != tc.event || input[tc.inputKey] != "git push" {
				t.Errorf("payload = %s, want %s with tool_input.%s", p.Body, tc.tool, tc.inputKey)
			}
		})
	}
}

func TestBuild_EditPayloadsWhereTheToolInputIsDocumented(t *testing.T) {
	root := t.TempDir()
	goose, err := Build("goose", "PostToolUse", "edit", root, Input{Edit: "src/a.go"})
	if err != nil {
		t.Fatal(err)
	}
	doc := decode(t, goose.Body)
	if input, _ := doc["tool_input"].(map[string]any); doc["tool_name"] != "edit" || input["before"] == nil || !strings.HasSuffix(input["path"].(string), "a.go") {
		t.Errorf("goose edit payload = %s", goose.Body)
	}
	augment, err := Build("augment", "PostToolUse", "", root, Input{Edit: "src/a.go"})
	if err != nil {
		t.Fatal(err)
	}
	doc = decode(t, augment.Body)
	changes, _ := doc["file_changes"].([]any)
	if input, _ := doc["tool_input"].(map[string]any); doc["tool_name"] != "str-replace-editor" || input["path"] != "src/a.go" || len(changes) != 1 {
		t.Errorf("augment edit payload = %s", augment.Body)
	}
	for _, target := range []string{"trae", "openhands"} {
		if _, err := Build(target, "PreToolUse", "", root, Input{Edit: "a.go"}); !errors.As(err, new(Unbuilt)) || !strings.Contains(err.Error(), "--payload") {
			t.Errorf("%s: err = %v, want Unbuilt for an undocumented edit input", target, err)
		}
	}
}

func TestBuild_PromptAndSessionEvents(t *testing.T) {
	root := t.TempDir()
	oh, err := Build("openhands", "UserPromptSubmit", "", root, Input{Prompt: "hi"})
	if err != nil || decode(t, oh.Body)["message"] != "hi" {
		t.Errorf("openhands prompt = %s, %v", oh.Body, err)
	}
	gp, err := Build("goose", "UserPromptSubmit", "^deploy", root, Input{Prompt: "deploy now"})
	if err != nil || !gp.Fires || decode(t, gp.Body)["matcher_context"] != "deploy now" {
		t.Errorf("goose prompt = %s fires=%v, %v", gp.Body, gp.Fires, err)
	}
	if _, err := Build("augment", "UserPromptSubmit", "", root, Input{Prompt: "hi"}); err == nil {
		t.Error("augment built a prompt event it does not have")
	}
	// OpenHands matches a matcher only against a tool name; with none,
	// only `*` or an empty matcher runs (hooks/config.py HookMatcher).
	if s, _ := Build("openhands", "SessionStart", "terminal", root, Input{}); s.Fires {
		t.Error("openhands SessionStart fired with a tool matcher")
	}
}

func TestBuild_MatcherRulesPerTarget(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		target, matcher string
		fires           bool
	}{
		{"openhands", "term", false},
		{"openhands", "term.*", true},
		{"openhands", "/termi.al/", true},
		{"trae", "Run", true},
		{"goose", "*", false},
		{"goose", "she", true},
		{"augment", "launch", true},
	} {
		p, err := Build(tc.target, "PreToolUse", tc.matcher, root, Input{Bash: "ls"})
		if err != nil {
			t.Fatal(err)
		}
		if p.Fires != tc.fires {
			t.Errorf("%s matcher %q fires = %v, want %v", tc.target, tc.matcher, p.Fires, tc.fires)
		}
	}
}

func TestDecide_MoreTargets(t *testing.T) {
	for _, tc := range []struct {
		target, event string
		r             Result
		h             Handler
		want          Decision
	}{
		{"trae", "PreToolUse", Result{Exit: 2}, Handler{}, Block},
		{"trae", "PreToolUse", Result{Stdout: `{"hookSpecificOutput":{"permissionDecision":"deny"}}`}, Handler{}, Block},
		{"openhands", "PreToolUse", Result{Exit: 1, Stdout: `{"decision":"deny"}`}, Handler{}, Block},
		{"openhands", "PostToolUse", Result{Exit: 2}, Handler{}, Error},
		{"openhands", "UserPromptSubmit", Result{Stdout: `{"continue":false}`}, Handler{}, Block},
		{"goose", "PreToolUse", Result{Exit: 1, Stdout: `{"decision":"block"}`}, Handler{}, Block},
		{"goose", "PreToolUse", Result{Stdout: "log line"}, Handler{}, Error},
		{"goose", "PreToolUse", Result{Stdout: "log line"}, Handler{FailClosed: true}, Block},
		{"goose", "PreToolUse", Result{Stdout: `{"decision":"allow"}`}, Handler{}, Allow},
		{"goose", "PostToolUse", Result{Exit: 2}, Handler{}, Error},
		{"augment", "PreToolUse", Result{Exit: 2}, Handler{}, Block},
		{"augment", "PostToolUse", Result{Exit: 2}, Handler{}, Error},
		{"augment", "PostToolUse", Result{Stdout: `{"decision":"block"}`}, Handler{}, Block},
	} {
		if got := DecideHandler(tc.target, tc.event, tc.h, tc.r); got != tc.want {
			t.Errorf("%s %s %+v = %s, want %s", tc.target, tc.event, tc.r, got, tc.want)
		}
	}
}

func TestArgv_MoreTargets(t *testing.T) {
	for _, tc := range []struct {
		target, goos, command string
		want                  []string
	}{
		{"trae", "linux", "echo hi", []string{"bash", "-c", "echo hi"}},
		{"trae", "windows", "echo hi", []string{"powershell.exe", "-NoProfile", "-Command", "echo hi"}},
		{"openhands", "darwin", "echo hi", []string{"/bin/sh", "-c", "echo hi"}},
		{"openhands", "windows", "echo hi", []string{"cmd.exe", "/c", "echo hi"}},
		{"goose", "windows", "echo hi", []string{"sh", "-c", "echo hi"}},
		{"augment", "linux", ".augment/hooks/guard.sh", []string{".augment/hooks/guard.sh"}},
		{"augment", "windows", ".augment/hooks/guard.ps1", []string{"powershell.exe", "-Command", ".augment/hooks/guard.ps1"}},
		{"augment", "windows", ".augment/hooks/guard.cmd", []string{"cmd.exe", "/c", ".augment/hooks/guard.cmd"}},
	} {
		if got := Argv(tc.target, tc.goos, Handler{Command: tc.command}); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s on %s: Argv = %q, want %q", tc.target, tc.goos, got, tc.want)
		}
	}
}

func TestHandlersFromDoc_ReadsTimeoutUnitsAndFailurePolicy(t *testing.T) {
	doc := []byte(`{"hooks":{"PreToolUse":[{"matcher":"shell","hooks":[{"type":"command","command":"a","timeout":5,"on_failure":"block"}]}]}}`)
	got, err := HandlersFromDoc("goose", doc)
	if err != nil || len(got) != 1 || got[0].Timeout.Seconds() != 5 || !got[0].FailClosed {
		t.Errorf("goose = %+v, %v", got, err)
	}
	got, err = HandlersFromDoc("augment", []byte(`{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"a.sh","timeout":5000}]}]}}`))
	if err != nil || len(got) != 1 || got[0].Timeout.Seconds() != 5 {
		t.Errorf("augment = %+v, %v", got, err)
	}
}

func TestAugmentRunsOnlyScriptPaths(t *testing.T) {
	if AugmentRuns("echo hi") || !AugmentRuns(".augment/hooks/guard.sh") {
		t.Error("AugmentRuns misreads the script extension rule")
	}
}
