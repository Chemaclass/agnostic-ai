package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A --payload names the tool itself, and OpenHands and Augment hand that
// name to the hook in their env.
func TestHookRun_RawPayloadSetsTheToolEnv(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [openhands, augment]\n")
	script := filepath.Join(dir, ".agnostic-ai", "scripts", "tool.sh")
	mustWrite(t, script, "#!/bin/sh\ncat >/dev/null\necho \"tool=${OPENHANDS_TOOL_NAME:-}${AUGMENT_TOOL_NAME:-}\"\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "tool.yaml"), "name: tool\nevent: PreToolUse\ncommand: .agnostic-ai/scripts/tool.sh\n")
	mustSync(t)
	payload := filepath.Join(dir, "call.json")
	mustWrite(t, payload, `{"event_type":"PreToolUse","hook_event_name":"PreToolUse","tool_name":"terminal"}`)

	out, err := runHookRun(t, "tool", "--payload", payload)
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	if strings.Count(out, "stdout: tool=terminal") != 2 {
		t.Errorf("want both targets to see tool=terminal:\n%s", out)
	}
}

func TestHookRun_RawToolPayloadRespectsTheMatcher(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := hookRunProject(t, "name: guard\nevent: PreToolUse\ntarget: claude\nmatcher: Bash\ncommand: 'exit 2'\n")
	payload := filepath.Join(dir, "call.json")
	mustWrite(t, payload, `{"hook_event_name":"PreToolUse","tool_name":"Read","tool_input":{"file_path":"README.md"}}`)

	out, err := runHookRun(t, "guard", "--payload", payload, "--expect", "allow")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	if !strings.Contains(out, `matcher "Bash" does not match Read`) {
		t.Errorf("output misses the unmatched tool:\n%s", out)
	}
}

func TestHookRun_GooseRawPayloadUsesMatcherContext(t *testing.T) {
	skipWithoutPOSIXShell(t)
	for _, tc := range []struct {
		event, tool, context, matcher string
		fires                         bool
	}{
		{"PreToolUse", "developer__shell", "shell", "^shell$", true},
		{"PreToolUse", "shell", "developer__shell", "^shell$", false},
		{"BeforeShellExecution", "shell", "git push", "^git push$", true},
		{"BeforeShellExecution", "shell", "git status", "^git push$", false},
		{"AfterFileEdit", "edit", "/project/src/a.go", "^/project/src/", true},
		{"AfterFileEdit", "edit", "/project/vendor/a.go", "^/project/src/", false},
		{"UserPromptSubmit", "", "run tests", "^run tests$", true},
		{"UserPromptSubmit", "", "hello", "^run tests$", false},
	} {
		t.Run(tc.event+"/"+tc.context, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [goose]\n")
			mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"),
				"name: guard\nevent: "+tc.event+"\nmatcher: "+tc.matcher+"\ncommand: |\n  echo '{\"decision\":\"allow\"}'\n")
			mustSync(t)
			raw, err := json.Marshal(map[string]any{"event": tc.event, "tool_name": tc.tool, "matcher_context": tc.context})
			if err != nil {
				t.Fatal(err)
			}
			payload := filepath.Join(dir, "call.json")
			mustWrite(t, payload, string(raw))

			out, err := runHookRun(t, "guard", "--payload", payload, "--expect", "allow")
			if err != nil {
				t.Fatalf("err = %v\n%s", err, out)
			}
			if ran := strings.Contains(out, `stdout: {"decision":"allow"}`); ran != tc.fires {
				t.Errorf("handler ran=%v, want %v:\n%s", ran, tc.fires, out)
			}
			if !tc.fires && !strings.Contains(out, "does not match "+tc.context) {
				t.Errorf("output misses the unmatched context:\n%s", out)
			}
		})
	}
}

func TestHookRun_AugmentStopBlocksWithTheNestedDecision(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [augment]\n")
	script := filepath.Join(dir, ".agnostic-ai", "scripts", "stop.sh")
	mustWrite(t, script, `#!/bin/sh
cat >/dev/null
echo '{"hookSpecificOutput":{"hookEventName":"Stop","decision":"block","reason":"Run tests"}}'
`)
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "stop.yaml"), "name: stop\nevent: Stop\ncommand: .agnostic-ai/scripts/stop.sh\n")
	mustSync(t)
	payload := filepath.Join(dir, "stop.json")
	mustWrite(t, payload, `{"hook_event_name":"Stop"}`)

	out, err := runHookRun(t, "stop", "--payload", payload, "--expect", "block")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
}
