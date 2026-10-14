package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestHookMemory_ClineUsesContextModificationReply(t *testing.T) {
	text := "## Shared memory\n\n- fixture fact\n"
	out, err := memoryHookReply("cline", text)
	if err != nil {
		t.Fatal(err)
	}
	var reply struct {
		ContextModification string `json:"contextModification"`
	}
	if err := json.Unmarshal([]byte(out), &reply); err != nil {
		t.Fatalf("decode Cline memory reply: %v", err)
	}
	if reply.ContextModification != text {
		t.Errorf("Cline memory reply lost context: %s", out)
	}
}

func TestHookRun_ClineVSCodeTaskStartAddsContext(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [cline]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "memory.yaml"), "name: memory\nevent: TaskStart\ntarget: cline\ncommand: >-\n  printf '{\"contextModification\":\"fixture fact\"}'\n")
	syncProject(t)
	out, err := runHookRun(t, "memory", "--target", "cline", "--runtime", "vscode", "--format", "json")
	if err != nil {
		t.Fatalf("run VS Code TaskStart: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"adds_context": true`) || !strings.Contains(out, `"event": "TaskStart"`) {
		t.Errorf("VS Code TaskStart did not report its context channel: %s", out)
	}
	out, err = runHookRun(t, "memory", "--target", "cline", "--format", "json")
	if err != nil {
		t.Fatalf("run SDK TaskStart: %v\n%s", err, out)
	}
	if strings.Contains(out, `"adds_context": true`) {
		t.Errorf("default SDK TaskStart must remain detached: %s", out)
	}
}

func TestHookRun_VSCodeRuntimeRejectsOtherTargets(t *testing.T) {
	hookRunProject(t, "name: guard\nevent: SessionStart\ncommand: 'true'\n")
	_, err := runHookRun(t, "guard", "--target", "codex", "--runtime", "vscode")
	if err == nil || !strings.Contains(err.Error(), "runtime vscode requires --target cline") {
		t.Errorf("runtime must reject an incompatible target: %v", err)
	}
}
