package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestAiderRoundTrip_SyncImportSyncIsByteEqual is the aider audit's
// byte-stability gate from #340: the legacy concatenated rules-file
// layout has to survive sync -> import -> sync without nesting the
// `## Rules` wrapper deeper on every cycle. The fixture is rules-only
// because aider's MergedDocument output stores skill bodies as `Source:`
// pointers, which the importer cannot reconstitute; the agents branch
// is exercised in the unit-level codex/aider importer tests.
func TestAiderRoundTrip_SyncImportSyncIsByteEqual(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	seedAiderRoundTripFixture(t, dir)

	runCmd(t, "sync", "-t", "aider")
	first := snapshotAiderEmit(t, dir)
	if len(first) == 0 {
		t.Fatalf("first sync produced no aider output")
	}

	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", "rules")); err != nil {
		t.Fatal(err)
	}

	runCmd(t, "import", "aider")

	if err := os.Remove(filepath.Join(dir, "CONVENTIONS.md")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	runCmd(t, "sync", "-t", "aider")
	second := snapshotAiderEmit(t, dir)

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

func seedAiderRoundTripFixture(t *testing.T, dir string) {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(`version: 1
sources:
  rules: .agnostic-ai/rules
targets:
  - aider
outputs:
  aider:
    rules-file: CONVENTIONS.md
gitignore:
  enabled: false
`), 0o644))

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/rules"), 0o755))
	for _, n := range []string{"r1", "r2", "r3"} {
		must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/rules", n+".md"),
			[]byte("---\nname: "+n+"\n---\n\n"+n+" body\n"), 0o644))
	}
}

func snapshotAiderEmit(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	path := filepath.Join(root, "CONVENTIONS.md")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return out
	}
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	out["CONVENTIONS.md"] = string(data)
	return out
}
