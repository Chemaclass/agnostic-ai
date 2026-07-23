package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// writeParallelFixture writes a project that fans out to every registered
// target and carries a spread of spec kinds — including kinds several
// targets do not emit (settings, environments) so capability warnings and
// coverage notes buffer during emission. Their flushed ordering is the
// output most exposed to goroutine scheduling, so it belongs in every
// determinism assertion. extraConfig is appended verbatim under the config
// root (e.g. per-target provenance opt-outs).
func writeParallelFixture(t *testing.T, dir, extraConfig string) {
	t.Helper()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	writeSpec := func(rel, content string) {
		p := filepath.Join(dir, ".agnostic-ai", rel)
		must(os.MkdirAll(filepath.Dir(p), 0o755))
		must(os.WriteFile(p, []byte(content), 0o644))
	}

	names := adapters.Names()
	sort.Strings(names)
	var cfg strings.Builder
	cfg.WriteString("version: 1\n")
	cfg.WriteString("sync:\n  dropped-summary: true\n  collision-policy: prefer-spec\n")
	cfg.WriteString("targets:\n")
	for _, n := range names {
		cfg.WriteString("  - " + n + "\n")
	}
	cfg.WriteString(extraConfig)
	must(os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfg.String()), 0o644))

	for i := 0; i < 6; i++ {
		writeSpec(fmt.Sprintf("rules/rule-%d.md", i),
			fmt.Sprintf("---\nname: rule-%d\ndescription: Rule %d.\nglobs: \"**/*.go\"\n---\nRule %d body. Keep it deterministic.\n", i, i, i))
		writeSpec(fmt.Sprintf("agents/agent-%d.md", i),
			fmt.Sprintf("---\nname: agent-%d\ndescription: Agent %d.\n---\nAgent %d body. Review then report.\n", i, i, i))
		writeSpec(fmt.Sprintf("skills/skill-%d/SKILL.md", i),
			fmt.Sprintf("---\nname: skill-%d\ndescription: Skill %d.\n---\n# skill-%d\n\nDeterministic skill body.\n", i, i, i))
	}
	// Kinds that not every target supports, so warnings and notes buffer.
	writeSpec("settings/defaults.yaml", "model: claude-opus-4-8\ntemperature: 0.2\n")
	writeSpec("environments/dev.yaml", "name: dev\nvalues:\n  FOO: bar\n")
	writeSpec("commands/deploy.md", "---\nname: deploy\ndescription: Deploy command.\n---\nRun the deploy.\n")
}

var durationToken = regexp.MustCompile(`\d+(\.\d+)?(µs|ms|s)\b`)

// syncOutputAt runs a full `sync --jobs jobs` in a fresh copy of the
// fixture and returns the normalized human output (summary sink + flushed
// capability warnings) and the emitted file tree. The elapsed-time token
// in the summary is normalized so only order-sensitive content is
// compared. extraConfig lets a caller vary per-target settings.
func syncOutputAt(t *testing.T, jobs int, extraConfig string) (string, map[string]string) {
	t.Helper()
	dir := t.TempDir()
	writeParallelFixture(t, dir, extraConfig)
	testutil.Chdir(t, dir)

	logBuf := &strings.Builder{}
	warnBuf := &strings.Builder{}
	prevOut, prevV := logOut, verbosity
	logOut = logBuf
	verbosity = levelDefault
	adapters.SetWarner(warnBuf)
	defer func() {
		logOut, verbosity = prevOut, prevV
		adapters.SetWarner(os.Stderr)
	}()

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--gitignore", "on", "--jobs", strconv.Itoa(jobs)})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	if err := root.Execute(); err != nil {
		t.Fatalf("sync --jobs %d: %v", jobs, err)
	}
	out := durationToken.ReplaceAllString(logBuf.String()+"\n"+warnBuf.String(), "DUR")
	return out, snapshotTree(t, dir)
}

