package hookrun

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestReadKiro_FollowsTheDocumentedExitCodes(t *testing.T) {
	for name, tc := range map[string]struct {
		event     string
		r         Result
		want      Decision
		uncounted bool
	}{
		"PreToolUse exit 0 allows":                       {"PreToolUse", Result{}, Allow, false},
		"PreToolUse exit 2 blocks":                       {"PreToolUse", Result{Exit: 2}, Block, false},
		"PreToolUse exit 1 is disputed":                  {"PreToolUse", Result{Exit: 1}, Error, true},
		"UserPromptSubmit exit 2 only blocks in the IDE": {"UserPromptSubmit", Result{Exit: 2}, Allow, false},
		"UserPromptSubmit exit 3 is disputed":            {"UserPromptSubmit", Result{Exit: 3}, Error, true},
		"PostToolUse exit 2 cannot block":                {"PostToolUse", Result{Exit: 2}, Error, false},
		"Stop block decision keeps the agent":            {"Stop", Result{Stdout: `{"decision":"block","reason":"run the tests"}`}, Block, false},
		"Stop with another reply stops":                  {"Stop", Result{Stdout: `{"decision":"done"}`}, Allow, false},
		"Stop with plain text stops":                     {"Stop", Result{Stdout: "block"}, Allow, false},
		"Stop exit 1 starts another turn":                {"Stop", Result{Exit: 1}, Block, false},
		"Stop exit 1 with a reply starts another turn":   {"Stop", Result{Exit: 1, Stdout: `{"decision":"block"}`}, Block, false},
		"Stop exit 2 leaves the response finished":       {"Stop", Result{Exit: 2}, Error, false},
		"a JSON reply does not block PreToolUse":         {"PreToolUse", Result{Stdout: `{"decision":"block"}`}, Allow, false},
		"a timeout":                                      {"PreToolUse", Result{TimedOut: true}, Timeout, false},
		"a command that did not start":                   {"UserPromptSubmit", Result{StartErr: errors.New("missing")}, Error, false},
	} {
		t.Run(name, func(t *testing.T) {
			got := readKiro(tc.event, tc.r)
			if got.decision != tc.want || (got.uncounted != "") != tc.uncounted {
				t.Errorf("readKiro = %+v, want %s, uncounted %t", got, tc.want, tc.uncounted)
			}
			if DecideHandler("kiro", tc.event, Handler{}, tc.r) != tc.want {
				t.Errorf("DecideHandler disagrees with readKiro")
			}
		})
	}
	if got := KiroUncounted("PreToolUse", Result{Exit: 1}); got != "Kiro's docs disagree on whether a non-zero exit other than 2 blocks" {
		t.Errorf("KiroUncounted = %q", got)
	}
	if KiroNote("Stop", Result{Stdout: `{"decision":"block"}`}) == "" {
		t.Error("a Stop block must explain why it reads as block")
	}
	if got := KiroNote("Stop", Result{Exit: 1}); !strings.Contains(got, "exit 1") || !strings.Contains(got, "keeps the agent running") {
		t.Errorf("a Stop exit 1 note = %q", got)
	}
	if got := KiroNote("UserPromptSubmit", Result{Exit: 2}); !strings.Contains(got, "CLI V3") || !strings.Contains(got, "only the IDE blocks") {
		t.Errorf("a prompt exit 2 note = %q", got)
	}
	if got := KiroNote("PreToolUse", Result{Exit: 2}); got != "" {
		t.Errorf("a PreToolUse block needs no note: %q", got)
	}
}

