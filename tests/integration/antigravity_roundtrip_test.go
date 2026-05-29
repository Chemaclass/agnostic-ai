package integration

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestAntigravityRoundTrip_SyncImportSyncIsByteEqual is the
// antigravity audit's byte-stability gate from #343:
//
//	sync antigravity -> snapshot .agent/* + AGENTS.md
//	                 -> wipe source specs
//	                 -> import antigravity
//	                 -> wipe emit
//	                 -> sync antigravity
//	                 -> assert byte-for-byte identical
//
// Fixture: 3 rules + 3 agents. Skills are excluded because the
// antigravity adapter does not declare KindSkill support.
func TestAntigravityRoundTrip_SyncImportSyncIsByteEqual(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	seedAntigravityRoundTripFixture(t, dir)

	runCmd(t, "sync", "-t", "antigravity")
	first := snapshotAntigravityEmit(t, dir)
	if len(first) == 0 {
		t.Fatalf("first sync produced no antigravity output")
	}

	for _, sub := range []string{"agents", "rules"} {
		if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", sub)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "import", "antigravity")

	if err := os.RemoveAll(filepath.Join(dir, ".agent")); err != nil {
		t.Fatal(err)
	}

	runCmd(t, "sync", "-t", "antigravity")
	second := snapshotAntigravityEmit(t, dir)

	firstPaths := sortedKeys(first)
	secondPaths := sortedKeys(second)
	if !equalStringSlice(firstPaths, secondPaths) {
		t.Fatalf("emit path set changed across round-trip\nfirst:  %v\nsecond: %v",
			firstPaths, secondPaths)
	}
	for _, p := range firstPaths {
		if first[p] != second[p] {
			t.Errorf("byte mismatch at %s (first=%d bytes, second=%d bytes)\n%s",
				p, len(first[p]), len(second[p]), unifiedDiffLines(first[p], second[p]))
		}
	}
}

func seedAntigravityRoundTripFixture(t *testing.T, dir string) {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(`version: 1
sources:
  agents: .agnostic-ai/agents
  rules: .agnostic-ai/rules
targets:
  - antigravity
gitignore:
  enabled: false
`), 0o644))

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/agents"), 0o755))
	for _, n := range []string{"alpha", "beta", "gamma"} {
		must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/agents", n+".md"),
			[]byte("---\nname: "+n+"\n---\n\n"+n+" body\n"), 0o644))
	}

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/rules"), 0o755))
	for _, n := range []string{"r1", "r2", "r3"} {
		must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/rules", n+".md"),
			[]byte("---\nname: "+n+"\n---\n\n"+n+" body\n"), 0o644))
	}
}

func snapshotAntigravityEmit(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	full := filepath.Join(root, ".agent")
	info, err := os.Stat(full)
	if os.IsNotExist(err) {
		return out
	}
	if err != nil {
		t.Fatalf("stat %s: %v", full, err)
	}
	if !info.IsDir() {
		return out
	}
	err = filepath.WalkDir(full, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", full, err)
	}
	return out
}
