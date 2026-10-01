package hookrun

import (
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
		got, err := ClaudeIfRuns("Edit(src/**)", "PreToolUse", toolBody(t, "Edit", map[string]any{"file_path": tc.file}), "/p")
		if err != nil || got != tc.want {
			t.Errorf("Edit(src/**) on %s = %v, %v, want %v", tc.file, got, err, tc.want)
		}
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
