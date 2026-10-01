package hookrun

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestDrift_NamesDifferingEnvKeysWithoutValues(t *testing.T) {
	native := []byte(`{"hooks":{"BeforeTool":[{"hooks":[{"type":"command","command":"a.sh","env":{"TOKEN":"old-secret","KEEP":"same-secret"}}]}]}}`)
	h := Handler{Command: "a.sh", Env: map[string]string{"TOKEN": "new-secret", "KEEP": "same-secret", "ADDED": "added-secret"}}

	drift, err := Drift("gemini", native, "BeforeTool", "", "linux", []Handler{h}, anyCover)
	if err != nil || len(drift) != 1 {
		t.Fatalf("Drift = %+v, %v", drift, err)
	}
	reason := drift[0].Reason
	if !strings.Contains(reason, "ADDED, TOKEN") {
		t.Errorf("reason = %q, want the differing keys named", reason)
	}
	if strings.Contains(reason, "secret") {
		t.Errorf("reason = %q leaks an env value", reason)
	}
}

// docs.augmentcode.com/cli/hooks: Stop and PostToolUse block with
// hookSpecificOutput.decision "block".
func TestDecide_AugmentReadsTheNestedDecision(t *testing.T) {
	for _, event := range []string{"Stop", "PostToolUse"} {
		r := Result{Stdout: `{"hookSpecificOutput":{"hookEventName":"` + event + `","decision":"block","reason":"run tests"}}`}
		if got := DecideHandler("augment", event, Handler{}, r); got != Block {
			t.Errorf("%s = %s, want block", event, got)
		}
	}
}

// code.claude.com/docs/en/hooks: "a single-segment directory pattern like
// "Edit(src/**)" matches only the src directory in the working directory
// ... Before v2.1.214, "Edit(src/**)" matched a directory named src at
// any depth".
func TestClaudeIfRuns_SingleDirectoryIsAnchoredAtTheWorkingDirectory(t *testing.T) {
	for _, tc := range []struct {
		file string
		want bool
	}{
		{"/p/src/x/a.ts", true},
		{"/p/vendor/pkg/src/lib.js", false},
	} {
		got, err := ClaudeIfRuns("Edit(src/**)", "PreToolUse", toolBody(t, "Edit", map[string]any{"file_path": filepath.FromSlash(tc.file)}), filepath.FromSlash("/p"))
		if err != nil || got != tc.want {
			t.Errorf("Edit(src/**) on %s = %v, %v, want %v", tc.file, got, err, tc.want)
		}
	}
}

func TestBuild_RawToolPayloadUsesTheTargetMatcher(t *testing.T) {
	for _, tc := range []struct {
		target, event, tool, matcher string
		fires                        bool
	}{
		{"claude", "PreToolUse", "Read", "Bash", false},
		{"claude", "PostToolUseFailure", "Read", "Read, Write", true},
		{"codex", "PreToolUse", "apply_patch", "Edit", true},
		{"codex", "PreToolUse", "apply_patch", "MultiEdit", false},
		{"gemini", "BeforeTool", "read_file", "write", false},
		{"gemini", "AfterTool", "read_file", "read", true},
		{"trae", "PreToolUse", "RunCommand", "Edit", false},
		{"trae", "PostToolUse", "RunCommand", "Run", true},
		{"openhands", "PreToolUse", "file_editor", "file", false},
		{"openhands", "PostToolUse", "file_editor", "/file.*/", true},
		{"augment", "PreToolUse", "web-fetch", "launch-process", false},
		{"augment", "PostToolUse", "web-fetch", "web", true},
	} {
		raw := toolBody(t, tc.tool, map[string]any{})
		p, err := Build(tc.target, tc.event, tc.matcher, t.TempDir(), Input{Raw: raw})
		if err != nil {
			t.Errorf("%s %s: %v", tc.target, tc.event, err)
			continue
		}
		if p.Fires != tc.fires || p.Trigger != tc.tool {
			t.Errorf("%s %s matcher %q: fires=%v trigger=%q, want %v %q", tc.target, tc.event, tc.matcher, p.Fires, p.Trigger, tc.fires, tc.tool)
		}
		if string(p.Body) != string(raw) {
			t.Errorf("%s changed the supplied payload: %s", tc.target, p.Body)
		}
	}
}

