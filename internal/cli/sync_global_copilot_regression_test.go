package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSyncGlobal_CopilotImportKeepsExistingGlobalScriptSpec(t *testing.T) {
	for _, portable := range []bool{false, true} {
		name := "native"
		if portable {
			name = "portable"
		}
		t.Run(name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			native := filepath.Join(home, "native home")
			t.Setenv("COPILOT_HOME", native)
			mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [copilot]\n")
			const body = "#!/bin/sh\nprintf '%s\\n' checked\n"
			mustWriteGlobalTest(t, filepath.Join(source, "scripts", "guard.sh"), body)
			declaration := "event: PreToolUse\n"
			if portable {
				declaration = "on: before-tool\n"
			}
			mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\ntarget: copilot\n"+declaration+"command: .agnostic-ai/scripts/guard.sh\n")
			if _, _, err := runGlobalAgentTest("--only", "copilot"); err != nil {
				t.Fatal(err)
			}
			managed := filepath.Join(native, "hooks", "agnostic-ai.json")
			before := readGlobalTest(t, managed)
			if _, warnings, err := runImportGlobalTest("copilot"); err != nil {
				t.Fatalf("import: %v\n%s", err, warnings)
			}
			specs, err := os.ReadDir(filepath.Join(source, "hooks"))
			if err != nil {
				t.Fatal(err)
			}
			if len(specs) != 1 {
				t.Errorf("import duplicated existing global source spec: %v", specs)
			}
			if _, _, err := runGlobalAgentTest("--only", "copilot"); err != nil {
				t.Fatalf("resync: %v", err)
			}
			got := readGlobalJSON(t, managed)
			handlers := got["hooks"].(map[string]any)["PreToolUse"].([]any)
			if len(handlers) != 1 {
				t.Errorf("global sync/import/sync emitted %d handlers, want one", len(handlers))
			}
			if after := readGlobalTest(t, managed); after != before {
				t.Errorf("import changed emitted command or wrapper: before %s\nafter %s", before, after)
			}
			copied := filepath.Join(native, "hooks", "hooks", "guard.sh")
			if got := readGlobalTest(t, copied); got != body {
				t.Errorf("copied script changed: %s", got)
			}
			if _, _, err := runGlobalAgentTest("--only", "copilot", "--check"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSyncGlobal_CopilotRemovingManagedHookKeepsHandwrittenFileVersion(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("COPILOT_HOME", "")
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [copilot]\n")
	specPath := filepath.Join(source, "hooks", "managed.yaml")
	mustWriteGlobalTest(t, specPath, "name: managed\ntarget: copilot\nevent: SessionStart\ncommand: echo managed\n")
	if _, _, err := runGlobalAgentTest("--only", "copilot"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".copilot", "hooks", "agnostic-ai.json")
	doc := readGlobalJSON(t, path)
	hooks := doc["hooks"].(map[string]any)
	hooks["SessionEnd"] = []any{map[string]any{"type": "command", "command": "echo handwritten"}}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, path, string(raw))
	if err := os.Remove(specPath); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest("--only", "copilot"); err != nil {
		t.Fatal(err)
	}
	after := readGlobalJSON(t, path)
	if after["version"] != float64(1) {
		t.Errorf("handwritten handler file lost required version: %v", after)
	}
	remaining, _ := after["hooks"].(map[string]any)["SessionEnd"].([]any)
	if len(remaining) != 1 || remaining[0].(map[string]any)["command"] != "echo handwritten" {
		t.Errorf("handwritten handler changed: %v", after)
	}
	if _, _, err := runGlobalAgentTest("--only", "copilot", "--check"); err != nil {
		t.Errorf("unchanged handwritten file check: %v", err)
	}
}

func TestSyncGlobal_CopilotScopedWindowsScriptUsesNativeHome(t *testing.T) {
	home, source := globalAgentTestHome(t)
	native := filepath.Join(home, "native home")
	t.Setenv("COPILOT_HOME", native)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [copilot]\n")
	const windowsBody = "Write-Output checked\n"
	mustWriteGlobalTest(t, filepath.Join(source, "scripts", "guard.ps1"), windowsBody)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\ntarget: copilot\nevent: SessionStart\ncommand: echo checked\nx-copilot:\n  commandWindows: \"& .agnostic-ai/scripts/guard.ps1\"\n")
	if _, _, err := runGlobalAgentTest("--only", "copilot"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(native, "hooks", "agnostic-ai.json")
	doc := readGlobalJSON(t, path)
	handlers := doc["hooks"].(map[string]any)["SessionStart"].([]any)
	if len(handlers) != 1 {
		t.Fatalf("handlers: %v", handlers)
	}
	windows, _ := handlers[0].(map[string]any)["powershell"].(string)
	expected := filepath.ToSlash(filepath.Join(native, "hooks", "hooks", "guard.ps1"))
	if !strings.Contains(windows, expected) || strings.Contains(windows, ".github/") || strings.Contains(windows, ".agnostic-ai/scripts") {
		t.Errorf("target-scoped Windows command points outside native home: %s", windows)
	}
	copied, err := os.ReadFile(filepath.FromSlash(expected))
	if err != nil || string(copied) != windowsBody {
		t.Errorf("Windows copied script = %q, %v", copied, err)
	}
	if _, _, err := runGlobalAgentTest("--only", "copilot", "--check"); err != nil {
		t.Fatal(err)
	}
}

func TestSyncGlobal_CopilotMatchingNeighborCannotHideEditedImportedSource(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("COPILOT_HOME", "")
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [copilot]\n")
	dir := filepath.Join(home, ".copilot", "hooks")
	original := filepath.Join(dir, "z-personal.json")
	const native = `{"version":1,"hooks":{"SessionStart":[{"type":"command","command":"echo original"}]}}`
	mustWriteGlobalTest(t, original, native)
	if _, _, err := runImportGlobalTest("copilot"); err != nil {
		t.Fatal(err)
	}
	earlier := filepath.Join(dir, "a-matching.json")
	mustWriteGlobalTest(t, earlier, native)
	edited := strings.ReplaceAll(native, "echo original", "echo changed")
	mustWriteGlobalTest(t, original, edited)
	_, _, err := runGlobalAgentTest("--only", "copilot")
	if err == nil || !strings.Contains(err.Error(), original) || !strings.Contains(err.Error(), "differs from its source file") {
		t.Errorf("matching earlier file bypassed source conflict: %v", err)
	}
	if got := readGlobalTest(t, original); got != edited {
		t.Errorf("conflict changed edited native file: %s", got)
	}
	if got := readGlobalTest(t, earlier); got != native {
		t.Errorf("conflict changed neighboring file: %s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "agnostic-ai.json")); !os.IsNotExist(err) {
		t.Errorf("conflict generated a native file: %v", err)
	}
}

func TestSyncGlobal_CopilotMemoryWindowsCommandUsesCopilotReplyFormat(t *testing.T) {
	home, source := globalAgentTestHome(t)
	native := filepath.Join(home, "copilot native home")
	t.Setenv("COPILOT_HOME", native)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [copilot]\nbuiltins: [memory]\n")
	if _, warnings, err := runGlobalAgentTest("--only", "copilot"); err != nil {
		t.Fatalf("global memory sync: %v\n%s", err, warnings)
	}
	assertTarget := func(path string) {
		t.Helper()
		doc := readGlobalJSON(t, path)
		handlers, _ := doc["hooks"].(map[string]any)["SessionStart"].([]any)
		if len(handlers) != 1 {
			t.Fatalf("expected one real memory hook in %s: %v", path, handlers)
		}
		handler := handlers[0].(map[string]any)
		bash, _ := handler["bash"].(string)
		windows, _ := handler["powershell"].(string)
		if !strings.Contains(bash, "hook memory --target copilot") || strings.Contains(bash, "hook memory --target codex") {
			t.Errorf("%s: Bash memory hook uses wrong reply format: %s", path, bash)
		}
		if !strings.Contains(windows, "hook memory --target copilot") || strings.Contains(windows, "hook memory --target codex") {
			t.Errorf("%s: Windows memory hook uses wrong reply format: %s", path, windows)
		}
	}
	assertTarget(filepath.Join(native, "hooks", "agnostic-ai.json"))
	project := t.TempDir()
	mustWriteGlobalTest(t, filepath.Join(project, "agnostic-ai.yaml"), "targets: [copilot]\nbuiltins: [memory]\n")
	testutil.Chdir(t, project)
	cmd := NewRootCmd("test")
	cmd.SetArgs([]string{"sync", "-t", "copilot"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("project memory control sync: %v", err)
	}
	assertTarget(filepath.Join(project, ".github", "hooks", "agnostic-ai.json"))
}
