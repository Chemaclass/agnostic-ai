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

func TestImportClaude_ProjectRootHookRunsOnCodex(t *testing.T) {
	for _, variable := range []string{"$CLAUDE_PROJECT_DIR", "${CLAUDE_PROJECT_DIR}"} {
		t.Run(variable, func(t *testing.T) {
			repo, _ := gitRepo(t)
			project := filepath.Join(repo, "packages", "app with space")
			if err := os.MkdirAll(project, 0o755); err != nil {
				t.Fatal(err)
			}
			testutil.Chdir(t, project)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
			mustWriteFile(t, "CLAUDE.md", "# Project\n")
			mustWriteFile(t, ".claude/hooks/guard.sh", "#!/bin/sh\nprintf 'ran:%s' \"$1\"\n")
			settings := map[string]any{"hooks": map[string]any{"PostToolUse": []any{map[string]any{"matcher": "Write|Edit", "hooks": []any{map[string]any{"type": "command", "command": `sh "` + variable + `/.claude/hooks/guard.sh" "arg with space"`}}}}}}
			data, err := json.Marshal(settings)
			if err != nil {
				t.Fatal(err)
			}
			mustWriteFile(t, ".claude/settings.json", string(data))
			if out, err := runCLI(t, "import", "claude"); err != nil {
				t.Fatalf("import: %v %s", err, out)
			}
			if out, err := runCLI(t, "sync", "--only", "codex"); err != nil {
				t.Fatalf("sync: %v %s", err, out)
			}
			hook := firstGlobalHandler(t, readGlobalJSON(t, ".codex/hooks.json"), "PostToolUse")
			command, _ := hook["command"].(string)
			nested := filepath.Join(project, "child")
			if err := os.Mkdir(nested, 0o755); err != nil {
				t.Fatal(err)
			}
			run := exec.Command("sh", "-c", command)
			run.Dir = nested
			for _, item := range os.Environ() {
				if !strings.HasPrefix(item, "CLAUDE_PROJECT_DIR=") {
					run.Env = append(run.Env, item)
				}
			}
			output, err := run.CombinedOutput()
			if err != nil || string(output) != "ran:arg with space" {
				t.Errorf("command=%s output=%q err=%v", command, output, err)
			}
		})
	}
}

func TestSyncGlobal_HookRootUsesRuntimeProject(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\nevent: PreToolUse\ntarget: codex\ncommand: 'printf %s \"${CLAUDE_PROJECT_DIR}\"'\n")
	if _, _, err := runGlobalAgentTest("--only", "codex"); err != nil {
		t.Fatal(err)
	}
	command := firstGlobalHandler(t, readGlobalJSON(t, filepath.Join(home, ".codex", "hooks.json")), "PreToolUse")["command"].(string)
	repo, _ := gitRepo(t)
	child := filepath.Join(repo, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	run := exec.Command("sh", "-c", command)
	run.Dir = child
	run.Env = []string{"PATH=" + os.Getenv("PATH")}
	output, err := run.CombinedOutput()
	canonical, canonicalErr := filepath.EvalSymlinks(repo)
	if canonicalErr != nil {
		t.Fatal(canonicalErr)
	}
	if err != nil || string(output) != filepath.ToSlash(canonical) {
		t.Errorf("global hook bound wrong root: command=%s result=%q err=%v want=%s", command, output, err, canonical)
	}
}

func TestSyncGlobal_NonPOSIXHookRootStaysUnchanged(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\nevent: PreToolUse\ntarget: codex\nshell: powershell\ncommand: '\"$CLAUDE_PROJECT_DIR/custom/guard.ps1\"'\n")
	if _, _, err := runGlobalAgentTest("--only", "codex"); err != nil {
		t.Fatal(err)
	}
	command := firstGlobalHandler(t, readGlobalJSON(t, filepath.Join(home, ".codex", "hooks.json")), "PreToolUse")["command"].(string)
	if !strings.Contains(command, `"$CLAUDE_PROJECT_DIR/custom/guard.ps1"`) || strings.Contains(command, "git rev-parse") {
		t.Errorf("non-POSIX root changed: %s", command)
	}
}

func TestSyncGlobal_WindowsHookRootWarnsAndHonorsError(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: windows-guard\nevent: PreToolUse\ntarget: codex\ncommand: guard.sh\ncommandWindows: '$env:CLAUDE_PROJECT_DIR/custom/guard.ps1'\n")
	_, warnings, err := runGlobalAgentTest("--only", "codex")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warnings, "windows-guard") || !strings.Contains(warnings, "commandWindows") {
		t.Errorf("missing named Windows warning: %s", warnings)
	}
	handler := firstGlobalHandler(t, readGlobalJSON(t, filepath.Join(home, ".codex", "hooks.json")), "PreToolUse")
	if handler["commandWindows"] != `$env:CLAUDE_PROJECT_DIR/custom/guard.ps1` {
		t.Errorf("Windows override changed: %v", handler["commandWindows"])
	}
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "on-unsupported: error\n")
	if _, _, err := runGlobalAgentTest("--only", "codex"); err == nil || !strings.Contains(err.Error(), "windows-guard") {
		t.Errorf("want named global Windows error, got %v", err)
	}
}