func TestBuild_RawGoosePayloadMatchesTheContext(t *testing.T) {
	for _, tc := range []struct {
		event, tool, context, matcher string
		fires                         bool
	}{
		{"PreToolUse", "developer__shell", "shell", "^shell$", true},
		{"PreToolUse", "shell", "developer__shell", "^shell$", false},
		{"PostToolUse", "developer__shell", "shell", "^shell$", true},
		{"PostToolUse", "shell", "developer__shell", "^shell$", false},
		{"BeforeShellExecution", "shell", "git push", "^git push$", true},
		{"BeforeShellExecution", "shell", "git status", "^git push$", false},
		{"AfterShellExecution", "shell", "git push", "^git push$", true},
		{"AfterShellExecution", "shell", "git status", "^git push$", false},
		{"BeforeReadFile", "read", "/project/src/a.go", "^/project/src/", true},
		{"BeforeReadFile", "read", "/project/vendor/a.go", "^/project/src/", false},
		{"AfterFileEdit", "edit", "/project/src/a.go", "^/project/src/", true},
		{"AfterFileEdit", "edit", "/project/vendor/a.go", "^/project/src/", false},
		{"UserPromptSubmit", "", "run tests", "^run tests$", true},
		{"UserPromptSubmit", "", "hello", "^run tests$", false},
		{"SessionStart", "", "", "^$", true},
		{"SessionStart", "", "", "^shell$", false},
		{"Stop", "", "", "", true},
		{"Stop", "", "", "*", false},
	} {
		raw, err := json.MarshalIndent(map[string]any{"event": tc.event, "tool_name": tc.tool, "matcher_context": tc.context}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		p, err := Build("goose", tc.event, tc.matcher, t.TempDir(), Input{Raw: raw})
		if err != nil {
			t.Errorf("%s: %v", tc.event, err)
			continue
		}
		trigger := tc.context
		if trigger == "" {
			trigger = "session"
		}
		if p.Fires != tc.fires || p.Trigger != trigger {
			t.Errorf("%s matcher %q: fires=%v trigger=%q, want %v %q", tc.event, tc.matcher, p.Fires, p.Trigger, tc.fires, trigger)
		}
		if string(p.Body) != string(raw) {
			t.Errorf("%s changed the supplied payload: %s", tc.event, p.Body)
		}
	}
}

func TestDecide_AugmentStopNeedsAnExitZeroJSONBlock(t *testing.T) {
	r := Result{Exit: 2, Stdout: `{"hookSpecificOutput":{"hookEventName":"Stop","decision":"block","reason":"Run tests"}}`}
	if got := DecideHandler("augment", "Stop", Handler{}, r); got != Error {
		t.Errorf("Stop exit 2 with JSON = %s, want error", got)
	}
	r.Exit = 0
	if got := DecideHandler("augment", "Stop", Handler{}, r); got != Block {
		t.Errorf("Stop exit 0 with JSON = %s, want block", got)
	}
}

func TestPayloadTool_ReadsTheToolNameFromAnyPayload(t *testing.T) {
	if got := PayloadTool([]byte(`{"tool_name":"terminal"}`)); got != "terminal" {
		t.Errorf("PayloadTool = %q", got)
	}
	if got := PayloadTool([]byte(`{"event":"SessionStart"}`)); got != "" {
		t.Errorf("PayloadTool = %q, want none", got)
	}
}

// code.claude.com/docs/en/hooks: "Only letters, digits, _, -, spaces, ,,
// and | ... Exact string, or list of exact strings separated by | or ,
// with optional surrounding whitespace".
func TestMatches_ClaudeExactListsTakeCommasSpacesAndDashes(t *testing.T) {
	for _, tc := range []struct {
		matcher, value string
		want           bool
	}{
		{"Edit, Write", "Write", true},
		{"Edit | Write", "Edit", true},
		{"code-reviewer", "code-reviewer", true},
		{"code-reviewer", "code-reviewerx", false},
		{"Edit,Write", "MultiEdit", false},
	} {
		got, err := matches(tc.matcher, tc.value)
		if err != nil || got != tc.want {
			t.Errorf("matches(%q, %q) = %v, %v, want %v", tc.matcher, tc.value, got, err, tc.want)
		}
	}
}
