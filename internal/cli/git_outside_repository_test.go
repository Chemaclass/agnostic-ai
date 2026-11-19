package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/gitquery"
)

// syncGitRunsInsideRepository is how many Git processes one sync started
// in a fresh repository before the outside-repository shortcut landed.
const syncGitRunsInsideRepository = 5

func TestSync_GitRunsOutsideRepository(t *testing.T) {
	log := countGitRuns(t)
	newProject(t)
	if _, err := runCLI(t, "sync"); err != nil {
		t.Fatal(err)
	}
	if runs := gitRunLines(t, log); len(runs) > 2 {
		t.Errorf("sync outside a repository started %d Git processes, want at most 2:\n%s", len(runs), strings.Join(runs, "\n"))
	}
}

func TestSync_GitRunsInsideRepository(t *testing.T) {
	log := countGitRuns(t)
	dir := newProject(t)
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if err := os.Truncate(log, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "sync"); err != nil {
		t.Fatal(err)
	}
	if runs := gitRunLines(t, log); len(runs) != syncGitRunsInsideRepository {
		t.Errorf("sync inside a repository started %d Git processes, want %d:\n%s", len(runs), syncGitRunsInsideRepository, strings.Join(runs, "\n"))
	}
}

func TestTrackedFiles_FollowsGitDirOutsideRepository(t *testing.T) {
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"add", "tracked.txt"}} {
		if args[0] == "add" {
			if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	outside := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(outside))
	t.Setenv("GIT_DIR", filepath.Join(repo, ".git"))
	t.Setenv("GIT_WORK_TREE", repo)
	gitquery.Reset()
	t.Cleanup(gitquery.Reset)

	files, ok := trackedFiles(outside)
	if !ok || !slices.Contains(files, "tracked.txt") {
		t.Errorf("trackedFiles = %v, %v; want tracked.txt from GIT_DIR", files, ok)
	}
}

// countGitRuns puts a git wrapper first on PATH that logs one line per
// process, and returns the log path.
func countGitRuns(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the counting wrapper is a shell script")
	}
	real, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not found")
	}
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "git.log")
	if err := os.WriteFile(log, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho \"$*\" >> '" + log + "'\nexec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(t.TempDir()))
	return log
}

func gitRunLines(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	return strings.FieldsFunc(string(data), func(r rune) bool { return r == '\n' })
}
