package integration

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestCursorRoundTrip_SyncImportSyncIsByteEqual is the cursor
// audit's byte-stability gate from #332 acceptance criterion C.
// Scope: rules only. The cursor importer reads `.cursor/rules/*.mdc`
// without filename-prefix classification (agents/skills get flattened
// into rules) and ignores `.cursor/mcp.json`. Filed as follow-up
// outside of this audit.
func TestCursorRoundTrip_SyncImportSyncIsByteEqual(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	seedCursorRoundTripFixture(t, dir)

	runCmd(t, "sync", "-t", "cursor")
	first := snapshotCursorEmit(t, dir)
	if len(first) == 0 {
		t.Fatalf("first sync produced no cursor output")
	}

	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai/rules")); err != nil {
		t.Fatal(err)
	}

	runCmd(t, "import", "cursor")

	if err := os.RemoveAll(filepath.Join(dir, ".cursor")); err != nil {
		t.Fatal(err)
	}

	runCmd(t, "sync", "-t", "cursor")
	second := snapshotCursorEmit(t, dir)

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

func seedCursorRoundTripFixture(t *testing.T, dir string) {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(`version: 1
sources:
  rules: .agnostic-ai/rules
targets:
  - cursor
gitignore:
  enabled: false
`), 0o644))

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/rules"), 0o755))
	for _, n := range []string{"r1", "r2", "r3"} {
		must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/rules", n+".md"),
			[]byte("---\nname: "+n+"\n---\n\n"+n+" body\n"), 0o644))
	}
}

func snapshotCursorEmit(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	full := filepath.Join(root, ".cursor")
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
