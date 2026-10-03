package hookrun

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestDecideCopilot_FollowsTheDocumentedRules(t *testing.T) {
	for name, tc := range map[string]struct {
		event string
		r     Result
		want  Decision
	}{
		"deny blocks":                             {"preToolUse", Result{Stdout: `{"permissionDecision":"deny","permissionDecisionReason":"no"}`}, Block},
		"PascalCase deny blocks":                  {"PreToolUse", Result{Stdout: `{"permissionDecision":"deny"}`}, Block},
		"ask blocks":                              {"preToolUse", Result{Stdout: `{"permissionDecision":"ask"}`}, Block},
		"allow allows":                            {"preToolUse", Result{Stdout: `{"permissionDecision":"allow"}`}, Allow},
		"empty output allows":                     {"preToolUse", Result{}, Allow},
		"invalid JSON is no output":               {"preToolUse", Result{Stdout: "ok"}, Allow},
		"two objects are no output":               {"preToolUse", Result{Stdout: "{\"permissionDecision\":\"deny\"}\n{\"permissionDecision\":\"deny\"}"}, Allow},
		"progress lines are stripped":             {"preToolUse", Result{Stdout: "{\"type\":\"progress\",\"message\":\"x\"}\n{\"permissionDecision\":\"deny\"}"}, Block},
		"a multi-line reply parses":               {"preToolUse", Result{Stdout: "{\n  \"permissionDecision\": \"deny\"\n}"}, Block},
		"exit 2 denies over allow":                {"preToolUse", Result{Exit: 2, Stdout: `{"permissionDecision":"allow"}`}, Block},
		"another exit fails closed":               {"preToolUse", Result{Exit: 1}, Block},
		"a crash fails closed":                    {"preToolUse", Result{StartErr: errors.New("no such file")}, Block},
		"a timeout fails open":                    {"preToolUse", Result{TimedOut: true}, Timeout},
		"permissionRequest exit 2 denies":         {"permissionRequest", Result{Exit: 2}, Block},
		"permissionRequest behavior deny blocks":  {"PermissionRequest", Result{Stdout: `{"behavior":"deny"}`}, Block},
		"permissionRequest other exit fails open": {"permissionRequest", Result{Exit: 1}, Error},
		"agentStop block forces a turn":           {"Stop", Result{Stdout: `{"decision":"block","reason":"go on"}`}, Block},
		"subagentStop block forces a turn":        {"subagentStop", Result{Stdout: `{"decision":"block"}`}, Block},
		"exit 2 elsewhere is a warning":           {"sessionStart", Result{Exit: 2}, Error},
		"postToolUse cannot block":                {"postToolUse", Result{Stdout: `{"permissionDecision":"deny"}`}, Allow},
		"postToolUseFailure exit 2 is context":    {"postToolUseFailure", Result{Exit: 2, Stdout: "retry"}, Allow},
	} {
		t.Run(name, func(t *testing.T) {
			if got := decideCopilot(tc.event, tc.r); got != tc.want {
				t.Errorf("decideCopilot = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestBuildCopilot_WritesEachPayloadFormat(t *testing.T) {
	p, err := buildCopilot("preToolUse", "bash|edit", "/project", Input{Bash: "rm -rf /"})
	if err != nil || !p.Fires || p.Trigger != "bash" {
		t.Fatalf("camelCase preToolUse = %+v, %v", p, err)
	}
	var camel map[string]any
	if err := json.Unmarshal(p.Body, &camel); err != nil || camel["toolName"] != "bash" || camel["toolArgs"] != `{"command":"rm -rf /"}` || camel["cwd"] != "/project" || camel["sessionId"] != SessionID {
		t.Errorf("camelCase payload = %s", p.Body)
	}

	p, err = buildCopilot("PreToolUse", "Bash", "/project", Input{Bash: "ls"})
	if err != nil || !p.Fires || p.Trigger != "Bash" {
		t.Fatalf("PascalCase PreToolUse = %+v, %v", p, err)
	}
	var pascal map[string]any
	if err := json.Unmarshal(p.Body, &pascal); err != nil || pascal["tool_name"] != "Bash" || pascal["hook_event_name"] != "PreToolUse" || pascal["session_id"] != SessionID {
		t.Errorf("PascalCase payload = %s", p.Body)
	}
	if input, _ := pascal["tool_input"].(map[string]any); input["command"] != "ls" {
		t.Errorf("tool_input = %v", pascal["tool_input"])
	}

	for _, tc := range []struct {
		event, matcher string
		fires          bool
	}{
		{"preToolUse", "Bash", false},
		{"preToolUse", "ba", false},
		{"preToolUse", "", true},
		{"PreToolUse", "bash", true},
		{"PreToolUse", "Edit|Write", false},
		{"PreToolUse", "**", true},
		{"PreToolUse", "B.*", true},
		{"PreToolUse", "b.*", false},
	} {
		if p, _ := buildCopilot(tc.event, tc.matcher, "/project", Input{Bash: "ls"}); p.Fires != tc.fires {
			t.Errorf("%s matcher %q fires = %t, want %t", tc.event, tc.matcher, p.Fires, tc.fires)
		}
	}
}

func TestBuildCopilot_RefusesWhatIsUndocumented(t *testing.T) {
	for name, tc := range map[string]struct {
		event string
		in    Input
	}{
		"edit tool input":       {"preToolUse", Input{Edit: "a.go"}},
		"PostToolUse tool name": {"PostToolUse", Input{Bash: "ls"}},
		"an unbuilt event":      {"agentStop", Input{}},
	} {
		var unbuilt Unbuilt
		if _, err := buildCopilot(tc.event, "", "/project", tc.in); !errors.As(err, &unbuilt) {
			t.Errorf("%s: err = %v, want Unbuilt so other targets still run", name, err)
		}
	}
}

func TestBuild_MatchesARawCopilotPayloadAsCopilotDoes(t *testing.T) {
	for name, tc := range map[string]struct {
		event, matcher, body string
		want                 bool
	}{
		"camelCase tool, anchored":  {"preToolUse", "ba", `{"toolName":"bash"}`, false},
		"camelCase tool, same":      {"postToolUse", "bash|edit", `{"toolName":"edit"}`, true},
		"PascalCase runtime name":   {"PreToolUse", "create", `{"tool_name":"Write"}`, true},
		"notification type":         {"notification", "agent_.*", `{"notification_type":"shell_completed"}`, false},
		"subagent name":             {"subagentStart", "explore", `{"agentName":"explore"}`, true},
		"an event with no matcher":  {"agentStop", "anything", `{}`, true},
		"Task matches Agent's tool": {"PreToolUse", "Task", `{"tool_name":"Agent"}`, true},
	} {
		t.Run(name, func(t *testing.T) {
			p, err := Build("copilot", tc.event, tc.matcher, "/project", Input{Raw: []byte(tc.body)})
			if err != nil || p.Fires != tc.want {
				t.Errorf("fires = %t, %v; want %t", p.Fires, err, tc.want)
			}
		})
	}
}

func TestCopilotAssumptions_NameEachUndocumentedItem(t *testing.T) {
	assumed, reason := Assumptions("copilot", "darwin", Handler{Command: ".github/hooks/scripts/guard.sh"})
	if reason != "" || len(assumed) != 2 || assumed[0].Item != "shell" || assumed[1].Item != "working directory" {
		t.Errorf("command form = %+v, %q", assumed, reason)
	}
	assumed, reason = Assumptions("copilot", "windows", Handler{Command: "guard", Exec: true, Cwd: "scripts"})
	if reason != "" || len(assumed) != 0 {
		t.Errorf("exec form with cwd needs no shell and no root: %+v, %q", assumed, reason)
	}
	assumed, _ = Assumptions("copilot", "linux", Handler{Command: ".github/hooks/scripts/guard.sh", Exec: true, Cwd: "scripts"})
	if len(assumed) != 1 || assumed[0].Item != "exec path" {
		t.Errorf("a relative exec path is assumed to resolve from cwd: %+v", assumed)
	}
	if h := CopilotExec("/project", Handler{Command: "../bin/guard", Exec: true, Cwd: "scripts"}); h.Command != filepath.Join("/project", "bin/guard") {
		t.Errorf("exec path = %s", h.Command)
	}
	for name, h := range map[string]Handler{
		"shell syntax":  {Command: "cat | grep rm"},
		"env expansion": {Command: "guard.sh", Env: map[string]string{"P": "$HOME/x"}},
	} {
		if _, reason := Assumptions("copilot", "linux", h); reason == "" {
			t.Errorf("%s must not run", name)
		}
	}
	if _, reason := Assumptions("copilot", "windows", Handler{Command: "guard.ps1"}); reason == "" {
		t.Error("the powershell interpreter is undocumented")
	}
}

func TestCopilotHandlers_ReadEachForm(t *testing.T) {
	body := []byte(`{"version":1,"hooks":{"preToolUse":[
		{"type":"command","command":"a.sh","timeoutSec":5},
		{"type":"command","bash":"b.sh","command":"ignored","timeout":7},
		{"exec":"c","args":["--x"],"cwd":"scripts"},
		{"type":"http","url":"https://example.com"}]}}`)
	handlers, err := CopilotHandlers(body, "preToolUse")
	if err != nil || len(handlers) != 3 {
		t.Fatalf("handlers = %+v, %v", handlers, err)
	}
	if handlers[0].Command != "a.sh" || handlers[0].Timeout != 5*time.Second {
		t.Errorf("command form = %+v", handlers[0])
	}
	if handlers[1].Command != "b.sh" || handlers[1].Timeout != 7*time.Second {
		t.Errorf("bash wins over command, timeout is an alias: %+v", handlers[1])
	}
	if h := handlers[2]; !h.Exec || h.Command != "c" || h.Cwd != "scripts" || len(h.Args) != 1 {
		t.Errorf("exec form = %+v", h)
	}
	if argv := Argv("copilot", "linux", handlers[2]); len(argv) != 2 || argv[0] != "c" {
		t.Errorf("exec form runs with no shell: %v", argv)
	}
	if DefaultTimeout("copilot", "preToolUse") != 30*time.Second {
		t.Error("Copilot documents a 30 second default")
	}
}

func TestCopilotDrift_ComparesMatcherTimeoutAndCwd(t *testing.T) {
	body := []byte(`{"hooks":{"preToolUse":[{"type":"command","matcher":"bash","command":"a.sh","cwd":"scripts"}]}}`)
	h := Handler{Command: "a.sh", Cwd: "scripts"}
	if drift, err := copilotDrift(body, "preToolUse", "bash", []Handler{h}); err != nil || len(drift) != 0 {
		t.Errorf("in sync: %+v, %v", drift, err)
	}
	h.Cwd = ""
	if drift, _ := copilotDrift(body, "preToolUse", "bash", []Handler{h}); len(drift) != 1 {
		t.Error("a changed cwd must warn")
	}
	if drift, _ := copilotDrift(body, "preToolUse", "edit", []Handler{{Command: "a.sh", Cwd: "scripts"}}); len(drift) != 1 {
		t.Error("a changed matcher must warn")
	}
}

func TestCopilotMatcher_RefusesARegexJavaScriptReadsDifferently(t *testing.T) {
	for _, matcher := range []string{"(?i)bash", "(?!edit).*", "(?<=b)ash", `(b)\1`, "(?P<t>bash)", `\Abash`, "[[:alpha:]]+"} {
		var unbuilt Unbuilt
		if _, err := buildCopilot("preToolUse", matcher, "/project", Input{Bash: "ls"}); !errors.As(err, &unbuilt) {
			t.Errorf("matcher %q: err = %v, want Unbuilt", matcher, err)
		}
	}
	for _, matcher := range []string{"ba(?:sh)", `bash\.exe|bash`, "(?<t>bash)", "[a-z]+"} {
		if p, err := buildCopilot("preToolUse", matcher, "/project", Input{Bash: "ls"}); err != nil || !p.Fires {
			t.Errorf("matcher %q reads alike in both dialects: %+v, %v", matcher, p, err)
		}
	}
}

func TestCopilotMergedBlocks_LaterOutputOverrides(t *testing.T) {
	deny, allow := Result{Stdout: `{"behavior":"deny"}`}, Result{Stdout: `{"behavior":"allow"}`}
	for name, tc := range map[string]struct {
		results []Result
		want    bool
	}{
		"deny then allow allows":        {[]Result{deny, allow}, false},
		"allow then deny denies":        {[]Result{allow, deny}, true},
		"an absent behavior keeps deny": {[]Result{deny, {Stdout: `{"message":"x"}`}}, true},
		"exit 2 denies over its allow":  {[]Result{{Exit: 2, Stdout: `{"behavior":"allow"}`}}, true},
		"a failed hook is skipped":      {[]Result{deny, {Exit: 1, Stdout: `{"behavior":"allow"}`}}, true},
	} {
		if got, ok := CopilotMergedBlocks("PermissionRequest", tc.results); !ok || got != tc.want {
			t.Errorf("%s: blocks = %t, %t", name, got, ok)
		}
	}
	if _, ok := CopilotMergedBlocks("preToolUse", []Result{deny}); ok {
		t.Error("preToolUse blocks on any deny and does not merge")
	}
}

func TestCopilotErrored_RecordsAFailureThatDenies(t *testing.T) {
	for name, tc := range map[string]struct {
		r    Result
		want bool
	}{
		"did not start":      {Result{StartErr: errors.New("no such file")}, true},
		"exit 1":             {Result{Exit: 1}, true},
		"exit 2 is a denial": {Result{Exit: 2}, false},
		"a timeout":          {Result{TimedOut: true}, false},
	} {
		if got := CopilotErrored("preToolUse", tc.r); got != tc.want {
			t.Errorf("%s = %t", name, got)
		}
	}
}
