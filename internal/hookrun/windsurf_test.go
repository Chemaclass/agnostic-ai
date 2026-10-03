package hookrun

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestReadWindsurf_FollowsTheExitCodesAndReplies(t *testing.T) {
	for name, tc := range map[string]struct {
		event     string
		r         Result
		want      Decision
		uncounted bool
	}{
		"exit 0 allows":                          {"PreToolUse", Result{}, Allow, false},
		"exit 2 blocks":                          {"PreToolUse", Result{Exit: 2}, Block, false},
		"another exit is logged":                 {"PreToolUse", Result{Exit: 1}, Error, false},
		"decision block blocks":                  {"PreToolUse", Result{Stdout: `{"decision":"block","reason":"no"}`}, Block, false},
		"decision approve allows":                {"PermissionRequest", Result{Stdout: `{"decision":"approve"}`}, Allow, false},
		"plain stdout allows":                    {"PreToolUse", Result{Stdout: "checked"}, Allow, false},
		"a null decision allows":                 {"PreToolUse", Result{Stdout: `{"decision":null}`}, Allow, false},
		"Stop blocks on decision block":          {"Stop", Result{Stdout: `{"decision":"block"}`}, Block, false},
		"exit 2 blocks a prompt":                 {"UserPromptSubmit", Result{Exit: 2}, Block, false},
		"exit 2 with decision block agrees":      {"PreToolUse", Result{Exit: 2, Stdout: `{"decision":"block"}`}, Block, false},
		"exit 2 with decision approve":           {"PreToolUse", Result{Exit: 2, Stdout: `{"decision":"approve"}`}, Block, true},
		"exit 1 with decision block":             {"PreToolUse", Result{Exit: 1, Stdout: `{"decision":"block"}`}, Error, true},
		"an undocumented decision":               {"PreToolUse", Result{Stdout: `{"decision":"deny"}`}, Allow, true},
		"a decision that is not a string":        {"PreToolUse", Result{Stdout: `{"decision":true}`}, Allow, true},
		"exit 2 after the tool ran":              {"PostToolUse", Result{Exit: 2}, Block, true},
		"decision block on SessionStart":         {"SessionStart", Result{Stdout: `{"decision":"block"}`}, Block, true},
		"another exit after the tool ran":        {"PostToolUse", Result{Exit: 1}, Error, false},
		"PostCompaction allows on exit 0":        {"PostCompaction", Result{}, Allow, false},
		"a timeout":                              {"PreToolUse", Result{TimedOut: true}, Timeout, false},
		"a command that did not start":           {"PreToolUse", Result{StartErr: errors.New("missing")}, Error, false},
		"a reply that is not an object is plain": {"PreToolUse", Result{Stdout: `["block"]`}, Allow, false},
	} {
		t.Run(name, func(t *testing.T) {
			got := readWindsurf(tc.event, tc.r)
			if got.decision != tc.want || (got.uncounted != "") != tc.uncounted {
				t.Errorf("readWindsurf = %+v, want %s, uncounted %t", got, tc.want, tc.uncounted)
			}
			if DecideHandler("windsurf", tc.event, Handler{}, tc.r) != tc.want {
				t.Errorf("DecideHandler disagrees with readWindsurf")
			}
		})
	}
	if got := WindsurfUncounted("PostToolUse", Result{Exit: 2}); got != "Devin CLI does not document what a block does on PostToolUse" {
		t.Errorf("WindsurfUncounted = %q", got)
	}
	if got := WindsurfUncounted("PreToolUse", Result{Exit: 1, Stdout: `{"decision":"block"}`}); got != "Devin CLI does not document whether it reads a reply on a non-zero exit" {
		t.Errorf("WindsurfUncounted = %q", got)
	}
}

func TestWindsurfNote_NamesARewrittenToolInput(t *testing.T) {
	rewrite := Result{Stdout: `{"hookSpecificOutput":{"hookEventName":"PreToolUse","updatedInput":{"command":"rtk git status"}}}`}
	if WindsurfNote("PreToolUse", rewrite) == "" {
		t.Error("updatedInput on PreToolUse must say Devin CLI merges it into the tool's arguments")
	}
	if WindsurfNote("PostToolUse", rewrite) != "" || WindsurfNote("PreToolUse", Result{Exit: 2, Stdout: rewrite.Stdout}) != "" {
		t.Error("updatedInput applies only to a PreToolUse call that runs")
	}
}

