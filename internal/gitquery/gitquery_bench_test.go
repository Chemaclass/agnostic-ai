package gitquery

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// BenchmarkLayoutAtFresh measures one uncached layout read in a work tree,
// a bare repository, and a folder outside any repository, three subjects
// side by side:
//   - status-quo: the earlier two-call read, kept below as
//     layoutAtFreshTwoCalls, which retries without --show-toplevel.
//   - proposed:   LayoutAtFresh, one rev-parse with --show-toplevel last.
//   - baseline:   one rev-parse whose output is discarded, the floor.
func BenchmarkLayoutAtFresh(b *testing.B) {
	worktree := b.TempDir()
	benchGit(b, worktree, "init", "-q")
	bare := b.TempDir()
	benchGit(b, bare, "init", "-q", "--bare")
	outside := b.TempDir()
	b.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(outside))

	folders := []struct {
		name string
		dir  string
	}{
		{"worktree", worktree},
		{"bare", bare},
		{"outside", outside},
	}
	subjects := []struct {
		name string
		read func(string) (Layout, error)
	}{
		{"status-quo", layoutAtFreshTwoCalls},
		{"proposed", LayoutAtFresh},
		{"baseline", naiveLayoutRead},
	}
	for _, folder := range folders {
		for _, subject := range subjects {
			b.Run(folder.name+"/"+subject.name, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					_, _ = subject.read(folder.dir)
				}
			})
		}
	}
}

func benchGit(b *testing.B, dir string, args ...string) {
	b.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		b.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func naiveLayoutRead(dir string) (Layout, error) {
	err := isolateGitCommand(exec.Command("git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir", "--show-prefix", "--show-toplevel")).Run()
	return Layout{}, err
}

func layoutAtFreshTwoCalls(dir string) (Layout, error) {
	key := absKey(dir)
	const base = "--path-format=absolute"
	out, err := isolateGitCommand(exec.Command("git", "-C", key, "rev-parse", base, "--show-toplevel", "--git-dir", "--git-common-dir", "--show-prefix")).Output()
	hasTop := err == nil
	if err != nil {
		out, err = isolateGitCommand(exec.Command("git", "-C", key, "rev-parse", base, "--git-dir", "--git-common-dir", "--show-prefix")).Output()
	}
	if err != nil {
		return Layout{}, err
	}
	parts := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	for i := range parts {
		parts[i] = strings.TrimSuffix(parts[i], "\r")
	}
	switch {
	case hasTop && len(parts) == 4:
		return Layout{Toplevel: parts[0], HasToplevel: true, GitDir: parts[1], CommonDir: parts[2], Prefix: parts[3]}, nil
	case !hasTop && len(parts) == 3:
		return Layout{GitDir: parts[0], CommonDir: parts[1], Prefix: parts[2]}, nil
	}
	return Layout{}, errNoAnswer
}