// snapshotTree returns every emitted file under dir keyed by its path
// relative to dir, skipping the sync-state cache (it carries a wall-clock
// timestamp that legitimately differs run to run).
func snapshotTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	stateRel := filepath.Join(".agnostic-ai", ".sync-state")
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == stateRel {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		tree[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return tree
}

// assertSameTree fails with the first divergent path when two emitted
// trees are not byte-identical.
func assertSameTree(t *testing.T, label string, a, b map[string]string) {
	t.Helper()
	for rel, ca := range a {
		cb, ok := b[rel]
		if !ok {
			t.Errorf("%s: path %q present in serial tree, missing in parallel tree", label, rel)
			continue
		}
		if ca != cb {
			t.Errorf("%s: content differs for %q\n--- serial ---\n%s\n--- parallel ---\n%s", label, rel, ca, cb)
		}
	}
	for rel := range b {
		if _, ok := a[rel]; !ok {
			t.Errorf("%s: path %q present in parallel tree, missing in serial tree", label, rel)
		}
	}
}

// TestSync_ParallelEmission_DeterministicTreeAndSummary pins the core
// guarantee of --jobs: the emitted file tree, the summary, and the flushed
// capability warnings are identical whether one worker or eight ran the
// per-target emits. Run under -race it also exercises the concurrent emit
// path across every target on its own session.
func TestSync_ParallelEmission_DeterministicTreeAndSummary(t *testing.T) {
	out1, tree1 := syncOutputAt(t, 1, "")
	out8, tree8 := syncOutputAt(t, 8, "")

	if out1 != out8 {
		t.Errorf("human output differs between --jobs 1 and --jobs 8:\n=== jobs=1 ===\n%s\n=== jobs=8 ===\n%s", out1, out8)
	}
	assertSameTree(t, "jobs1-vs-jobs8", tree1, tree8)
}

// TestSync_ParallelEmission_MixedProvenance covers the one emit-time global
// adapters still share: the provenance-header toggle. Half the targets opt
// out, so concurrent emission must batch by setting or a target's header
// bleeds into another's files. The parallel tree must still match serial.
func TestSync_ParallelEmission_MixedProvenance(t *testing.T) {
	names := adapters.Names()
	sort.Strings(names)
	var extra strings.Builder
	extra.WriteString("outputs:\n")
	for i, n := range names {
		if i%2 == 0 {
			extra.WriteString("  " + n + ":\n    provenance-header: false\n")
		}
	}
	cfg := extra.String()

	_, tree1 := syncOutputAt(t, 1, cfg)
	_, tree8 := syncOutputAt(t, 8, cfg)
	assertSameTree(t, "mixed-provenance", tree1, tree8)
}

// TestSync_ParallelEmission_DeterministicJSON pins the JSON result: the
// per-target writes / skipped / errors must appear in the same stable
// order regardless of --jobs.
func TestSync_ParallelEmission_DeterministicJSON(t *testing.T) {
	runJSON := func(jobs int) string {
		dir := t.TempDir()
		writeParallelFixture(t, dir, "")
		testutil.Chdir(t, dir)
		adapters.SetWarner(io.Discard)
		defer adapters.SetWarner(os.Stderr)

		buf := &strings.Builder{}
		root := NewRootCmd("test")
		root.SetArgs([]string{"sync", "--json", "--gitignore", "on", "--jobs", strconv.Itoa(jobs)})
		root.SetOut(buf)
		root.SetErr(io.Discard)
		if err := root.Execute(); err != nil {
			t.Fatalf("sync --json --jobs %d: %v", jobs, err)
		}
		return buf.String()
	}

	if got1, got8 := runJSON(1), runJSON(8); got1 != got8 {
		t.Errorf("JSON output differs between --jobs 1 and --jobs 8:\n=== jobs=1 ===\n%s\n=== jobs=8 ===\n%s", got1, got8)
	}
}

// TestSync_ParallelEmission_MoreWorkersThanTargets stresses the worker
// bound above the target count and confirms the run still succeeds and
// stays byte-identical to serial. Meaningful under -race.
func TestSync_ParallelEmission_MoreWorkersThanTargets(t *testing.T) {
	_, treeSerial := syncOutputAt(t, 1, "")
	_, treeMany := syncOutputAt(t, 64, "")
	assertSameTree(t, "jobs1-vs-jobs64", treeSerial, treeMany)
}

// TestResolveJobs checks the flag-to-worker mapping in isolation.
func TestResolveJobs(t *testing.T) {
	if got := resolveJobs(0, 8); got < 1 {
		t.Errorf("resolveJobs(0, 8) = %d, want >= 1 (NumCPU)", got)
	}
	if got := resolveJobs(1, 8); got != 1 {
		t.Errorf("resolveJobs(1, 8) = %d, want 1 (serial)", got)
	}
	if got := resolveJobs(16, 4); got != 4 {
		t.Errorf("resolveJobs(16, 4) = %d, want 4 (capped at target count)", got)
	}
	if got := resolveJobs(-3, 8); got < 1 {
		t.Errorf("resolveJobs(-3, 8) = %d, want >= 1", got)
	}
	if got := resolveJobs(0, 0); got != 1 {
		t.Errorf("resolveJobs(0, 0) = %d, want 1 (no targets floors at 1)", got)
	}
}
