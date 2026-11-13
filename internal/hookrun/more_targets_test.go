package hookrun

import (
	"errors"
	"reflect"
	"slices"
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
		{"augment", "PreToolUse", "bash", "hook_event_name", "command"},
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
	if input, _ := doc["tool_input"].(map[string]any); doc["tool_name"] != "edit" || input["path"] != "src/a.go" || input["edits"] == nil || len(changes) != 1 || doc["tool_is_error"] != false || doc["tool_result"] == nil {
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
		t.Error("augment built UserPromptSubmit, an event it does not have")
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
		{"augment", "^bash$", true},
		{"augment", "Bash", true},
		{"augment", "^launch-process$", true},
		{"augment", "edit|write", false},
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

func TestAddsContext_AugmentAndOpenHandsSessionStart(t *testing.T) {
	for _, tc := range []struct {
		target, event string
		r             Result
		want          bool
	}{
		{"augment", "SessionStart", Result{Stdout: "sprint 23\n"}, true},
		{"augment", "SessionStart", Result{Stdout: `{"hookSpecificOutput":{"additionalContext":"sprint 23"}}`}, true},
		{"augment", "SessionStart", Result{Stdout: `{"hookSpecificOutput":{"hookEventName":"SessionStart"}}`}, false},
		{"augment", "SessionStart", Result{Exit: 1, Stdout: "sprint 23"}, false},
		{"augment", "SessionStart", Result{Exit: 2, Stdout: "sprint 23"}, false},
		{"augment", "PostToolUse", Result{Stdout: `{"hookSpecificOutput":{"additionalContext":"lint passed"}}`}, true},
		{"augment", "PostToolUse", Result{Stdout: "lint passed"}, false},
		{"openhands", "SessionStart", Result{Stdout: `{"additionalContext":"sprint 23"}`}, true},
		{"openhands", "SessionStart", Result{TimedOut: true, Stdout: `{"additionalContext":"sprint 23"}`}, false},
		{"openhands", "SessionEnd", Result{Stdout: `{"additionalContext":"sprint 23"}`}, false},
		{"openhands", "SessionStart", Result{Stdout: `{"hookSpecificOutput":{"additionalContext":"sprint 23"}}`}, false},
		{"openhands", "SessionStart", Result{Stdout: "sprint 23"}, false},
		{"openhands", "UserPromptSubmit", Result{Stdout: `{"additionalContext":"sprint 23"}`}, true},
	} {
		if got := AddsContext(tc.target, tc.event, tc.r); got != tc.want {
			t.Errorf("%s %s %+v adds context = %v, want %v", tc.target, tc.event, tc.r, got, tc.want)
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
		{"augment", "linux", ".augment/hooks/guard.sh", []string{"/bin/sh", "-c", ".augment/hooks/guard.sh"}},
		{"augment", "windows", ".augment/hooks/guard.ps1", []string{"cmd.exe", "/d", "/s", "/c", ".augment/hooks/guard.ps1"}},
		{"augment", "windows", ".augment/hooks/guard.cmd", []string{"cmd.exe", "/d", "/s", "/c", ".augment/hooks/guard.cmd"}},
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

func TestHandlersFromDoc_AugmentKeepsArgs(t *testing.T) {
	got, err := HandlersFromDoc("augment", []byte(`{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"node","args":["check.js","--strict"]}]}]}}`))
	if err != nil || len(got) != 1 || !slices.Equal(got[0].Args, []string{"check.js", "--strict"}) {
		t.Errorf("augment = %+v, %v", got, err)
	}
}

// Auggie 1.x starts a command with args and no shell, and hands one
// without args to a shell.
func TestOtherArgv_Augment(t *testing.T) {
	for _, tc := range []struct {
		goos string
		h    Handler
		want []string
	}{
		{"linux", Handler{Command: "node", Args: []string{"check.js", "two words"}}, []string{"node", "check.js", "two words"}},
		{"linux", Handler{Command: "gofmt -l . | head"}, []string{"/bin/sh", "-c", "gofmt -l . | head"}},
		{"windows", Handler{Command: "guard.sh", Args: []string{"x"}}, []string{"bash.exe", "guard.sh", "x"}},
		{"windows", Handler{Command: "guard.ps1", Args: []string{"x"}}, []string{"powershell.exe", "-File", "guard.ps1", "x"}},
		{"windows", Handler{Command: "echo hi"}, []string{"cmd.exe", "/d", "/s", "/c", "echo hi"}},
		{"windows", Handler{Command: "guard.ps1"}, []string{"cmd.exe", "/d", "/s", "/c", "guard.ps1"}},
		{"windows", Handler{Command: "guard.bat", Args: []string{"a&b", "x y"}}, []string{"cmd.exe", "/d", "/s", "/v:off", "/c", "guard.bat", "a^&b", "x y"}},
	} {
		got, ok := otherArgv("augment", tc.goos, tc.h)
		if !ok || !slices.Equal(got, tc.want) {
			t.Errorf("%s %+v = %q, want %q", tc.goos, tc.h, got, tc.want)
		}
	}
}

// docs.augmentcode.com/cli/hooks.md: Auggie 1.x names its tools bash,
// edit, and write, matches case-insensitively, and still matches the
// 0.x names launch-process, str-replace-editor, and save-file.
func TestBuild_AugmentEditPicksTheToolTheMatcherNames(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		matcher, tool string
		fires         bool
	}{
		{"", "edit", true},
		{"^write$", "write", true},
		{"Write", "write", true},
		{"save-file", "write", true},
		{"str-replace-editor", "edit", true},
		{"bash", "edit", false},
	} {
		p, err := Build("augment", "PreToolUse", tc.matcher, root, Input{Edit: "src/a.go"})
		if err != nil {
			t.Fatal(err)
		}
		if doc := decode(t, p.Body); doc["tool_name"] != tc.tool || p.Fires != tc.fires {
			t.Errorf("matcher %q: tool %v fires %v, want %s %v", tc.matcher, doc["tool_name"], p.Fires, tc.tool, tc.fires)
		}
	}
}

// docs.augmentcode.com/cli/hooks.md: "Note that only `PreToolUse` can
// block tool execution. `Stop`, `SessionStart`, `SessionEnd`,
// `PromptSubmit`, and `Notification` hooks run in the background".
func TestDecide_AugmentBackgroundEventsCannotBlock(t *testing.T) {
	block := Result{Stdout: `{"decision":"block","continue":false,"hookSpecificOutput":{"permissionDecision":"deny","decision":"block"}}`}
	for _, tc := range []struct {
		event string
		want  Decision
		note  string
	}{
		{"PreToolUse", Block, ""},
		{"PostToolUse", Block, ""},
		{"Stop", Block, "Auggie 1.x runs Stop hooks in the background"},
		{"SessionStart", Allow, "Auggie runs SessionStart hooks in the background and cannot block them"},
		{"SessionEnd", Allow, "Auggie runs SessionEnd hooks in the background and cannot block them"},
		{"PromptSubmit", Allow, "Auggie runs PromptSubmit hooks in the background and cannot block them"},
		{"Notification", Allow, "Auggie runs Notification hooks in the background and cannot block them"},
	} {
		if got := DecideHandler("augment", tc.event, Handler{}, block); got != tc.want {
			t.Errorf("%s = %s, want %s", tc.event, got, tc.want)
		}
		note := AugmentNote(tc.event, block)
		if tc.note == "" && note != "" || !strings.Contains(note, tc.note) {
			t.Errorf("%s note = %q, want %q", tc.event, note, tc.note)
		}
		if note := AugmentNote(tc.event, Result{}); note != "" {
			t.Errorf("%s allow reply note = %q, want none", tc.event, note)
		}
	}
}