func TestWindsurfAddsContext_ReadsAdditionalContextOnItsEvents(t *testing.T) {
	reply := Result{Stdout: `{"hookSpecificOutput":{"hookEventName":"UserPromptSubmit","additionalContext":"Deploys require a ticket."}}`}
	for event, want := range map[string]bool{"UserPromptSubmit": true, "SessionStart": true, "PostToolUse": true, "PreToolUse": false, "Stop": false} {
		if got := AddsContext("windsurf", event, reply); got != want {
			t.Errorf("%s: AddsContext = %t, want %t", event, got, want)
		}
	}
	if AddsContext("windsurf", "UserPromptSubmit", Result{Stdout: "plain text"}) {
		t.Error("plain stdout is not documented as context")
	}
}

func TestBuildWindsurf_WritesTheDocumentedPayloads(t *testing.T) {
	p, err := Build("windsurf", "PreToolUse", "exec", "/project", Input{Bash: "rm -rf /"})
	if err != nil || !p.Fires || p.Trigger != "exec" {
		t.Fatalf("--bash calls exec: %+v %v", p, err)
	}
	var doc struct {
		Event        string         `json:"hook_event_name"`
		SessionID    string         `json:"session_id"`
		PromptID     string         `json:"prompt_id"`
		ToolName     string         `json:"tool_name"`
		ToolInput    map[string]any `json:"tool_input"`
		ToolResponse map[string]any `json:"tool_response"`
		Prompt       string         `json:"prompt"`
	}
	if err := json.Unmarshal(p.Body, &doc); err != nil || doc.Event != "PreToolUse" || doc.SessionID == "" || doc.PromptID == "" ||
		doc.ToolName != "exec" || doc.ToolInput["command"] != "rm -rf /" || doc.ToolInput["shell_id"] != "main" || doc.ToolResponse != nil {
		t.Errorf("payload = %s", p.Body)
	}

	p, err = Build("windsurf", "PostToolUse", "", "/project", Input{Bash: "ls"})
	if err != nil || json.Unmarshal(p.Body, &doc) != nil || doc.ToolResponse["success"] != true || doc.ToolResponse["error"] != nil {
		t.Errorf("PostToolUse adds a successful tool_response with a null error: %s %v", p.Body, err)
	}
	if _, present := doc.ToolResponse["error"]; !present {
		t.Errorf("tool_response lists error: %s", p.Body)
	}

	p, err = Build("windsurf", "UserPromptSubmit", "", "/project", Input{Prompt: "deploy"})
	if err != nil || json.Unmarshal(p.Body, &doc) != nil || doc.Prompt != "deploy" {
		t.Errorf("--prompt builds UserPromptSubmit: %s %v", p.Body, err)
	}
	for _, event := range []string{"Stop", "PostCompaction"} {
		if p, err := Build("windsurf", event, "", "/project", Input{}); err != nil || !p.Fires {
			t.Errorf("%s needs no input: %+v %v", event, p, err)
		}
	}

	var unbuilt Unbuilt
	for name, call := range map[string]func() error{
		"--edit has no documented input": func() error {
			_, err := Build("windsurf", "PreToolUse", "", "/project", Input{Edit: "a.go"})
			return err
		},
		"SessionStart lists no source": func() error { _, err := Build("windsurf", "SessionStart", "", "/project", Input{}); return err },
		"SessionEnd lists no reason":   func() error { _, err := Build("windsurf", "SessionEnd", "", "/project", Input{}); return err },
		"a matcher that does not compile": func() error {
			_, err := Build("windsurf", "PreToolUse", "(", "/project", Input{Bash: "ls"})
			return err
		},
		"a matcher on an event with no tool": func() error { _, err := Build("windsurf", "Stop", "exec", "/project", Input{}); return err },
	} {
		if err := call(); !errors.As(err, &unbuilt) {
			t.Errorf("%s: want Unbuilt, got %v", name, err)
		}
	}
}

func TestBuildWindsurf_MatcherIsAnUnanchoredRegex(t *testing.T) {
	for matcher, fires := range map[string]bool{"": true, "exec": true, "ex": true, "^exec$": true, "^(exec|edit)$": true, "Bash": false, "^edit$": false} {
		p, err := Build("windsurf", "PreToolUse", matcher, "/project", Input{Bash: "ls"})
		if err != nil || p.Fires != fires {
			t.Errorf("matcher %q: fires = %t, want %t (%v)", matcher, p.Fires, fires, err)
		}
	}
}