func TestKiroAddsContext_PromptStdoutAndAStopReason(t *testing.T) {
	if !AddsContext("kiro", "UserPromptSubmit", Result{Stdout: "remember the style guide"}) {
		t.Error("UserPromptSubmit stdout at exit 0 is added to the agent's context")
	}
	if AddsContext("kiro", "PreToolUse", Result{Stdout: "ignored"}) {
		t.Error("PreToolUse stdout is not added to the context")
	}
	if !AddsContext("kiro", "Stop", Result{Stdout: `{"decision":"block","reason":"run the tests"}`}) {
		t.Error("a Stop block reason is sent to the agent")
	}
	if AddsContext("kiro", "UserPromptSubmit", Result{Exit: 2, Stdout: "x"}) {
		t.Error("a failed run adds nothing")
	}
}

func TestBuildKiro_WritesTheDocumentedPayloads(t *testing.T) {
	p, err := Build("kiro", "UserPromptSubmit", "", "/project", Input{Prompt: "deploy now"})
	if err != nil || !p.Fires || p.Trigger != "prompt" {
		t.Fatalf("--prompt builds userPromptSubmit: %+v %v", p, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(p.Body, &doc); err != nil || doc["hook_event_name"] != "userPromptSubmit" || doc["cwd"] != "/project" ||
		doc["session_id"] != SessionID || doc["prompt"] != "deploy now" {
		t.Errorf("payload = %s", p.Body)
	}

	if p, _ := Build("kiro", "UserPromptSubmit", "^deploy", "/project", Input{Prompt: "please deploy"}); !p.Fires {
		t.Error("CLI V3 does not evaluate a UserPromptSubmit matcher")
	}

	p, err = Build("kiro", "Stop", "anything", "/project", Input{})
	if err != nil || !p.Fires {
		t.Fatalf("Stop needs no input and ignores the matcher: %+v %v", p, err)
	}
	if err := json.Unmarshal(p.Body, &doc); err != nil || doc["hook_event_name"] != "stop" || doc["assistant_response"] != "" {
		t.Errorf("payload = %s", p.Body)
	}

	var unbuilt Unbuilt
	for name, in := range map[string]Input{"--bash": {Bash: "ls"}, "--edit": {Edit: "a.go"}} {
		if _, err := Build("kiro", "PreToolUse", "", "/project", in); !errors.As(err, &unbuilt) {
			t.Errorf("%s must leave Kiro unbuilt: %v", name, err)
		}
	}
	if _, err := Build("kiro", "PostFileSave", "", "/project", Input{Raw: []byte(`{}`)}); !errors.As(err, &unbuilt) || unbuilt.Reason != "Kiro documents no payload for PostFileSave" {
		t.Errorf("an event with no documented payload must stay unbuilt: %v", err)
	}
	if _, err := Build("kiro", "PreToolUse", "(", "/project", Input{Raw: []byte(`{"tool_name":"read_file"}`)}); !errors.As(err, &unbuilt) {
		t.Errorf("a tool matcher that does not compile must leave Kiro unbuilt: %v", err)
	}
}

func TestKiroAssumptions_RunsOnlyACommandOnAnAssumedShell(t *testing.T) {
	for _, tc := range []struct {
		name, goos string
		h          Handler
		reason     string
	}{
		{"agent action", "darwin", Handler{Agent: true}, "Kiro runs an agent prompt, not a command"},
		{"confirm", "darwin", Handler{Command: "./a.sh", Confirm: true}, "Kiro asks the user before a hook with confirm runs"},
		{"windows", "windows", Handler{Command: "./a.sh"}, "Kiro does not document how it runs a hook command on Windows"},
		{"shell syntax", "linux", Handler{Command: "cat | grep x"}, "Kiro does not document its shell; use a script path"},
	} {
		if _, reason := Assumptions("kiro", tc.goos, tc.h); reason != tc.reason {
			t.Errorf("%s: reason = %q, want %q", tc.name, reason, tc.reason)
		}
	}
	got, reason := Assumptions("kiro", "darwin", Handler{Command: ".kiro/scripts/a.sh"})
	if reason != "" || len(got) != 1 || got[0].Item != "shell" || got[0].Value != "sh -c" {
		t.Errorf("a script path assumes the shell only: %+v %q", got, reason)
	}
	got, _ = Assumptions("kiro", "darwin", Handler{Command: ".kiro/scripts/a.sh", Untimed: true})
	if len(got) != 2 || got[1].Item != "timeout" || got[1].Value != "1m0s" {
		t.Errorf("timeout 0 runs with the 60s default, marked assumed: %+v", got)
	}
}

const kiroHookFile = `{"version": "v1", "hooks": [
	{"name": "guard", "trigger": "PreToolUse", "matcher": "shell", "action": {"type": "command", "command": "./g.sh"}, "timeout": 5},
	{"name": "guard-2", "trigger": "PreToolUse", "matcher": "shell", "action": {"type": "command", "command": "./untimed.sh"}, "timeout": 0},
	{"name": "off", "trigger": "PreToolUse", "action": {"type": "command", "command": "./off.sh"}, "enabled": false},
	{"name": "ask", "trigger": "Stop", "action": {"type": "command", "command": "./s.sh"}, "confirm": {"question": "Run?"}},
	{"name": "steer", "trigger": "Stop", "action": {"type": "agent", "prompt": "Check coverage"}}
]}`

func TestKiroHandlers_ReadsTheHookFile(t *testing.T) {
	handlers, err := KiroHandlers([]byte(kiroHookFile))
	if err != nil || len(handlers) != 4 {
		t.Fatalf("handlers = %+v %v; want every enabled entry", handlers, err)
	}
	if handlers[0].Command != "./g.sh" || handlers[0].Timeout != 5*time.Second || handlers[0].Untimed {
		t.Errorf("command entry = %+v", handlers[0])
	}
	if !handlers[1].Untimed || handlers[1].Timeout != 0 {
		t.Errorf("timeout 0 = %+v; want untimed", handlers[1])
	}
	if !handlers[2].Confirm || handlers[3].Command != "" || !handlers[3].Agent {
		t.Errorf("confirm and agent entries = %+v %+v", handlers[2], handlers[3])
	}
}

func TestKiroDrift_ComparesCommandMatcherTimeoutAndConfirm(t *testing.T) {
	same := func(n, s string) bool { return n == s }
	body := []byte(kiroHookFile)
	drift, err := Drift("kiro", body, "PreToolUse", "shell", "darwin", []Handler{{Command: "./g.sh", Timeout: 5 * time.Second}, {Command: "./untimed.sh", Untimed: true}}, same)
	if err != nil || len(drift) != 0 {
		t.Errorf("a synced file drifts: %+v %v", drift, err)
	}
	for _, tc := range []struct {
		event, matcher string
		h              Handler
		reason         string
	}{
		{"PreToolUse", "write", Handler{Command: "./g.sh", Timeout: 5 * time.Second}, `runs "./g.sh" with matcher "shell", not "write"`},
		{"PreToolUse", "shell", Handler{Command: "./g.sh"}, `runs "./g.sh" with timeout 5s, not default`},
		{"PreToolUse", "shell", Handler{Command: "./untimed.sh"}, `runs "./untimed.sh" with timeout 0 (none), not default`},
		{"PreToolUse", "", Handler{Command: "./off.sh"}, `has no PreToolUse command "./off.sh"`},
		{"Stop", "", Handler{Command: "./s.sh"}, `runs "./s.sh" with confirm true, not false`},
	} {
		drift, _ := Drift("kiro", body, tc.event, tc.matcher, "darwin", []Handler{tc.h}, same)
		if len(drift) != 1 || drift[0].Reason != tc.reason {
			t.Errorf("drift = %+v, want %q", drift, tc.reason)
		}
	}
	if drift, _ := Drift("kiro", body, "Stop", "", "darwin", []Handler{{Agent: true}}, same); len(drift) != 0 {
		t.Errorf("an agent action runs no command to compare: %+v", drift)
	}
}
