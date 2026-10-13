package claude

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestProjectKey_CutsALongNameAndAddsClaudesHash(t *testing.T) {
	path := "/tmp/" + strings.Repeat("a", 120) + "/" + strings.Repeat("b", 120) + "/project.name"

	got := projectKey(path)

	// The hash is the one Claude Code 2.1.296 computes for this path.
	want := strings.Map(func(r rune) rune {
		if r == '/' || r == '.' {
			return '-'
		}
		return r
	}, path)[:claudeProjectKeyMax] + "-ns0hdz"
	if got != want {
		t.Errorf("projectKey = %q, want %q", got, want)
	}
}

func TestNativeMemoryDir_KeysABareRepositoryOnItsGitFolder(t *testing.T) {
	config := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", config)
	t.Setenv("CLAUDE_CODE_PROJECT_DIR_NAME", "")
	parent := canonicalPath(t.TempDir())
	bare := filepath.Join(parent, "repo.git")
	gitIn(t, parent, "init", "-q", "--bare", bare)
	seed := filepath.Join(parent, "seed")
	gitIn(t, parent, "clone", "-q", bare, seed)
	gitIn(t, seed, "commit", "-q", "--allow-empty", "-m", "init")
	gitIn(t, seed, "push", "-q", "origin", "HEAD")
	wt := filepath.Join(parent, "wt1")
	gitIn(t, bare, "worktree", "add", "-q", wt)

	got, err := Adapter{}.NativeMemoryDir(wt)
	if err != nil {
		t.Fatal(err)
	}

	if want := filepath.Join(config, "projects", projectKey(bare), "memory"); got != want {
		t.Errorf("NativeMemoryDir = %s, want %s", got, want)
	}
}

func TestNativeMemoryDir_IgnoresAProjectDirNameClaudeRejects(t *testing.T) {
	config := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", config)
	dir := canonicalPath(testutil.TempCwd(t))

	for _, name := range []string{"CON", "a b", strings.Repeat("x", 65)} {
		t.Setenv("CLAUDE_CODE_PROJECT_DIR_NAME", name)
		got, err := Adapter{}.NativeMemoryDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(config, "projects", projectKey(dir), "memory"); got != want {
			t.Errorf("%q: NativeMemoryDir = %s, want %s", name, got, want)
		}
	}
}

func TestNativeMemoryDir_UsesTheFolderTheUserSettingsName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	config := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", config)
	if err := os.WriteFile(filepath.Join(config, "settings.json"), []byte(`{"autoMemoryDirectory": "~/notes/claude"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Adapter{}.NativeMemoryDir(testutil.TempCwd(t))
	if err != nil {
		t.Fatal(err)
	}

	if want := filepath.Join(home, "notes", "claude"); got != want {
		t.Errorf("NativeMemoryDir = %s, want %s", got, want)
	}
}
