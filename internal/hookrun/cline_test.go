package hookrun

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestClineControl_ReadsStdoutAsClineDoes(t *testing.T) {
	for name, tc := range map[string]struct {
		stdout  string
		want    any
		invalid bool
	}{
		"empty stdout is no control":       {"  \n", nil, false},
		"the whole stdout is JSON":         {"\n{\"cancel\":true}\n", map[string]any{"cancel": true}, false},
		"the last HOOK_CONTROL line wins":  {"HOOK_CONTROL\t{\"cancel\":false}\nlog\n  HOOK_CONTROL\t{\"cancel\":true}  \nmore", map[string]any{"cancel": true}, false},
		"text without a control line":      {"checked\n", nil, true},
		"two objects are not one JSON":     {`{"a":1}{"b":2}`, nil, true},
		"a control line that is not JSON":  {"HOOK_CONTROL\tnope", nil, true},
		"a bare value is JSON too":         {"true", true, false},
		"a tab-only control line is plain": {"HOOK_CONTROL\t\n", nil, true},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := clineControl(tc.stdout)
			if (err != nil) != tc.invalid {
				t.Fatalf("err = %v, want invalid %t", err, tc.invalid)
			}
			if !tc.invalid && !jsonEqual(got, tc.want) {
				t.Errorf("control = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func TestReadCline_BlocksOnlyOnCancel(t *testing.T) {
	ignored := `Cline ignores the exit code; print {"cancel": true} to block`
	for name, tc := range map[string]struct {
		event string
		r     Result
		want  Decision
		note  string
	}{
		"empty stdout allows":                    {"PreToolUse", Result{}, Allow, ""},
		"cancel true blocks":                     {"PreToolUse", Result{Stdout: `{"cancel":true}`}, Block, "cancel: Cline skips the tool call and stops the run"},
		"cancel true blocks whatever the exit":   {"PreToolUse", Result{Exit: 1, Stdout: `{"cancel":true}`}, Block, ""},
		"cancel on PostToolUse stops the run":    {"PostToolUse", Result{Stdout: `{"cancel":true}`}, Block, "cancel: the tool already ran; Cline stops the run"},
		"cancel false allows":                    {"PreToolUse", Result{Stdout: `{"cancel":false}`}, Allow, ""},
		"a string cancel allows":                 {"PreToolUse", Result{Stdout: `{"cancel":"true"}`}, Allow, ""},
		"an array allows":                        {"PreToolUse", Result{Stdout: `[true]`}, Allow, ""},
		"exit 2 without cancel allows":           {"PreToolUse", Result{Exit: 2}, Allow, ignored},
		"invalid JSON is an error Cline skips":   {"PreToolUse", Result{Stdout: "Blocked"}, Error, "stdout is not JSON: Cline logs it and goes on as if the hook allowed"},
		"invalid JSON with exit 2":               {"PreToolUse", Result{Exit: 2, Stdout: "Blocked"}, Error, ignored},
		"a timeout Cline skips":                  {"PostToolUse", Result{TimedOut: true}, Timeout, "Cline logs a hook that times out and goes on as if it allowed"},
		"a hook that did not start":              {"PreToolUse", Result{StartErr: errors.New("no bash")}, Error, "Cline logs a hook that does not start and goes on as if it allowed"},
		"a detached hook cannot block":           {"UserPromptSubmit", Result{Exit: 2, Stdout: `{"cancel":true}`}, Allow, ""},
		"a detached hook can still time out":     {"TaskStart", Result{TimedOut: true}, Timeout, ""},
		"a detached hook that did not start":     {"TaskComplete", Result{StartErr: errors.New("no bash")}, Error, ""},
		"the last control line decides":          {"PreToolUse", Result{Stdout: "HOOK_CONTROL\t{\"cancel\":true}\nHOOK_CONTROL\t{}"}, Allow, ""},
		"plain text before a control line is ok": {"PreToolUse", Result{Stdout: "checking\nHOOK_CONTROL\t{\"cancel\":true}"}, Block, ""},
	} {
		t.Run(name, func(t *testing.T) {
			got := readCline(tc.event, tc.r)
			if got.decision != tc.want {
				t.Errorf("decision = %s, want %s", got.decision, tc.want)
			}
			if tc.note != "" && !slices.Contains(got.notes, tc.note) {
				t.Errorf("notes = %q, want %q", got.notes, tc.note)
			}
			if DecideHandler("cline", tc.event, Handler{}, tc.r) != tc.want {
				t.Error("DecideHandler disagrees with readCline")
			}
		})
	}
	if notes := ClineNotes("PreToolUse", Result{Exit: 1, Stdout: `{"cancel":true}`}); slices.Contains(notes, ignored) {
		t.Errorf("a cancel needs no exit code hint: %q", notes)
	}
}

func TestClineAddsContext_FromAReplyThatDoesNotCancel(t *testing.T) {
	for stdout, want := range map[string]bool{
		`{"context":"use make"}`:                      true,
		`{"contextModification":"use make"}`:          true,
		`{"errorMessage":"use make"}`:                 true,
		`{"context":"","errorMessage":"use make"}`:    false,
		`{"context":"  "}`:                            false,
		`{"cancel":true,"context":"no"}`:              false,
		"HOOK_CONTROL\t{\"context\":\"from a line\"}": true,
	} {
		if got := AddsContext("cline", "PreToolUse", Result{Stdout: stdout}); got != want {
			t.Errorf("%s: AddsContext = %t, want %t", stdout, got, want)
		}
	}
	if AddsContext("cline", "UserPromptSubmit", Result{Stdout: `{"context":"x"}`}) {
		t.Error("a detached hook's stdout never reaches Cline")
	}
}

type clinePayload struct {
	HookName       string `json:"hookName"`
	ClineVersion   string `json:"clineVersion"`
	Timestamp      string `json:"timestamp"`
	TaskID         string `json:"taskId"`
	SessionContext struct {
		RootSessionID string `json:"rootSessionId"`
	} `json:"sessionContext"`
	WorkspaceRoots []string `json:"workspaceRoots"`
	AgentID        string   `json:"agent_id"`
	ParentAgentID  *string  `json:"parent_agent_id"`
	Iteration      int      `json:"iteration"`
	ToolCall       *struct {
		ID    string         `json:"id"`
		Name  string         `json:"name"`
		Input map[string]any `json:"input"`
	} `json:"tool_call"`
	ToolResult *struct {
		Name   string         `json:"name"`
		Input  map[string]any `json:"input"`
		Output *string        `json:"output"`
	} `json:"tool_result"`
	PreToolUse *struct {
		ToolName   string            `json:"toolName"`
		Parameters map[string]string `json:"parameters"`
	} `json:"preToolUse"`
	PostToolUse *struct {
		ToolName   string            `json:"toolName"`
		Parameters map[string]string `json:"parameters"`
		Success    bool              `json:"success"`
	} `json:"postToolUse"`
	UserPromptSubmit *struct {
		Prompt string `json:"prompt"`
	} `json:"userPromptSubmit"`
}

func decodeCline(t *testing.T, p Payload) clinePayload {
	t.Helper()
	var doc clinePayload
	if err := json.Unmarshal(p.Body, &doc); err != nil {
		t.Fatalf("payload %s: %v", p.Body, err)
	}
	return doc
}

func TestBuildCline_WritesTheSDKPayloads(t *testing.T) {
	p, err := Build("cline", "PreToolUse", "Bash", "/project", Input{Bash: "a && b"})
	if err != nil || !p.Fires || p.Trigger != "run_commands" {
		t.Fatalf("--bash calls run_commands, whatever the matcher: %+v %v", p, err)
	}
	if !strings.Contains(string(p.Body), `"a && b"`) || strings.HasSuffix(string(p.Body), "\n") {
		t.Errorf("the payload is written as JSON.stringify writes it: %s", p.Body)
	}
	doc := decodeCline(t, p)
	if doc.HookName != "tool_call" || doc.TaskID != SessionID || doc.SessionContext.RootSessionID != SessionID ||
		len(doc.WorkspaceRoots) != 1 || doc.WorkspaceRoots[0] != "/project" || doc.AgentID == "" || doc.ParentAgentID != nil || doc.Iteration != 1 {
		t.Errorf("base fields = %+v", doc)
	}
	if _, err := time.Parse(time.RFC3339, doc.Timestamp); err != nil {
		t.Errorf("timestamp %q: %v", doc.Timestamp, err)
	}
	if doc.ToolCall == nil || doc.ToolCall.Name != "run_commands" || !jsonEqual(doc.ToolCall.Input, map[string]any{"commands": []string{"a && b"}}) {
		t.Errorf("tool_call = %+v", doc.ToolCall)
	}
	if doc.PreToolUse == nil || doc.PreToolUse.ToolName != "run_commands" || doc.PreToolUse.Parameters["commands"] != `["a && b"]` {
		t.Errorf("preToolUse = %+v", doc.PreToolUse)
	}

	p, err = Build("cline", "PostToolUse", "", "/project", Input{Edit: "a.go"})
	if err != nil || p.Trigger != "editor" {
		t.Fatalf("--edit calls editor: %+v %v", p, err)
	}
	doc = decodeCline(t, p)
	path := filepath.Join("/project", "a.go")
	if doc.HookName != "tool_result" || doc.ToolResult == nil || doc.ToolResult.Name != "editor" || doc.ToolResult.Input["path"] != path ||
		doc.ToolResult.Output == nil || doc.PostToolUse == nil || doc.PostToolUse.Parameters["path"] != path || !doc.PostToolUse.Success || doc.ToolCall != nil {
		t.Errorf("tool_result payload = %s", p.Body)
	}

	p, err = Build("cline", "UserPromptSubmit", "", "/project", Input{Prompt: "hi"})
	if doc := decodeCline(t, p); err != nil || doc.HookName != "prompt_submit" || doc.UserPromptSubmit == nil || doc.UserPromptSubmit.Prompt != "hi" {
		t.Errorf("--prompt builds prompt_submit: %s %v", p.Body, err)
	}

	var unbuilt Unbuilt
	if _, err := Build("cline", "PreCompact", "", "/project", Input{Raw: []byte(`{}`)}); !errors.As(err, &unbuilt) || unbuilt.Reason != "Cline has no PreCompact hook event" {
		t.Errorf("PreCompact is never run: %v", err)
	}
	if _, err := Build("cline", "TaskStart", "", "/project", Input{}); err == nil {
		t.Error("TaskStart needs --payload")
	}
	p, err = Build("cline", "PreToolUse", "Bash", "/project", Input{Raw: []byte(`{"tool_call":{"name":"read_files"}}`)})
	if err != nil || !p.Fires || p.Trigger != "read_files" {
		t.Errorf("--payload fires on any tool and names it: %+v %v", p, err)
	}
}

func TestClineRunsTheScriptWithBash(t *testing.T) {
	h := Handler{Command: "/project/.clinerules/hooks/PreToolUse", Script: "set -e\necho hi\n"}
	if got := Argv("cline", "darwin", h); !slices.Equal(got, []string{"bash", "-c", h.Script, h.Command}) {
		t.Errorf("argv = %q", got)
	}
	if got := ExpandCommand("cline", "darwin", ".clinerules/hooks/PreToolUse", "/project"); got != filepath.Join("/project", ".clinerules/hooks/PreToolUse") {
		t.Errorf("Cline starts the script by absolute path, got %q", got)
	}
	if DefaultTimeout("cline", "PreToolUse") != 120*time.Second {
		t.Error("Cline waits 120 s on a tool hook")
	}
	if assumed, reason := Assumptions("cline", "darwin", h); reason != "" || len(assumed) != 1 || assumed[0].Item != "working directory" {
		t.Errorf("assumptions = %+v %q", assumed, reason)
	}
	if _, reason := Assumptions("cline", "windows", h); reason != "Cline runs hooks with bash; hook run does not assume which bash Windows resolves" {
		t.Errorf("windows reason = %q", reason)
	}
	if !FireAndForget("cline", "UserPromptSubmit") || !FireAndForget("cline", "TaskStart") || FireAndForget("cline", "PostToolUse") {
		t.Error("Cline waits only on PreToolUse and PostToolUse")
	}
	if ContractDocs("cline") != clineSource {
		t.Error("the assumption cites the pinned source")
	}
}

func TestClineRunNotes_NameWhatClineDoesNotRead(t *testing.T) {
	notes := ClineRunNotes("PreToolUse", "Bash", 5*time.Second, "editor")
	for _, want := range []string{
		`Cline has no matcher: sync drops "Bash", and the script runs on every tool call`,
		"Cline has no per-hook timeout: sync drops the spec's 5s, and hook run uses Cline's 120s",
		"Cline calls apply_patch instead of editor for a model whose id holds gpt or codex",
	} {
		if !slices.Contains(notes, want) {
			t.Errorf("notes %q miss %q", notes, want)
		}
	}
	if notes := ClineRunNotes("PreToolUse", "", 0, "run_commands"); len(notes) != 0 {
		t.Errorf("a plain spec needs no note: %q", notes)
	}
	if notes := ClineRunNotes("UserPromptSubmit", "", 0, "prompt"); len(notes) != 1 || !strings.Contains(notes[0], "orchestrated sessions") {
		t.Errorf("a prompt hook may not run on the CLI: %q", notes)
	}
}

func TestClineDrift_FindsTheSpecInTheSharedScript(t *testing.T) {
	h := Handler{Command: ".clinerules/hooks/PreToolUse", Script: "set -e\nexport AGNOSTIC_AI_TARGET=cline\n\n./guard.sh\n"}
	shared := []byte("# Generated by agnostic-ai.\nset -e\nexport AGNOSTIC_AI_TARGET=cline\n\n./other.sh\n\n./guard.sh\n")
	if drift, err := Drift("cline", shared, "PreToolUse", "", "darwin", []Handler{h}, nil); err != nil || len(drift) != 0 {
		t.Errorf("a script that joins two specs runs this one: %+v %v", drift, err)
	}
	stale := []byte("# Generated by agnostic-ai.\nset -e\nexport AGNOSTIC_AI_TARGET=cline\n\n./old.sh\n")
	drift, _ := Drift("cline", stale, "PreToolUse", "", "darwin", []Handler{h}, nil)
	if len(drift) != 1 || drift[0].Reason != "does not run this spec's PreToolUse commands" {
		t.Errorf("drift = %+v", drift)
	}
}

// A filtered spec runs only where the script feeds every command the
// payload; a script that reads it inside one command's filter drifts.
func TestClineDrift_NeedsThePayloadFedToEveryCommand(t *testing.T) {
	prologue := "set -e\nexport AGNOSTIC_AI_TARGET=cline\n"
	block := func(lines ...string) string { return "\nset +e\n(\nset -e\n" + strings.Join(lines, "\n") + "\n)\n" }
	editFilter := `case $aai_in in *'"toolName":"editor"'*) ;; *) exit 0 ;; esac`
	shellFilter := `case $aai_in in *'"toolName":"run_commands"'*) ;; *) exit 0 ;; esac`
	read, feed := "aai_in=$(cat)\n", "exec <<<\"$aai_in\"\n"
	guard := Handler{Command: ".clinerules/hooks/PreToolUse", Script: prologue + read + feed + block(shellFilter, "./guard.sh")}
	native := Handler{Command: ".clinerules/hooks/PreToolUse", Script: prologue + block("./audit.sh")}

	fed := []byte(prologue + read + feed + block(editFilter, "./edit.sh") + feed + block("./audit.sh") + feed + block(shellFilter, "./guard.sh"))
	if drift, err := Drift("cline", fed, "PreToolUse", "", "darwin", []Handler{guard, native}, nil); err != nil || len(drift) != 0 {
		t.Errorf("a script that feeds each command runs both specs: %+v %v", drift, err)
	}
	for name, body := range map[string]string{
		"read inside each filter": prologue + block("aai_in=$(cat)", editFilter, feed+"./edit.sh") + block("aai_in=$(cat)", shellFilter, feed+"./guard.sh"),
		"a command left unfed":    prologue + read + feed + block(editFilter, "./edit.sh") + block(shellFilter, "./guard.sh"),
	} {
		if drift, _ := Drift("cline", []byte(body), "PreToolUse", "", "darwin", []Handler{guard}, nil); len(drift) != 1 {
			t.Errorf("%s: drift = %+v, want the guard named", name, drift)
		}
	}
}

func TestClineSharedScript_NamesTheSiblings(t *testing.T) {
	if ClineSharedScript(nil) != "" {
		t.Error("a spec alone in its script counts")
	}
	if got := ClineSharedScript([]string{"a", "b"}); got != "Cline runs this hook in one script with a, b, which can change its result" {
		t.Errorf("reason = %q", got)
	}
}
