package hookrun

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestReadAntigravity_CountsOnlyTheDocumentedReplies(t *testing.T) {
	for name, tc := range map[string]struct {
		event     string
		r         Result
		want      Decision
		uncounted bool
	}{
		"allow allows":                          {"PreToolUse", Result{Stdout: `{"decision":"allow"}`}, Allow, false},
		"deny blocks":                           {"PreToolUse", Result{Stdout: `{"decision":"deny","reason":"no"}`}, Block, false},
		"deny_unless_prior_grant blocks":        {"PreToolUse", Result{Stdout: `{"decision":"deny_unless_prior_grant"}`}, Block, false},
		"ask blocks until the user answers":     {"PreToolUse", Result{Stdout: `{"decision":"ask"}`}, Block, false},
		"force_ask blocks too":                  {"PreToolUse", Result{Stdout: `{"decision":"force_ask"}`}, Block, false},
		"string overrides are documented":       {"PreToolUse", Result{Stdout: `{"decision":"allow","permissionOverrides":["command(ls)"]}`}, Allow, false},
		"Stop continue keeps the agent running": {"Stop", Result{Stdout: `{"decision":"continue"}`}, Block, false},
		"Stop with another value stops":         {"Stop", Result{Stdout: `{"decision":"done"}`}, Allow, false},
		"PostToolUse returns an empty object":   {"PostToolUse", Result{Stdout: `{}`}, Allow, false},
		"a non-zero exit":                       {"PreToolUse", Result{Exit: 2}, Error, true},
		"a non-zero exit with a deny reply":     {"PreToolUse", Result{Exit: 1, Stdout: `{"decision":"deny"}`}, Error, true},
		"no output":                             {"PreToolUse", Result{}, Error, true},
		"plain text":                            {"PostToolUse", Result{Stdout: "done"}, Error, true},
		"invalid JSON":                          {"PreToolUse", Result{Stdout: `{"decision":`}, Error, true},
		"no decision":                           {"PreToolUse", Result{Stdout: `{"reason":"x"}`}, Error, true},
		"a decision that is not a string":       {"PreToolUse", Result{Stdout: `{"decision":true}`}, Error, true},
		"an undocumented decision":              {"PreToolUse", Result{Stdout: `{"decision":"block"}`}, Error, true},
		"a reason that is not a string":         {"PreToolUse", Result{Stdout: `{"decision":"allow","reason":1}`}, Error, true},
		"overrides that are not strings":        {"PreToolUse", Result{Stdout: `{"decision":"allow","permissionOverrides":[1]}`}, Error, true},
		"a Stop reply without decision":         {"Stop", Result{Stdout: `{}`}, Error, true},
		"a timeout":                             {"PreToolUse", Result{TimedOut: true}, Timeout, true},
		"a command that did not start":          {"PreToolUse", Result{StartErr: errors.New("missing")}, Error, true},
	} {
		t.Run(name, func(t *testing.T) {
			got := readAntigravity(tc.event, tc.r)
			if got.decision != tc.want || (got.uncounted != "") != tc.uncounted {
				t.Errorf("readAntigravity = %+v, want %s, uncounted %t", got, tc.want, tc.uncounted)
			}
			if DecideHandler("antigravity", tc.event, Handler{}, tc.r) != tc.want {
				t.Errorf("DecideHandler disagrees with readAntigravity")
			}
		})
	}
	if got := AntigravityUncounted("PreToolUse", Result{Exit: 1}); got != "Antigravity does not document exit codes" {
		t.Errorf("AntigravityUncounted = %q", got)
	}
	if AntigravityNote("PreToolUse", Result{Stdout: `{"decision":"deny_unless_prior_grant"}`}) == "" {
		t.Error("deny_unless_prior_grant must explain why it reads as block")
	}
}

