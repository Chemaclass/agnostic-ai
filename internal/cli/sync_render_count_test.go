package cli

import (
	"maps"
	"strings"
	"sync"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// renderCounts counts, per target, the renders into a capture session
// (captures) and the emits that write or preview files (writes).
type renderCounts struct {
	mu       sync.Mutex
	captures map[string]int
	writes   map[string]int
}

func countRenders(t *testing.T) *renderCounts {
	t.Helper()
	counts := &renderCounts{captures: map[string]int{}, writes: map[string]int{}}
	original := emitAdapter
	emitAdapter = func(sess *adapters.Session, a adapters.Adapter, b spec.Bundle, cfg *config.Config, dryRun bool) error {
		counts.mu.Lock()
		if sess.IsCapturing() {
			counts.captures[a.Name()]++
		} else {
			counts.writes[a.Name()]++
		}
		counts.mu.Unlock()
		return original(sess, a, b, cfg, dryRun)
	}
	t.Cleanup(func() { emitAdapter = original })
	return counts
}

// expect checks that every target rendered into a capture session
// captures times and wrote or previewed its files writes times.
func (c *renderCounts) expect(t *testing.T, captures, writes int) {
	t.Helper()
	if len(c.captures) == 0 {
		t.Fatal("no target rendered")
	}
	for target, n := range c.captures {
		if n != captures {
			t.Errorf("%s rendered %d times into a capture session, want %d", target, n, captures)
		}
		if got := c.writes[target]; got != writes {
			t.Errorf("%s wrote its files %d times, want %d", target, got, writes)
		}
	}
}

func TestSync_RendersEachTargetOnce(t *testing.T) {
	for _, tc := range []struct {
		args   []string
		writes int
	}{
		{[]string{"sync"}, 1},
		{[]string{"sync", "--json"}, 1},
		{[]string{"sync", "--dry-run"}, 1},
		{[]string{"sync", "--plan"}, 0},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			syncedFixture(t)
			counts := countRenders(t)

			if out, err := runCLI(t, tc.args...); err != nil {
				t.Fatalf("%v: %v\n%s", tc.args, err, out)
			}

			counts.expect(t, 1, tc.writes)
		})
	}
}

func TestSyncCheck_RendersEachTargetOnce(t *testing.T) {
	syncedFixture(t)
	counts := countRenders(t)

	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Fatalf("sync --check: %v\n%s", err, out)
	}

	counts.expect(t, 1, 0)
}

// gitignoreFixture is a synced project whose sync runs every gitignore
// planner: a commit kind, shared skills, personal built-ins, and a
// configured target the run leaves out.
func gitignoreFixture(t *testing.T) {
	t.Helper()
	silence(t)
	chdir(t, t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex, opencode]\nbuiltins: []\ngitignore:\n  enabled: true\n  commit: [skills]\nsync:\n  shared-skills: true\n")
	mustWriteFile(t, "agnostic-ai.local.yaml", "builtins: [handoff]\n")
	mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", "Project instructions.\n")
	mustWriteFile(t, ".agnostic-ai/rules/r1.md", "---\nname: r1\n---\nrule body\n")
	mustWriteFile(t, ".agnostic-ai/skills/s1/SKILL.md", "---\nname: s1\ndescription: One skill.\n---\nskill body\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
}

func TestSync_RendersEachTargetOnceWithGitignorePlanners(t *testing.T) {
	gitignoreFixture(t)
	counts := countRenders(t)

	if out, err := runCLI(t, "sync", "-t", "claude", "-t", "codex"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}

	// One render of the project's specs, two for gitignore.commit (no
	// specs, then skills only), and one of the team's built-ins.
	wantCaptures := map[string]int{"claude": 4, "codex": 4, "opencode": 4}
	wantWrites := map[string]int{"claude": 1, "codex": 1}
	if !maps.Equal(counts.captures, wantCaptures) || !maps.Equal(counts.writes, wantWrites) {
		t.Errorf("captures %v and writes %v, want %v and %v", counts.captures, counts.writes, wantCaptures, wantWrites)
	}
}
