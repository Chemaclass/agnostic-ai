package integration

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestClineRoundTrip_SyncImportSyncIsByteEqual is the cline audit's
// byte-stability gate from #328 acceptance criterion C:
//
//	sync cline -> snapshot .clinerules/*
//	           -> wipe source specs
//	           -> import cline
//	           -> wipe emit
//	           -> sync cline
//	           -> assert byte-for-byte identical
//
// The fixture covers every cline-supported kind (agents, skills,
// rules) with three specimens each. The workflows-dir branch is
// intentionally left off: the importer reclassifies every .md it
// finds under .clinerules/ by filename prefix, so a workflow at
// .clinerules/workflows/<agent>.md would re-import as a rule named
// after the agent, doubling the spec set. Workflow round-trip needs
// a separate harness (importer would need to learn the workflows-
// dir layout).
func TestClineRoundTrip_SyncImportSyncIsByteEqual(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	seedClineRoundTripFixture(t, dir)

	runCmd(t, "sync", "-t", "cline")
	first := snapshotClineEmit(t, dir)
	if len(first) == 0 {
		t.Fatalf("first sync produced no cline output")
	}

	for _, sub := range []string{"agents", "skills", "rules"} {
		if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", sub)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "import", "cline")

	if err := os.RemoveAll(filepath.Join(dir, ".clinerules")); err != nil {
		t.Fatal(err)
	}

	runCmd(t, "sync", "-t", "cline")
	second := snapshotClineEmit(t, dir)

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

func seedClineRoundTripFixture(t *testing.T, dir string) {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(`version: 1
sources:
  agents: .agnostic-ai/agents
  skills: .agnostic-ai/skills
  rules: .agnostic-ai/rules
targets:
  - cline
gitignore:
  enabled: false
`), 0o644))

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/agents"), 0o755))
	for _, n := range []string{"alpha", "beta", "gamma"} {
		must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/agents", n+".md"),
			[]byte("---\nname: "+n+"\n---\n\n"+n+" body\n"), 0o644))
	}

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/skills"), 0o755))
	for _, n := range []string{"uno", "dos", "tres"} {
		must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/skills", n), 0o755))
		must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/skills", n, "SKILL.md"),
			[]byte("---\nname: "+n+"\n---\n\n"+n+" skill body\n"), 0o644))
	}

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/rules"), 0o755))
	for _, n := range []string{"r1", "r2", "r3"} {
		must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/rules", n+".md"),
			[]byte("---\nname: "+n+"\n---\n\n"+n+" body\n"), 0o644))
	}
}

func snapshotClineEmit(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	full := filepath.Join(root, ".clinerules")
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
