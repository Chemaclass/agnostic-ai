package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func devinMemoryHandler(t *testing.T) (string, string) {
	t.Helper()
	globalAgentTestHome(t)
	root := setupMemoryProject(t, "windsurf")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	data, err := os.ReadFile(".devin/hooks.v1.json")
	if err != nil {
		t.Fatalf("read emitted Devin hook: %v", err)
	}
	var hooks map[string][]struct {
		Hooks []struct {
			Command string `json:"command"`
			Timeout int    `json:"timeout"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &hooks); err != nil {
		t.Fatal(err)
	}
	groups := hooks["SessionStart"]
	if len(groups) != 1 || len(groups[0].Hooks) != 1 {
		t.Fatalf("missing single session-start handler: %s", data)
	}
	handler := groups[0].Hooks[0]
	if handler.Timeout != 10 {
		t.Errorf("timeout=%d, want explicit 10 seconds", handler.Timeout)
	}
	if !strings.HasPrefix(handler.Command, "sh -c ") || !strings.Contains(handler.Command, "DEVIN_PROJECT_DIR") || !strings.Contains(handler.Command, "--target windsurf") {
		t.Errorf("missing explicit launcher/root/target: %s", handler.Command)
	}
	if !strings.Contains(readFile(t, ".devin/rules/shared-memory-policy.md"), "memory") {
		t.Error("memory policy fallback missing")
	}
	return root, handler.Command
}

func TestSync_DevinMemoryHasAnExplicitSessionStartLauncher(t *testing.T) {
	devinMemoryHandler(t)
}

func TestHookMemory_DevinUsesProjectEnvAndTaggedContext(t *testing.T) {
	globalAgentTestHome(t)
	memoryHookProject(t, sampleIndex)
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEVIN_PROJECT_DIR", root)
	testutil.Chdir(t, t.TempDir())
	out := runHookMemory(t, "--target", "windsurf")
	var reply struct {
		HookSpecificOutput struct {
			Event   string `json:"hookEventName"`
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &reply); err != nil {
		t.Fatalf("decode Devin reply: %v\n%s", err, out)
	}
	if reply.HookSpecificOutput.Event != "SessionStart" || !strings.Contains(reply.HookSpecificOutput.Context, "CI is Ubuntu only") {
		t.Errorf("wrong Devin context: %s", out)
	}
}

func TestSync_DevinMemoryLauncherRunsInTheNamedProject(t *testing.T) {
	skipWithoutPOSIXShell(t)
	root, command := devinMemoryHandler(t)
	bin := filepath.Join(root, "node_modules", ".bin", "agnostic-ai")
	mustWriteFile(t, bin, "#!/bin/sh\nif [ \"$1 $2\" = 'project --help' ]; then exit 0; fi\n[ \"$*\" = 'project -- hook memory --target windsurf' ] || exit 2\n[ \"$PWD\" = \"$DEVIN_PROJECT_DIR\" ] || exit 3\nprintf 'PROJECT_ROOT_SELECTED\\n'\n")
	if err := os.Chmod(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEVIN_PROJECT_DIR", canonicalDir(root))
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = t.TempDir()
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "PROJECT_ROOT_SELECTED") {
		t.Errorf("launcher from foreign cwd: %v\n%s", err, out)
	}
}