func TestBuildAntigravity_WritesTheDocumentedPayloads(t *testing.T) {
	p, err := buildAntigravity("PreToolUse", "run_command", "/project", Input{Bash: "ls"})
	if err != nil || !p.Fires || p.Trigger != "run_command" {
		t.Fatalf("--bash calls run_command: %+v %v", p, err)
	}
	var doc struct {
		ConversationID string   `json:"conversationId"`
		WorkspacePaths []string `json:"workspacePaths"`
		StepIdx        *int     `json:"stepIdx"`
		Error          *string  `json:"error"`
		ToolCall       struct {
			Name string         `json:"name"`
			Args map[string]any `json:"args"`
		} `json:"toolCall"`
	}
	if err := json.Unmarshal(p.Body, &doc); err != nil || doc.ConversationID == "" || len(doc.WorkspacePaths) != 1 || doc.StepIdx == nil ||
		doc.ToolCall.Args["CommandLine"] != "ls" || doc.ToolCall.Args["Cwd"] != "/project" || doc.Error != nil {
		t.Errorf("payload = %s", p.Body)
	}

	p, err = buildAntigravity("PostToolUse", "replace_file_content|multi_replace_file_content", "/project", Input{Edit: "a.go"})
	if err != nil || !p.Fires || p.Trigger != "replace_file_content" {
		t.Fatalf("--edit calls the first edit tool the matcher matches: %+v %v", p, err)
	}
	if err := json.Unmarshal(p.Body, &doc); err != nil || doc.ToolCall.Args["TargetFile"] != filepath.Join("/project", "a.go") || doc.Error == nil {
		t.Errorf("payload = %s", p.Body)
	}

	if p, _ := buildAntigravity("PreToolUse", "run", "/project", Input{Bash: "ls"}); p.Fires {
		t.Error("the matcher is anchored: run must not match run_command")
	}
	if p, _ := buildAntigravity("PreToolUse", "", "/project", Input{Edit: "a.go"}); !p.Fires || p.Trigger != "write_to_file" {
		t.Errorf("an empty matcher fires on write_to_file first: %+v", p)
	}
	var unbuilt Unbuilt
	if _, err := buildAntigravity("PreToolUse", "(", "/project", Input{Bash: "ls"}); !errors.As(err, &unbuilt) {
		t.Errorf("a matcher that does not compile must leave the target unbuilt: %v", err)
	}
	if _, err := buildAntigravity("Stop", "", "/project", Input{}); err == nil {
		t.Error("Stop needs --payload")
	}
}

func TestAntigravityRawPayload_MatchesToolCallName(t *testing.T) {
	p, err := Build("antigravity", "PreToolUse", "view_file", "/project", Input{Raw: []byte(`{"toolCall":{"name":"view_file","args":{}}}`)})
	if err != nil || !p.Fires || p.Trigger != "view_file" {
		t.Errorf("payload tool call = %+v %v", p, err)
	}
	p, err = Build("antigravity", "Stop", "anything", "/project", Input{Raw: []byte(`{"executionNum":1}`)})
	if err != nil || !p.Fires {
		t.Errorf("Stop ignores the matcher: %+v %v", p, err)
	}
}

func TestAntigravityHooks_ReadsDefinitionsByName(t *testing.T) {
	body := []byte(`{
		"guard": {"PreToolUse": [{"matcher": "run_command", "hooks": [{"type": "command", "command": "./g.sh", "timeout": 5}]}]},
		"off": {"enabled": false, "PreToolUse": [{"matcher": "", "hooks": [{"type": "command", "command": "./off.sh"}]}]},
		"stop": {"Stop": [{"type": "command", "command": "./s.sh"}]}
	}`)
	handlers, err := HandlersFromDoc("antigravity", body)
	if err != nil || len(handlers) != 2 {
		t.Fatalf("handlers = %+v %v; want the enabled guard and stop", handlers, err)
	}
	drift, err := Drift("antigravity", body, "PreToolUse", "run_command", "darwin", []Handler{{Command: "./g.sh", Timeout: 5 * time.Second}}, func(n, s string) bool { return n == s })
	if err != nil || len(drift) != 0 {
		t.Errorf("drift = %+v %v", drift, err)
	}
	drift, _ = Drift("antigravity", body, "Stop", "", "darwin", []Handler{{Command: "./s.sh"}}, func(n, s string) bool { return n == s })
	if len(drift) != 0 {
		t.Errorf("Stop handlers sit directly under the event: %+v", drift)
	}
}
