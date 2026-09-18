package integration

import (
	"os"
	"path/filepath"
	"testing"
)

// The memory-curator skill lives twice: the repository's own spec under
// .agnostic-ai/skills/, which maintainers edit and sync, and the copy under
// internal/cli/initdata/, which `init --demo` embeds and ships to a new
// project. go:embed refuses a symlink, so nothing but this test keeps the
// shipped copy from drifting behind the one exercised here.
func TestMemoryCurator_SeededCopyMatchesRepoSpec(t *testing.T) {
	t.Parallel()
	read := func(rel string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("..", "..", rel))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	const (
		repoRel   = ".agnostic-ai/skills/memory-curator/SKILL.md"
		seededRel = "internal/cli/initdata/skills/memory-curator.md"
	)
	if repo, seeded := read(repoRel), read(seededRel); repo != seeded {
		t.Errorf("%s and %s have diverged; copy one over the other\n--- %s ---\n%s\n--- %s ---\n%s",
			repoRel, seededRel, repoRel, repo, seededRel, seeded)
	}
}
