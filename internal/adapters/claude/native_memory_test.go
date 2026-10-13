package claude

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestNativeMemoryDir_KeysTheStoreOnTheRepositoryUnderClaudeConfigDir(t *testing.T) {
	config := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", config)
	t.Setenv("CLAUDE_CODE_PROJECT_DIR_NAME", "")
	repo := canonicalPath(t.TempDir())
	gitIn(t, repo, "init", "-q")

	got, err := Adapter{}.NativeMemoryDir(repo)
	if err != nil {
		t.Fatal(err)
	}

	if want := filepath.Join(config, "projects", projectKey(repo), "memory"); got != want {
		t.Errorf("NativeMemoryDir = %s, want %s", got, want)
	}
}

func TestNativeMemoryDir_SharesOneStoreAcrossWorktrees(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("CLAUDE_CODE_PROJECT_DIR_NAME", "")
	repo := canonicalPath(t.TempDir())
	gitIn(t, repo, "init", "-q")
	gitIn(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	linked := filepath.Join(t.TempDir(), "linked")
	gitIn(t, repo, "worktree", "add", "-q", linked)

	main, err := Adapter{}.NativeMemoryDir(repo)
	if err != nil {
		t.Fatal(err)
	}
	other, err := Adapter{}.NativeMemoryDir(linked)
	if err != nil {
		t.Fatal(err)
	}

	if other != main {
		t.Errorf("worktree store = %s, want the main checkout's %s", other, main)
	}
}

func TestNativeMemoryDir_UsesTheProjectDirNameWithAConfigDir(t *testing.T) {
	config := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", config)
	t.Setenv("CLAUDE_CODE_PROJECT_DIR_NAME", "my-project")

	got, err := Adapter{}.NativeMemoryDir(testutil.TempCwd(t))
	if err != nil {
		t.Fatal(err)
	}

	if want := filepath.Join(config, "projects", "my-project", "memory"); got != want {
		t.Errorf("NativeMemoryDir = %s, want %s", got, want)
	}
}

func TestNativeMemoryDir_IgnoresTheProjectDirNameWithoutAConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CLAUDE_CODE_PROJECT_DIR_NAME", "my-project")
	dir := canonicalPath(testutil.TempCwd(t))

	got, err := Adapter{}.NativeMemoryDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	if want := filepath.Join(home, ".claude", "projects", projectKey(dir), "memory"); got != want {
		t.Errorf("NativeMemoryDir = %s, want %s", got, want)
	}
}

func TestProjectKey_TurnsEveryCharacterOutsideLettersAndDigitsIntoADash(t *testing.T) {
	if got := projectKey("/Users/me/code/my.app_2"); got != "-Users-me-code-my-app-2" {
		t.Errorf("projectKey = %q", got)
	}
}
