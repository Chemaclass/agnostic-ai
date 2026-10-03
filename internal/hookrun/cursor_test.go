package hookrun

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDecideCursor_FollowsTheDocumentedRules(t *testing.T) {
	for name, tc := range map[string]struct {
		event string
		h     Handler
		r     Result
		want  Decision
	}{
		"exit 2 blocks a permission hook":      {"beforeShellExecution", Handler{}, Result{Exit: 2}, Block},
		"deny blocks":                          {"beforeShellExecution", Handler{}, Result{Stdout: `{"permission":"deny"}`}, Block},
		"allow allows":                         {"beforeShellExecution", Handler{}, Result{Stdout: `{"permission":"allow"}`}, Allow},
		"no output fails open":                 {"beforeShellExecution", Handler{}, Result{}, Error},
		"no output blocks with failClosed":     {"beforeShellExecution", Handler{FailClosed: true}, Result{}, Block},
		"text that is not JSON blocks":         {"beforeShellExecution", Handler{}, Result{Stdout: "ok"}, Block},
		"an unknown permission blocks":         {"beforeShellExecution", Handler{}, Result{Stdout: `{"permission":"maybe"}`}, Block},
		"a reply without permission allows":    {"beforeShellExecution", Handler{}, Result{Stdout: `{"user_message":"hi"}`}, Allow},
		"ask blocks a shell command":           {"beforeShellExecution", Handler{}, Result{Stdout: `{"permission":"ask"}`}, Block},
		"ask is not enforced on preToolUse":    {"preToolUse", Handler{}, Result{Stdout: `{"permission":"ask"}`}, Allow},
		"another exit fails open":              {"beforeShellExecution", Handler{}, Result{Exit: 1}, Error},
		"failClosed blocks on a failure":       {"beforeShellExecution", Handler{FailClosed: true}, Result{Exit: 1}, Block},
		"failClosed blocks on a timeout":       {"beforeShellExecution", Handler{FailClosed: true}, Result{TimedOut: true}, Block},
		"a timeout fails open":                 {"beforeShellExecution", Handler{}, Result{TimedOut: true}, Timeout},
		"empty output allows a non-permission": {"afterFileEdit", Handler{}, Result{}, Allow},
		"exit 2 cannot block afterFileEdit":    {"afterFileEdit", Handler{}, Result{Exit: 2}, Error},
		"continue false blocks a prompt":       {"beforeSubmitPrompt", Handler{}, Result{Stdout: `{"continue":false}`}, Block},
		"permission is case sensitive":         {"beforeShellExecution", Handler{}, Result{Stdout: `{"permission":"Deny"}`}, Block},
		"a wrong permission type blocks":       {"beforeShellExecution", Handler{}, Result{Stdout: `{"permission":true}`}, Block},
		"a wrong user_message type blocks":     {"beforeShellExecution", Handler{}, Result{Stdout: `{"permission":"allow","user_message":5}`}, Block},
		"a wrong agent_message type blocks":    {"beforeShellExecution", Handler{}, Result{Stdout: `{"permission":"allow","agent_message":["no"]}`}, Block},
		"a wrong updated_input type blocks":    {"preToolUse", Handler{}, Result{Stdout: `{"permission":"allow","updated_input":"npm ci"}`}, Block},
		"an updated_input object allows":       {"preToolUse", Handler{}, Result{Stdout: `{"permission":"allow","updated_input":{"command":"npm ci"}}`}, Allow},
		"a field Cursor does not list allows":  {"beforeShellExecution", Handler{}, Result{Stdout: `{"permission":"allow","note":1}`}, Allow},
	} {
		t.Run(name, func(t *testing.T) {
			if got := decideCursor(tc.event, tc.h, tc.r); got != tc.want {
				t.Errorf("decideCursor = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestBuildCursor_MatchesWhatEachEventDocuments(t *testing.T) {
	p, err := buildCursor("beforeShellExecution", "curl|wget", "/project", Input{Bash: "curl example.com"})
	if err != nil || !p.Fires {
		t.Fatalf("shell matcher runs against the command: %+v %v", p, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(p.Body, &doc); err != nil || doc["command"] != "curl example.com" || doc["cwd"] != "/project" || doc["hook_event_name"] != "beforeShellExecution" {
		t.Errorf("payload = %s", p.Body)
	}
	if p, _ := buildCursor("preToolUse", "Read", "/project", Input{Bash: "ls"}); p.Fires {
		t.Error("a Read matcher must not fire for the Shell tool")
	}
	if _, err := buildCursor("preToolUse", "", "/project", Input{Edit: "a.go"}); err == nil {
		t.Error("--edit on preToolUse must be refused: the Write tool input is undocumented")
	}
}

func TestBuild_MatchesARawCursorPayloadAsCursorDoes(t *testing.T) {
	for name, tc := range map[string]struct {
		event, matcher, body string
		want                 bool
	}{
		"tool event, other tool":      {"preToolUse", "Read", `{"tool_name":"Shell","tool_input":{"command":"ls"}}`, false},
		"shell event, other command":  {"beforeShellExecution", "curl|wget", `{"command":"ls -la"}`, false},
		"subagent, other type":        {"subagentStart", "explore", `{"subagent_type":"shell"}`, false},
		"subagent, same type":         {"subagentStop", "explore|shell", `{"subagent_type":"explore"}`, true},
		"beforeReadFile is Read":      {"beforeReadFile", "Read", `{"file_path":"/a"}`, true},
		"beforeReadFile is not Write": {"beforeReadFile", "Write", `{"file_path":"/a"}`, false},
		"Tab read is TabRead":         {"beforeTabFileRead", "^Read$", `{"file_path":"/a"}`, false},
		"Tab edit is TabWrite":        {"afterTabFileEdit", "TabWrite", `{"file_path":"/a"}`, true},
		"stop is Stop":                {"stop", "AgentResponse", `{"status":"completed"}`, false},
		"agent response":              {"afterAgentResponse", "AgentResponse", `{"text":"x"}`, true},
		"agent thought":               {"afterAgentThought", "AgentResponse", `{"text":"x"}`, false},
		"event with no match value":   {"sessionStart", "anything", `{"session_id":"s"}`, true},
	} {
		t.Run(name, func(t *testing.T) {
			p, err := Build("cursor", tc.event, tc.matcher, "/project", Input{Raw: []byte(tc.body)})
			if err != nil || p.Fires != tc.want {
				t.Errorf("Fires = %t (%v), want %t", p.Fires, err, tc.want)
			}
		})
	}
}

func TestShellNeutral_AcceptsTheQuotingSyncWrites(t *testing.T) {
	for command, want := range map[string]bool{
		".cursor/hooks/a.sh":                          true,
		".cursor/hooks/a.sh '--strict' 'a b' '$HOME'": true,
		".cursor/hooks/a.sh --level=3 50% a,b":        true,
		".cursor/hooks/a.sh 'it'\\''s'":               false,
		".cursor/hooks/a.sh 'open":                    false,
		".cursor/hooks/a.sh $HOME":                    false,
		"cat | grep x":                                false,
		"":                                            false,
	} {
		if got := ShellNeutral(command); got != want {
			t.Errorf("ShellNeutral(%q) = %t, want %t", command, got, want)
		}
	}
}

func TestFireAndForget_CursorSessionEvents(t *testing.T) {
	if !FireAndForget("cursor", "sessionStart") || !FireAndForget("cursor", "sessionEnd") {
		t.Error("Cursor documents sessionStart and sessionEnd as fire-and-forget")
	}
	if FireAndForget("cursor", "beforeShellExecution") || FireAndForget("claude", "SessionStart") {
		t.Error("only Cursor's session events are fire-and-forget")
	}
}

func TestAssumptions_CursorRunsOnlyShellNeutralCommandsOnUnix(t *testing.T) {
	if _, reason := Assumptions("cursor", "windows", Handler{Command: ".cursor/hooks/a.sh"}); reason == "" {
		t.Error("Windows must not run: the shell is undocumented")
	}
	if _, reason := Assumptions("cursor", "linux", Handler{Command: "cat | grep x"}); reason == "" {
		t.Error("shell syntax must not run under an assumed shell")
	}
	got, reason := Assumptions("cursor", "linux", Handler{Command: ".cursor/hooks/a.sh --strict", Timeout: 5 * time.Second})
	if reason != "" || len(got) != 1 || got[0].Item != "shell" {
		t.Errorf("Assumptions = %+v %q; want only the shell, since the spec sets a timeout", got, reason)
	}
	if got, _ := Assumptions("claude", "linux", Handler{Command: "x | y"}); got != nil {
		t.Errorf("a documented target assumes nothing: %+v", got)
	}
}

func TestBuild_CursorPayloadFileHonorsTheMatcher(t *testing.T) {
	body := []byte(`{"command": "ls -la"}`)
	p, err := Build("cursor", "beforeShellExecution", "curl", "/project", Input{Raw: body})
	if err != nil || p.Fires {
		t.Errorf("a curl matcher must not fire on ls: %+v %v", p, err)
	}
	p, _ = Build("cursor", "preToolUse", "Read", "/project", Input{Raw: []byte(`{"tool_name": "Read"}`)})
	if !p.Fires {
		t.Errorf("a Read matcher must fire on a Read tool payload: %+v", p)
	}
}
