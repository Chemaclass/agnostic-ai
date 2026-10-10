package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestProjectGeneratedMemory_AncestorLocalAndUpgradeBoundary(t *testing.T) {
	for _, scenario := range []string{"non-git", "nested-git", "old-local"} {
		t.Run(scenario, func(t *testing.T) {
			outer := t.TempDir()
			if scenario == "nested-git" {
				isolateGit(t)
				git(t, outer, "init", "-q")
			}
			root := filepath.Join(outer, "project with spaces")
			mustWriteFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: [copilot]\nbuiltins: [memory]\n")
			testutil.Chdir(t, root)
			if _, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(root, ".github", "hooks", "agnostic-ai.json"))
			if err != nil {
				t.Fatal(err)
			}
			var doc struct {
				Hooks map[string][]struct{ Bash, Powershell string }
			}
			if err := json.Unmarshal(data, &doc); err != nil {
				t.Fatal(err)
			}
			handlers := doc.Hooks["SessionStart"]
			if len(handlers) != 1 {
				t.Fatalf("expected one generated memory handler: %s", data)
			}
			handler := handlers[0]
			bin := filepath.Join(root, "node_modules", ".bin", "agnostic-ai")
			body := "#!/bin/sh\nif [ \"$2\" = --help ]; then exit 0; fi\necho local-memory-marker\n"
			oldBody := "#!/bin/sh\necho 'unknown command project' >&2\nexit 2\n"
			oldName := "agnostic-ai"
			if runtime.GOOS == "windows" {
				bin += ".cmd"
				oldName += ".cmd"
				body = "@echo off\r\nif \"%~2\"==\"--help\" exit /b 0\r\necho local-memory-marker\r\n"
				oldBody = "@echo off\r\necho unknown command project 1>&2\r\nexit /b 2\r\n"
			}
			if scenario == "old-local" {
				body = oldBody
			}
			mustWriteFile(t, bin, body)
			if err := os.Chmod(bin, 0o755); err != nil {
				t.Fatal(err)
			}
			global := filepath.Join(t.TempDir(), oldName)
			mustWriteFile(t, global, oldBody)
			if err := os.Chmod(global, 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", filepath.Dir(global)+string(os.PathListSeparator)+os.Getenv("PATH"))
			child := filepath.Join(root, "src", "nested")
			if err := os.MkdirAll(child, 0o755); err != nil {
				t.Fatal(err)
			}
			command := exec.Command("sh", "-c", handler.Bash)
			if runtime.GOOS == "windows" {
				command = exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", handler.Powershell)
			}
			command.Dir = child
			out, err := command.CombinedOutput()
			if scenario == "old-local" {
				if err == nil || !strings.Contains(string(out), "upgrade") || !strings.Contains(string(out), "project") {
					t.Errorf("old local lacks controlled recovery: %v\n%s", err, out)
				}
			} else if err != nil || !strings.Contains(string(out), "local-memory-marker") {
				t.Errorf("generated nested launcher missed local binary: %v\n%s", err, out)
			}
		})
	}
}

func TestProjectGeneratedGit_OldLocalReportsUpgrade(t *testing.T) {
	root := setupGitRepo(t)
	local := projectContractBinary(t, root, "0.82.0")
	mustWriteFile(t, local, "#!/bin/sh\necho 'unknown command project' >&2\nexit 2\n")
	testutil.Chdir(t, root)
	if _, err := runCLI(t, "install-hook"); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("sh", filepath.Join(root, ".git", "hooks", "pre-commit"))
	command.Dir = root
	out, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "upgrade") || !strings.Contains(string(out), "project") {
		t.Errorf("old Git integration lacks controlled recovery: %v\n%s", err, out)
	}
}

func TestProjectGeneratedGit_ColonPathUsesLocal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows filenames cannot contain a colon")
	}
	isolateGit(t)
	root := filepath.Join(t.TempDir(), "project:local")
	local := projectContractBinary(t, root, "0.82.0")
	mustWriteFile(t, local, "#!/bin/sh\nif [ \"$2\" = --help ]; then exit 0; fi\necho local-git-marker\n")
	git(t, root, "init", "-q")
	global := filepath.Join(t.TempDir(), "agnostic-ai")
	mustWriteFile(t, global, "#!/bin/sh\nif [ \"$2\" = --help ]; then exit 0; fi\necho old-global-marker\n")
	if err := os.Chmod(global, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(global)+string(os.PathListSeparator)+os.Getenv("PATH"))
	testutil.Chdir(t, root)
	if _, err := runCLI(t, "install-hook"); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("sh", filepath.Join(root, ".git", "hooks", "pre-commit"))
	command.Dir = root
	out, err := command.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "local-git-marker") || strings.Contains(string(out), "old-global-marker") {
		t.Errorf("colon path selected global binary: %v\n%s", err, out)
	}
}
