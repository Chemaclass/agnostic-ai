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
