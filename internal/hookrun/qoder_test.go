package hookrun

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestDecideQoder_FollowsTheHooksGuide(t *testing.T) {
	const deny = `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny"}}`
	for name, tc := range map[string]struct {
		event string
		r     Result
		want  Decision
	}{
		"exit 2 blocks a tool call":                   {"PreToolUse", Result{Exit: 2}, Block},
		"exit 2 blocks a prompt":                      {"UserPromptSubmit", Result{Exit: 2}, Block},
		"exit 2 keeps Stop working":                   {"Stop", Result{Exit: 2}, Block},
		"exit 2 cannot block PostToolUse":             {"PostToolUse", Result{Exit: 2}, Error},
		"exit 2 cannot block SessionStart":            {"SessionStart", Result{Exit: 2}, Error},
		"any failure fails WorktreeCreate":            {"WorktreeCreate", Result{Exit: 1}, Block},
		"another exit is a non-blocking error":        {"PreToolUse", Result{Exit: 1}, Error},
		"StopFailure ignores the exit code":           {"StopFailure", Result{Exit: 2}, Allow},
		"deny blocks a tool call":                     {"PreToolUse", Result{Stdout: deny}, Block},
		"ask blocks until the user answers":           {"PreToolUse", Result{Stdout: `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask"}}`}, Block},
		"permissionDecision takes precedence":         {"PreToolUse", Result{Stdout: `{"decision":"deny","hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow"}}`}, Allow},
		"decision deny is exit 2":                     {"UserPromptSubmit", Result{Stdout: `{"decision":"deny"}`}, Block},
		"decision deny cannot block PostToolUse":      {"PostToolUse", Result{Stdout: `{"decision":"deny"}`}, Allow},
		"continue false stops":                        {"PostToolUse", Result{Stdout: `{"continue":false}`}, Block},
		"a reply without hookEventName is rejected":   {"PreToolUse", Result{Stdout: `{"hookSpecificOutput":{"permissionDecision":"deny"}}`}, Error},
		"PermissionRequest blocks on behavior deny":   {"PermissionRequest", Result{Stdout: `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"deny"}}}`}, Block},
		"an elicitation declines":                     {"Elicitation", Result{Stdout: `{"hookSpecificOutput":{"hookEventName":"Elicitation","action":"decline"}}`}, Block},
		"plain stdout allows":                         {"UserPromptSubmit", Result{Stdout: "context"}, Allow},
		"a timeout":                                   {"PreToolUse", Result{TimedOut: true}, Timeout},
		"a command that did not start is not a block": {"PreToolUse", Result{StartErr: errors.New("missing")}, Error},
	} {
		t.Run(name, func(t *testing.T) {
			if got := decideQoder(tc.event, tc.r); got != tc.want {
				t.Errorf("decideQoder = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestBuildQoder_WritesTheDocumentedPayloads(t *testing.T) {
	p, err := buildQoder("PreToolUse", "Bash", "/project", Input{Bash: "ls"})
	if err != nil || !p.Fires || p.Trigger != "Bash" {
		t.Fatalf("--bash calls Bash: %+v %v", p, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(p.Body, &doc); err != nil || doc["cwd"] != "/project" || doc["permission_mode"] != "default" || doc["tool_input"].(map[string]any)["command"] != "ls" {
		t.Errorf("payload = %s", p.Body)
	}
	p, err = buildQoder("PostToolUse", "Write|Edit", "/project", Input{Edit: "a.go"})
	if err != nil || !p.Fires || p.Trigger != "Write" {
		t.Fatalf("--edit calls Write, whose input is documented: %+v %v", p, err)
	}
	if err := json.Unmarshal(p.Body, &doc); err != nil || doc["tool_input"].(map[string]any)["file_path"] != filepath.Join("/project", "a.go") || doc["tool_response"] == nil {
		t.Errorf("payload = %s", p.Body)
	}
	var unbuilt Unbuilt
	if _, err := buildQoder("PreToolUse", "Edit", "/project", Input{Edit: "a.go"}); !errors.As(err, &unbuilt) {
		t.Errorf("--edit on Edit must be unbuilt: its input is undocumented, got %v", err)
	}
	if p, _ := buildQoder("PreToolUse", "Bas", "/project", Input{Bash: "ls"}); p.Fires {
		t.Error("an exact matcher names one tool")
	}
	if p, _ := buildQoder("SessionStart", "new", "/project", Input{}); !p.Fires || p.Trigger != "new" {
		t.Errorf("a SessionStart matcher picks its source: %+v", p)
	}
	if _, err := Build("qoder", "Setup", "", "/project", Input{Raw: []byte(`{}`)}); !errors.As(err, &unbuilt) {
		t.Errorf("an event with undocumented decision rules must be unbuilt, got %v", err)
	}
}

func TestAssumptions_QoderAssumesTheDefaultShellAndCwd(t *testing.T) {
	got, reason := Assumptions("qoder", "linux", Handler{Command: ".qoder/hooks/a.sh"})
	if reason != "" || len(got) != 2 || got[0].Item != "shell" || got[0].Value != "sh -c" || got[1].Item != "working directory" {
		t.Errorf("Assumptions = %+v %q; want the shell and cwd, not the documented 600s timeout", got, reason)
	}
	if _, reason := Assumptions("qoder", "linux", Handler{Command: `"${QODER_PROJECT_DIR}"/.qoder/hooks/a.sh`}); reason != "" {
		t.Errorf("the guide's quoted root form must run: %s", reason)
	}
	if _, reason := Assumptions("qoder", "linux", Handler{Command: "cat | grep rm"}); reason != "Qoder does not document its shell; use a script path" {
		t.Errorf("shell syntax on the default shell: %q", reason)
	}
	if got, reason := Assumptions("qoder", "linux", Handler{Command: "cat | grep rm", Shell: "bash"}); reason != "" || len(got) != 1 {
		t.Errorf("shell: bash runs bash -c as documented: %+v %q", got, reason)
	}
	for _, h := range []Handler{{Command: ".qoder/hooks/a.sh"}, {Command: "a.sh", Shell: "bash"}, {Command: "a.cmd", Args: []string{"x"}}} {
		if _, reason := Assumptions("qoder", "windows", h); reason == "" {
			t.Errorf("%+v must not run on Windows", h)
		}
	}
	if _, reason := Assumptions("qoder", "windows", Handler{Command: "python3", Args: []string{"a.py"}}); reason != "" {
		t.Errorf("exec form runs with no shell on Windows too: %s", reason)
	}
	if _, reason := Assumptions("qoder", "linux", Handler{Command: "a.sh", Shell: "powershell"}); reason == "" {
		t.Error("an undocumented PowerShell must not run")
	}
	if DefaultTimeout("qoder", "PreToolUse") != 600*time.Second {
		t.Error("Qoder CLI's documented default is 600 seconds")
	}
	if argv := Argv("qoder", "linux", Handler{Command: "x", Shell: "bash"}); argv[0] != "bash" {
		t.Errorf("shell: bash argv = %v", argv)
	}
}

func TestQoderIfRuns_MatchesToolAndPrimaryArgument(t *testing.T) {
	bash := []byte(`{"tool_name":"Bash","tool_input":{"command":"git push"}}`)
	write := []byte(`{"tool_name":"Write","tool_input":{"file_path":"/p/src/a.ts"}}`)
	for _, tc := range []struct {
		rule string
		body []byte
		want bool
	}{
		{"Bash", bash, true},
		{"Bash(git *)", bash, true},
		{"Bash(npm *)", bash, false},
		{"Write|Edit", write, true},
		{"Write(*.ts)", write, true},
		{"Edit(*.ts)", write, false},
		{"Bash", []byte(`{"prompt":"x"}`), false},
	} {
		if got, err := QoderIfRuns(tc.rule, tc.body); err != nil || got != tc.want {
			t.Errorf("QoderIfRuns(%q) = %t %v, want %t", tc.rule, got, err, tc.want)
		}
	}
	var unbuilt Unbuilt
	if _, err := QoderIfRuns("Grep(*.go)", []byte(`{"tool_name":"Grep","tool_input":{"pattern":"x"}}`)); !errors.As(err, &unbuilt) {
		t.Errorf("an undocumented primary argument must be unbuilt, got %v", err)
	}
}
