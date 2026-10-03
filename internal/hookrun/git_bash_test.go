package hookrun

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestArgv_ClaudeOnWindowsUsesGitBashElsePowerShell(t *testing.T) {
	prev := gitBash
	t.Cleanup(func() { gitBash = prev })
	h := Handler{Command: `"$CLAUDE_PROJECT_DIR/.claude/hooks/guard.sh"`}

	gitBash = func() string { return `C:\Program Files\Git\bin\bash.exe` }
	if got := Argv("claude", "windows", h); !slices.Equal(got, []string{`C:\Program Files\Git\bin\bash.exe`, "-c", h.Command}) {
		t.Errorf("with Git Bash = %q", got)
	}
	gitBash = func() string { return "" }
	if got := Argv("claude", "windows", h); !slices.Equal(got, []string{"powershell.exe", "-NoProfile", "-Command", h.Command}) {
		t.Errorf("without Git Bash = %q", got)
	}
	if got := Argv("claude", "linux", h); got[0] != "bash" {
		t.Errorf("off Windows = %q", got)
	}
}

func TestGitBashBeside_FindsBashInTheGitInstall(t *testing.T) {
	root := t.TempDir()
	bash := filepath.Join(root, "Git", "bin", "bash.exe")
	for _, path := range []string{bash, filepath.Join(root, "Git", "cmd", "git.exe"), filepath.Join(root, "Git", "mingw64", "bin", "git.exe")} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, git := range []string{filepath.Join(root, "Git", "cmd", "git.exe"), filepath.Join(root, "Git", "mingw64", "bin", "git.exe")} {
		if got := gitBashBeside(git); got != bash {
			t.Errorf("gitBashBeside(%s) = %q, want %q", git, got, bash)
		}
	}
	if got := gitBashBeside(filepath.Join(root, "other", "bin", "git.exe")); got != "" {
		t.Errorf("a git outside a Git install = %q", got)
	}
}

func TestGitBashScript_PassesTheCommandInAFile(t *testing.T) {
	if got, cleanup, err := gitBashScript([]string{"bash", "-c", "echo hi"}); err != nil || !slices.Equal(got, []string{"bash", "-c", "echo hi"}) {
		t.Errorf("a bash that is not bash.exe = %q, %v", got, err)
	} else {
		cleanup()
	}
	got, cleanup, err := gitBashScript([]string{`C:\Git\bin\bash.exe`, "-c", `"$CLAUDE_PROJECT_DIR/x.sh"`})
	if err != nil || len(got) != 2 || got[0] != `C:\Git\bin\bash.exe` {
		t.Fatalf("argv = %q, %v", got, err)
	}
	body, err := os.ReadFile(got[1])
	if err != nil || string(body) != "\"$CLAUDE_PROJECT_DIR/x.sh\"\n" {
		t.Errorf("script = %q, %v", body, err)
	}
	cleanup()
	if _, err := os.Stat(got[1]); !os.IsNotExist(err) {
		t.Errorf("cleanup leaves the script: %v", err)
	}
}
