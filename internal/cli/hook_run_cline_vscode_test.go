package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
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
	payload := filepath.Join(dir, "cline-sdk-task-start.json")
	mustWriteFile(t, payload, `{"hookName":"agent_start","taskId":"fixture-task","sessionContext":{"rootSessionId":"fixture-session"}}`)
	out, err = runHookRun(t, "memory", "--target", "cline", "--payload", payload, "--format", "json")
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

func TestSync_ClineUnixRemovesLedgeredWindowsCompanion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("checks a Windows-to-Unix sync")
	}
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [cline]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "memory.yaml"), "name: memory\nevent: TaskStart\ntarget: cline\ncommand: 'true'\nx-cline:\n  runtime: vscode\n  commandWindows: 'Write-Output fixture'\n")
	syncProject(t)
	path := filepath.Join(".clinerules", "hooks", "TaskStart.ps1")
	body := "\U0000FEFF" + header.With("Write-Output fixture\n", header.FormatShell)
	mustWriteFile(t, path, body)
	state := readStateFile(dir)
	state.Outputs = append(state.Outputs, path)
	if state.OutputSums == nil {
		state.OutputSums = map[string]string{}
	}
	state.OutputSums[path] = adapters.ContentSum(body)
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, stateFilePath(dir), string(data))
	syncProject(t)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Unix sync must remove the owned Windows companion: %v", err)
	}
}

func TestHookRun_ClineVSCodeCanonicalizesNativeEvent(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [cline]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "memory.yaml"), "name: memory\nevent: taskstart\ntarget: cline\ncommand: >-\n  printf '{\"contextModification\":\"fixture fact\"}'\n")
	syncProject(t)
	out, err := runHookRun(t, "memory", "--target", "cline", "--runtime", "vscode", "--format", "json")
	if err != nil {
		t.Fatalf("diagnostic must use the event name the renderer accepts: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"adds_context": true`) || !strings.Contains(out, `"event": "TaskStart"`) {
		t.Errorf("canonical TaskStart lost its context channel: %s", out)
	}
}
