package cli

import (
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
