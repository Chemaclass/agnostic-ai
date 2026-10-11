package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/hookrun"
)

func TestSyncGlobal_CopilotPortableHookExecutesFromHomeWithSpaces(t *testing.T) {
	home, source := globalAgentTestHome(t)
	native := filepath.Join(home, "copilot native home with spaces")
	receipt := filepath.Join(home, "hook receipt")
	t.Setenv("COPILOT_HOME", native)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [copilot]\n")
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "receipt.yaml"), "name: receipt\ntarget: copilot\non: before-tool\ncommand: |\n  printf '%s\\n' \"$AGNOSTIC_AI_TARGET\" > \"$AGNOSTIC_RECEIPT\"\nenv:\n  AGNOSTIC_RECEIPT: '"+filepath.ToSlash(receipt)+"'\n")
	if _, warnings, err := runGlobalAgentTest("--only", "copilot"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	doc := readGlobalJSON(t, filepath.Join(native, "hooks", "agnostic-ai.json"))
	handlers, _ := doc["hooks"].(map[string]any)["PreToolUse"].([]any)
	if len(handlers) != 1 {
		t.Fatalf("emitted receipt handlers: %v", handlers)
	}
	handler := handlers[0].(map[string]any)
	command, _ := handler["command"].(string)
	if command == "" {
		t.Fatal("portable hook has no native Bash command")
	}
	argv := hookrun.Argv("claude", runtime.GOOS, hookrun.Handler{Shell: "bash"})
	if runtime.GOOS == "windows" && !filepath.IsAbs(argv[0]) {
		t.Fatal("native Bash acceptance requires Git Bash, not a PATH launcher")
	}
	env, _ := handler["env"].(map[string]any)
	for key, value := range env {
		text, ok := value.(string)
		if !ok {
			t.Fatalf("native hook env %s is not a string", key)
		}
		t.Setenv(key, text)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], "-c", command)
	cmd.WaitDelay = 250 * time.Millisecond
	cmd.Stdin = strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"true"}}`)
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("execute actual portable command %q: %v\n%s", command, err, out)
	}
	raw, err := os.ReadFile(receipt)
	if err != nil || string(raw) != "copilot\n" {
		t.Errorf("portable command did not produce its receipt: %q, %v", raw, err)
	}
}

func TestSyncGlobal_CopilotWindowsRootedScriptUsesOnlyNativePath(t *testing.T) {
	home, source := globalAgentTestHome(t)
	native := filepath.Join(home, "copilot native home with spaces")
	t.Setenv("COPILOT_HOME", native)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [copilot]\n")
	const body = "Write-Output checked\n"
	mustWriteGlobalTest(t, filepath.Join(source, "scripts", "guard.ps1"), body)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\ntarget: copilot\nevent: SessionStart\ncommand: echo checked\ncommandWindows: '& \"$CLAUDE_PROJECT_DIR/.agnostic-ai/scripts/guard.ps1\"'\n")
	if _, warnings, err := runGlobalAgentTest("--only", "copilot"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	doc := readGlobalJSON(t, filepath.Join(native, "hooks", "agnostic-ai.json"))
	handlers, _ := doc["hooks"].(map[string]any)["SessionStart"].([]any)
	if len(handlers) != 1 {
		t.Fatalf("emitted handlers: %v", handlers)
	}
	windows, _ := handlers[0].(map[string]any)["powershell"].(string)
	copied := filepath.Join(native, "hooks", "hooks", "guard.ps1")
	want := "& \"" + filepath.ToSlash(copied) + "\""
	if windows != want {
		t.Errorf("Windows rooted script command = %q, want %q", windows, want)
	}
	if raw := readGlobalTest(t, copied); raw != body {
		t.Errorf("copied native script changed: %s", raw)
	}
	if _, _, err := runGlobalAgentTest("--only", "copilot", "--check"); err != nil {
		t.Fatal(err)
	}
}
