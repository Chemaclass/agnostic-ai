package cli

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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

	// A selected target renders once before the emit for collisions and
	// shared skills. After the emit every target renders once for the
	// ignore lists, twice for gitignore.commit (no specs, then skills
	// only), and once with the team's built-ins.
	wantCaptures := map[string]int{"claude": 5, "codex": 5, "opencode": 4}
	wantWrites := map[string]int{"claude": 1, "codex": 1}
	if !maps.Equal(counts.captures, wantCaptures) || !maps.Equal(counts.writes, wantWrites) {
		t.Errorf("captures %v and writes %v, want %v and %v", counts.captures, counts.writes, wantCaptures, wantWrites)
	}
}

// An external adapter prints its warnings on every render, so each
// render sync no longer repeats is one warning fewer.
func TestSync_PrintsAnExternalAdapterWarningOncePerRender(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture requires a POSIX shell")
	}
	for _, tc := range []struct {
		args []string
		want int
	}{
		{[]string{"sync"}, 2},
		{[]string{"sync", "--check"}, 1},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			silence(t)
			chdir(t, t.TempDir())
			t.Setenv("HOME", t.TempDir())
			warningAdapterOnPath(t, "acme")
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, acme]\nsync:\n  shared-skills: true\n")
			mustWriteFile(t, ".agnostic-ai/skills/s1/SKILL.md", "---\nname: s1\ndescription: One skill.\n---\nskill body\n")
			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			var warned bytes.Buffer
			adapters.SetWarner(&warned)
			t.Cleanup(func() { adapters.SetWarner(os.Stderr) })

			if out, err := runCLI(t, tc.args...); err != nil {
				t.Fatalf("%v: %v\n%s", tc.args, err, out)
			}

			if got := strings.Count(warned.String(), "! acme: heads up\n"); got != tc.want {
				t.Errorf("%v printed the warning %d times, want %d:\n%s", tc.args, got, tc.want, warned.String())
			}
		})
	}
}

func TestTargetRenders_RendersAgainForAConfigThatDiffersBeyondTargets(t *testing.T) {
	counts := countRenders(t)
	adapter, err := adapters.Resolve("claude")
	if err != nil {
		t.Fatal(err)
	}
	b := spec.Bundle{Rules: []spec.Entry{{Kind: spec.KindRule, Name: "r1", Body: "rule body"}}}
	run := &config.Config{Version: 1, Targets: []string{"claude"}}
	renders := newTargetRenders(run, b)
	first := renders.capture(adapter, run)

	renders.capture(adapter, run.WithAdditionalTargets("codex"))
	if counts.captures["claude"] != 2 {
		t.Errorf("a config with one more target rendered %d times in all, want 2 (one per target list)", counts.captures["claude"])
	}
	renders.capture(adapter, run)
	if counts.captures["claude"] != 2 {
		t.Errorf("the run's config rendered again: %d renders, want 2", counts.captures["claude"])
	}
	moved := *run
	moved.Outputs = map[string]config.Output{"claude": {RulesDir: ".claude/moved"}}
	got := renders.capture(adapter, &moved)

	if counts.captures["claude"] != 3 {
		t.Errorf("a config with other outputs rendered %d times in all, want 3", counts.captures["claude"])
	}
	if slices.Equal(capturedPaths(got.files), capturedPaths(first.files)) {
		t.Errorf("a config with other outputs got the run's files: %v", capturedPaths(got.files))
	}
	if again := renders.capture(adapter, run); !slices.Equal(capturedPaths(again.files), capturedPaths(first.files)) {
		t.Errorf("the run's render changed after another config rendered: %v", capturedPaths(again.files))
	}
}

func capturedPaths(files []adapters.CapturedFile) []string {
	var out []string
	for _, f := range files {
		out = append(out, filepath.ToSlash(f.Path))
	}
	return out
}
