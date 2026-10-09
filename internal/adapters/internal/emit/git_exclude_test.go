package emit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func runGitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// A project in a subfolder of a linked worktree excludes its file in the
// list every worktree shares, anchored at the worktree root.
func TestExcludeOutputFromGit_AnchorsAtTheCheckoutInTheSharedList(t *testing.T) {
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(base, "main")
	runGitIn(t, base, "init", "-q", main)
	runGitIn(t, main, "commit", "-q", "--allow-empty", "-m", "base")
	linked := filepath.Join(base, "linked")
	runGitIn(t, main, "worktree", "add", "-q", linked)
	project := filepath.Join(linked, "app")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, project)

	if err := ExcludeOutputFromGit(filepath.Join(".codex", "config.toml")); err != nil {
		t.Fatal(err)
	}
	if err := ExcludeOutputFromGit(filepath.Join(".codex", "config.toml")); err != nil {
		t.Fatal(err)
	}

	exclude := runGitIn(t, project, "rev-parse", "--path-format=absolute", "--git-path", "info/exclude")
	data, err := os.ReadFile(exclude)
	if err != nil {
		t.Fatal(err)
	}
	lines := 0
	for _, line := range strings.Split(string(data), "\n") {
		if line == "/app/.codex/config.toml" {
			lines++
		}
	}
	if lines != 1 {
		t.Errorf("%s lists the file %d times:\n%s", exclude, lines, data)
	}
	if err := os.MkdirAll(filepath.Join(project, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".codex", "config.toml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if status := runGitIn(t, linked, "status", "--porcelain", "--untracked-files=all"); status != "" {
		t.Errorf("git status in the linked worktree:\n%s", status)
	}
}

func TestEscapeIgnorePattern_MatchesThePathAlone(t *testing.T) {
	for path, want := range map[string]string{
		".codex/config.toml": ".codex/config.toml",
		"a*b/[x]?.json":      `a\*b/\[x]\?.json`,
		"dir/name ":          `dir/name\ `,
	} {
		if got := escapeIgnorePattern(path); got != want {
			t.Errorf("escapeIgnorePattern(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestExcludeOutputFromGit_DoesNothingOutsideAGitCheckout(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	if err := ExcludeOutputFromGit(filepath.Join(".codex", "config.toml")); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("wrote %v outside a Git checkout", entries)
	}
}
