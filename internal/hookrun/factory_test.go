package hookrun

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDecideFactory_FollowsTheHooksGuide(t *testing.T) {
	for name, tc := range map[string]struct {
		event string
		r     Result
		want  Decision
	}{
		"exit 2 blocks a tool call":                {"PreToolUse", Result{Exit: 2}, Block},
		"exit 2 blocks a prompt":                   {"UserPromptSubmit", Result{Exit: 2}, Block},
		"exit 2 feeds Stop back":                   {"Stop", Result{Exit: 2}, Block},
		"exit 2 only surfaces on SubagentStop":     {"SubagentStop", Result{Exit: 2}, Error},
		"exit 2 only surfaces on SessionStart":     {"SessionStart", Result{Exit: 2}, Error},
		"another exit is a non-blocking error":     {"PreToolUse", Result{Exit: 1}, Error},
		"deny blocks a tool call":                  {"PreToolUse", Result{Stdout: `{"hookSpecificOutput":{"permissionDecision":"deny"}}`}, Block},
		"ask blocks until the user answers":        {"PreToolUse", Result{Stdout: `{"hookSpecificOutput":{"permissionDecision":"ask"}}`}, Block},
		"allow allows":                             {"PreToolUse", Result{Stdout: `{"hookSpecificOutput":{"permissionDecision":"allow"}}`}, Allow},
		"decision block blocks SubagentStop":       {"SubagentStop", Result{Stdout: `{"decision":"block"}`}, Block},
		"decision block is not a PreToolUse reply": {"PreToolUse", Result{Stdout: `{"decision":"block"}`}, Allow},
		"continue false stops":                     {"PostToolUse", Result{Stdout: `{"continue":false}`}, Block},
		"SessionEnd cannot block":                  {"SessionEnd", Result{Stdout: `{"continue":false}`}, Allow},
		"plain stdout allows":                      {"UserPromptSubmit", Result{Stdout: "context"}, Allow},
		"a timeout":                                {"PreToolUse", Result{TimedOut: true}, Timeout},
	} {
		t.Run(name, func(t *testing.T) {
			if got := decideFactory(tc.event, tc.r); got != tc.want {
				t.Errorf("decideFactory = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestBuildFactory_WritesTheDocumentedPayloads(t *testing.T) {
	p, err := buildFactory("PreToolUse", "Execute", "/project", Input{Bash: "ls"})
	if err != nil || !p.Fires || p.Trigger != "Execute" {
		t.Fatalf("--bash calls Execute: %+v %v", p, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(p.Body, &doc); err != nil || doc["cwd"] != "/project" || doc["permission_mode"] != "off" || doc["tool_input"].(map[string]any)["command"] != "ls" {
		t.Errorf("payload = %s", p.Body)
	}
	p, err = buildFactory("PostToolUse", "Create|Edit|ApplyPatch", "/project", Input{Edit: "a.go"})
	if err != nil || !p.Fires || p.Trigger != "Create" {
		t.Fatalf("--edit calls Create, whose input is documented: %+v %v", p, err)
	}
	if err := json.Unmarshal(p.Body, &doc); err != nil || doc["tool_input"].(map[string]any)["file_path"] != "/project/a.go" || doc["tool_response"] == nil {
		t.Errorf("payload = %s", p.Body)
	}
	if _, err := buildFactory("PreToolUse", "ApplyPatch", "/project", Input{Edit: "a.go"}); err == nil {
		t.Error("--edit on ApplyPatch must be refused: its input is undocumented")
	}
	if p, _ := buildFactory("PreToolUse", "Exec", "/project", Input{Bash: "ls"}); p.Fires {
		t.Error("an exact matcher names one tool")
	}
	if p, _ := buildFactory("SessionStart", "resume", "/project", Input{}); !p.Fires || p.Trigger != "resume" {
		t.Errorf("a SessionStart matcher picks its source: %+v", p)
	}
}

func TestAssumptions_FactoryAssumesTheShellAndCwd(t *testing.T) {
	if _, reason := Assumptions("factory", "windows", Handler{Command: ".factory/hooks/a.sh"}); reason == "" {
		t.Error("Windows must not run: the shell is undocumented")
	}
	got, reason := Assumptions("factory", "linux", Handler{Command: ".factory/hooks/a.sh"})
	if reason != "" || len(got) != 2 || got[0].Item != "shell" || got[1].Item != "cwd" {
		t.Errorf("Assumptions = %+v %q; want the shell and cwd, not the documented 60s timeout", got, reason)
	}
	for _, command := range []string{`"$FACTORY_PROJECT_DIR"/.factory/hooks/a.sh`, `${FACTORY_PROJECT_DIR}/.factory/hooks/a.sh --strict`} {
		if _, reason := Assumptions("factory", "linux", Handler{Command: command}); reason != "" {
			t.Errorf("%s must run: %s", command, reason)
		}
	}
	if got := ExpandCommand("factory", "linux", `"$FACTORY_PROJECT_DIR"/a.sh`, "/it's a dir"); got != `'/it'\''s a dir'/a.sh` {
		t.Errorf("ExpandCommand = %s", got)
	}
	if DefaultTimeout("factory", "PreToolUse") != 60*time.Second {
		t.Error("Factory's documented default is 60 seconds")
	}
}