func TestWindsurfRawPayload_MatchesToolName(t *testing.T) {
	p, err := Build("windsurf", "PreToolUse", "^edit$", "/project", Input{Raw: []byte(`{"tool_name":"edit","tool_input":{}}`)})
	if err != nil || !p.Fires || p.Trigger != "edit" {
		t.Errorf("payload tool call = %+v %v", p, err)
	}
	if p, _ := Build("windsurf", "PreToolUse", "exec", "/project", Input{Raw: []byte(`{"tool_name":"read"}`)}); p.Fires {
		t.Error("exec must not match read")
	}
	if p, err := Build("windsurf", "SessionStart", "", "/project", Input{Raw: []byte(`{"source":"startup"}`)}); err != nil || !p.Fires {
		t.Errorf("--payload builds any event: %+v %v", p, err)
	}
}

func TestWindsurfHooks_ReadsTheTopLevelFile(t *testing.T) {
	body := []byte(`{
		"PreToolUse": [{"matcher": "exec", "hooks": [{"type": "command", "command": ".devin/hooks/g.sh", "timeout": 5}, {"type": "prompt", "prompt": "Is this safe?"}]}],
		"Stop": [{"matcher": "", "hooks": [{"type": "command", "command": ".devin/hooks/s.sh"}]}]
	}`)
	handlers, err := HandlersFromDoc("windsurf", body)
	if err != nil || len(handlers) != 2 {
		t.Fatalf("handlers = %+v %v; want the two command hooks, not the prompt", handlers, err)
	}
	same := func(n, s string) bool { return n == s }
	drift, err := Drift("windsurf", body, "PreToolUse", "exec", "darwin", []Handler{{Command: ".devin/hooks/g.sh", Timeout: 5 * time.Second}}, same)
	if err != nil || len(drift) != 0 {
		t.Errorf("drift = %+v %v", drift, err)
	}
	drift, _ = Drift("windsurf", body, "PreToolUse", "Bash", "darwin", []Handler{{Command: ".devin/hooks/g.sh", Timeout: 5 * time.Second}}, same)
	if len(drift) != 1 {
		t.Errorf("a different matcher must drift: %+v", drift)
	}
	drift, _ = Drift("windsurf", body, "PreToolUse", "exec", "darwin", []Handler{{Command: ".devin/hooks/g.sh"}}, same)
	if len(drift) != 1 {
		t.Errorf("a different timeout must drift: %+v", drift)
	}
}

func TestAssumptions_WindsurfAssumesTheShellCwdAndTimeout(t *testing.T) {
	if _, reason := Assumptions("windsurf", "windows", Handler{Command: ".devin/hooks/a.sh"}); reason != "Devin CLI does not document how it runs a hook command on Windows" {
		t.Errorf("Windows reason = %q", reason)
	}
	if _, reason := Assumptions("windsurf", "linux", Handler{Command: "cat | grep rm"}); reason != "Devin CLI does not document its shell; use a script path" {
		t.Errorf("inline reason = %q", reason)
	}
	got, reason := Assumptions("windsurf", "linux", Handler{Command: ".devin/hooks/a.sh"})
	if reason != "" || len(got) != 3 || got[0].Item != "shell" || got[1].Item != "working directory" || got[2].Item != "timeout" || got[2].Value != "30s" {
		t.Errorf("Assumptions = %+v %q; want the shell, cwd, and 30s timeout", got, reason)
	}
	if got, _ := Assumptions("windsurf", "linux", Handler{Command: ".devin/hooks/a.sh", Timeout: 5 * time.Second}); len(got) != 2 {
		t.Errorf("a spec timeout removes the timeout assumption: %+v", got)
	}
	if argv := Argv("windsurf", "linux", Handler{Command: ".devin/hooks/a.sh"}); len(argv) != 3 || argv[0] != "sh" || argv[1] != "-c" {
		t.Errorf("Argv = %v", argv)
	}
	if ContractDocs("windsurf") != "https://docs.devin.ai/cli/extensibility/hooks" {
		t.Errorf("ContractDocs = %q", ContractDocs("windsurf"))
	}
}
