package hookrun

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestBuild_GeminiEditCallsTheFirstEditToolTheRegexMatches(t *testing.T) {
	root := t.TempDir()
	p, err := Build("gemini", "BeforeTool", "replace", root, Input{Edit: "src/app.go"})
	if err != nil {
		t.Fatal(err)
	}
	doc := decode(t, p.Body)
	input, _ := doc["tool_input"].(map[string]any)
	if !p.Fires || doc["tool_name"] != "replace" {
		t.Errorf("fires=%v tool_name=%v, want replace", p.Fires, doc["tool_name"])
	}
	if want := filepath.Join(root, "src", "app.go"); input["file_path"] != want {
		t.Errorf("file_path = %v, want %q", input["file_path"], want)
	}
	for _, key := range []string{"session_id", "transcript_path", "cwd", "hook_event_name", "timestamp"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("payload misses base field %s: %s", key, p.Body)
		}
	}
	if _, ok := doc["tool_use_id"]; ok {
		t.Errorf("Gemini payload carries tool_use_id: %s", p.Body)
	}
}

func TestBuild_GeminiMatchers(t *testing.T) {
	for _, tc := range []struct {
		name, event, matcher string
		in                   Input
		fires                bool
		trigger              string
	}{
		{"unanchored regex", "BeforeTool", "file", Input{Edit: "a.go"}, true, "write_file"},
		{"anchored miss", "BeforeTool", "^file", Input{Edit: "a.go"}, false, "write_file"},
		{"invalid regex is literal", "BeforeTool", "(", Input{Bash: "ls"}, false, "run_shell_command"},
		{"shell tool", "AfterTool", "run_shell_command", Input{Bash: "ls"}, true, "run_shell_command"},
		{"exact source", "SessionStart", "resume", Input{}, true, "resume"},
		{"no compact source", "SessionStart", "compact", Input{}, false, "startup"},
		{"prompt", "BeforeAgent", "", Input{Prompt: "hi"}, true, "prompt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Build("gemini", tc.event, tc.matcher, t.TempDir(), tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if p.Fires != tc.fires || p.Trigger != tc.trigger {
				t.Errorf("fires=%v trigger=%q, want %v %q", p.Fires, p.Trigger, tc.fires, tc.trigger)
			}
		})
	}
}

func TestBuild_GeminiRejectsClaudeEventNames(t *testing.T) {
	if _, err := Build("gemini", "PreToolUse", "", t.TempDir(), Input{Bash: "ls"}); err == nil {
		t.Error("Gemini built a PreToolUse payload")
	}
}

func TestDecide_GeminiReadsTheFirstNonEmptyStream(t *testing.T) {
	for _, tc := range []struct {
		name, event string
		r           Result
		want        Decision
	}{
		{"exit 2 with a reason blocks", "BeforeTool", Result{Exit: 2, Stderr: "protected"}, Block},
		{"exit 3 blocks too", "BeforeTool", Result{Exit: 3, Stderr: "no"}, Block},
		{"exit 2 with no output decides nothing", "BeforeTool", Result{Exit: 2}, Error},
		{"exit 1 warns", "AfterTool", Result{Exit: 1, Stderr: "lint"}, Error},
		{"deny reply", "BeforeTool", Result{Stdout: `{"decision":"deny","reason":"no"}`}, Block},
		{"block alias", "BeforeAgent", Result{Stdout: `{"decision":"block"}`}, Block},
		{"stop reply", "AfterTool", Result{Stdout: `{"continue":false}`}, Block},
		{"session start is advisory", "SessionStart", Result{Exit: 2, Stderr: "no"}, Error},
		{"session start deny ignored", "SessionStart", Result{Stdout: `{"decision":"deny"}`}, Allow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Decide("gemini", tc.event, tc.r); got != tc.want {
				t.Errorf("Decide = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestArgv_GeminiRunsBashOrPowerShell(t *testing.T) {
	h := Handler{Command: "echo hi"}
	if got, want := Argv("gemini", "linux", h), []string{"bash", "-c", "echo hi"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Argv = %q, want %q", got, want)
	}
	want := []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "echo hi; if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }"}
	if got := Argv("gemini", "windows", h); !reflect.DeepEqual(got, want) {
		t.Errorf("Argv = %q, want %q", got, want)
	}
}

func TestExpandCommand_GeminiReplacesProjectVariablesWithTheQuotedRoot(t *testing.T) {
	got := ExpandCommand("gemini", "linux", `sh $GEMINI_PROJECT_DIR/x.sh $CLAUDE_PROJECT_DIR ${GEMINI_CWD}`, "/a b")
	if want := `sh '/a b'/x.sh '/a b' ${GEMINI_CWD}`; got != want {
		t.Errorf("ExpandCommand = %q, want %q", got, want)
	}
	if got := ExpandCommand("gemini", "windows", `& $GEMINI_PROJECT_DIR\x.ps1`, `C:\it's`); got != `& 'C:\it''s'\x.ps1` {
		t.Errorf("PowerShell ExpandCommand = %q", got)
	}
	if got := ExpandCommand("claude", "linux", "$CLAUDE_PROJECT_DIR/x", "/r"); got != "$CLAUDE_PROJECT_DIR/x" {
		t.Errorf("Claude command changed: %q", got)
	}
}
