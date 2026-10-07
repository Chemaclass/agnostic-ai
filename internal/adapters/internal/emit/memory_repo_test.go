package emit

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestRepoSlug_NamesTheRepositoryAndHashesItsPath(t *testing.T) {
	got := RepoSlug("/src/My Repo/.git")
	if !regexp.MustCompile(`^My-Repo-[0-9a-f]{8}$`).MatchString(got) {
		t.Errorf("slug = %q", got)
	}
	if RepoSlug("/src/My Repo/.git") != got {
		t.Error("slug is not stable")
	}
	if other := RepoSlug("/elsewhere/My Repo/.git"); other == got {
		t.Errorf("two clones of one name share slug %q", other)
	}
}

func TestRepoSlug_NamesABareRepositoryWithoutItsSuffix(t *testing.T) {
	if got := RepoSlug("/srv/tools.git"); !strings.HasPrefix(got, "tools-") {
		t.Errorf("slug = %q", got)
	}
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestPersonalMemoryDir_SharesOneRepoStoreAcrossWorktrees(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AGNOSTIC_AI_HOME", home)
	repo := filepath.Join(t.TempDir(), "proj")
	gitIn(t, filepath.Dir(repo), "init", "-q", "proj")
	gitIn(t, repo, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "init")
	tree := filepath.Join(t.TempDir(), "tree")
	gitIn(t, repo, "worktree", "add", "-q", tree)
	cfg := &config.Config{Memory: config.MemoryConfig{Personal: config.PersonalMemoryRepo}}

	main, err := PersonalMemoryDir(cfg, repo)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := PersonalMemoryDir(cfg, tree)
	if err != nil {
		t.Fatal(err)
	}
	if main != wt {
		t.Errorf("worktree store %s, main store %s", wt, main)
	}
	if want := filepath.Join(home, "local", "memory") + string(filepath.Separator) + "proj-"; !strings.HasPrefix(main, want) {
		t.Errorf("store = %s, want under %s", main, want)
	}
}

func TestPersonalMemoryDir_KeepsTheCheckoutStoreByDefault(t *testing.T) {
	got, err := PersonalMemoryDir(&config.Config{}, "root")
	if err != nil || got != filepath.Join("root", ".agnostic-ai", "local", "memory") {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestPersonalMemoryDirFor_KeepsTheCheckoutStoreInCommittedFiles(t *testing.T) {
	testutil.TempCwd(t)
	gitIn(t, ".", "init", "-q")
	repo := config.MemoryConfig{Personal: config.PersonalMemoryRepo}
	for name, cfg := range map[string]*config.Config{
		"gitignore off":          {Memory: repo},
		"instructions committed": {Memory: repo, Gitignore: config.Gitignore{Enabled: true, Commit: []string{"instructions"}}},
		"codex kind committed":   {Memory: repo, Gitignore: config.Gitignore{Enabled: true, Commit: []string{"codex:mcps"}}},
	} {
		got, err := PersonalMemoryDirFor(cfg, "codex")
		if err != nil || filepath.IsAbs(got) {
			t.Errorf("%s: got %q, %v; want the checkout store", name, got, err)
		}
	}
	ignored := &config.Config{Memory: repo, Gitignore: config.Gitignore{Enabled: true, Commit: []string{"cursor:reviews"}}}
	if got, err := PersonalMemoryDirFor(ignored, "codex"); err != nil || !filepath.IsAbs(got) {
		t.Errorf("ignored output: got %q, %v; want the repo store", got, err)
	}
}

func TestRenderMemoryBlock_KeepsAnAbsolutePersonalIndex(t *testing.T) {
	index := filepath.Join(t.TempDir(), "store", "MEMORY.md")

	got := RenderMemoryBlock(".claude/CLAUDE.md", index)

	if !strings.Contains(got, "\n@"+filepath.ToSlash(index)+"\n") {
		t.Errorf("personal import not absolute:\n%s", got)
	}
}
