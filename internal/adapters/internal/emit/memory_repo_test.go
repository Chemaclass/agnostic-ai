package emit

import (
	"os"
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
	resolved, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(resolved, "local", "memory") + string(filepath.Separator) + "proj-"; !strings.HasPrefix(main, want) {
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
		got, err := PersonalMemoryDirFor(cfg, ".codex/config.toml", "codex")
		if err != nil || filepath.IsAbs(got) {
			t.Errorf("%s: got %q, %v; want the checkout store", name, got, err)
		}
	}
	ignored := &config.Config{Memory: repo, Gitignore: config.Gitignore{Enabled: true, Commit: []string{"cursor:reviews"}}}
	if got, err := PersonalMemoryDirFor(ignored, ".codex/config.toml", "codex"); err != nil || !filepath.IsAbs(got) {
		t.Errorf("ignored output: got %q, %v; want the repo store", got, err)
	}
}

func TestPersonalMemoryDirFor_KeepsTheCheckoutStoreWithAnAllowLine(t *testing.T) {
	cfg := &config.Config{
		Memory:    config.MemoryConfig{Personal: config.PersonalMemoryRepo},
		Gitignore: config.Gitignore{Enabled: true, Allow: []string{"opencode.json"}},
	}
	if got, err := PersonalMemoryDirFor(cfg, "opencode.json", "opencode"); err != nil || filepath.IsAbs(got) {
		t.Errorf("got %q, %v; want the checkout store", got, err)
	}
}

func TestRenderMemoryBlock_KeepsAnAbsolutePersonalIndex(t *testing.T) {
	index := filepath.Join(t.TempDir(), "store", "MEMORY.md")

	got := RenderMemoryBlock(".claude/CLAUDE.md", index)

	if !strings.Contains(got, "\n@"+filepath.ToSlash(index)+"\n") {
		t.Errorf("personal import not absolute:\n%s", got)
	}
}

func TestPersonalMemoryDirFor_RequiresAProjectRelativeIgnoredOutput(t *testing.T) {
	root := testutil.TempCwd(t)
	cfg := &config.Config{
		Memory:    config.MemoryConfig{Personal: config.PersonalMemoryRepo},
		Gitignore: config.Gitignore{Enabled: true},
	}
	for _, path := range []string{filepath.Join(root, "CLAUDE.md"), "../CLAUDE.md", "../../CLAUDE.md"} {
		got, err := PersonalMemoryDirFor(cfg, path, "claude")
		if err != nil || filepath.IsAbs(got) {
			t.Errorf("output %s: got %q, %v; want the checkout store", path, got, err)
		}
	}
}

// Codex refuses a writable root whose path goes through a symlink, so a
// linked AGNOSTIC_AI_HOME resolves to the folder it points at.
func TestPersonalMemoryDir_ResolvesALinkedHome(t *testing.T) {
	real, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "home-link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	t.Setenv("AGNOSTIC_AI_HOME", link)
	repo := filepath.Join(t.TempDir(), "proj")
	gitIn(t, filepath.Dir(repo), "init", "-q", "proj")
	cfg := &config.Config{Memory: config.MemoryConfig{Personal: config.PersonalMemoryRepo}}

	got, err := PersonalMemoryDir(cfg, repo)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(real, "local", "memory") + string(filepath.Separator); !strings.HasPrefix(got, want) {
		t.Errorf("store = %s, want under %s", got, want)
	}
}

// An index an earlier sync wrote through the symlink is still recognised
// as a personal index, so it leaves the list.
func TestWithoutStalePersonalIndexes_DropsAnIndexWrittenThroughALink(t *testing.T) {
	real, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "home-link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	t.Setenv("AGNOSTIC_AI_HOME", link)
	old := filepath.ToSlash(filepath.Join(link, "local", "memory", "proj-1", "MEMORY.md"))
	current := filepath.ToSlash(filepath.Join(real, "local", "memory", "proj-1", "MEMORY.md"))

	got := WithoutStalePersonalIndexes([]string{"AGENTS.md", old, current}, []string{current})
	if strings.Join(got, ",") != "AGENTS.md,"+current {
		t.Errorf("list = %v, want the linked path dropped", got)
	}
}
