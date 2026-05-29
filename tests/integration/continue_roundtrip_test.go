package integration

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestContinueRoundTrip_SyncImportSyncIsByteEqual is the continue
// audit's byte-stability gate from #330 acceptance criterion C:
//
//	sync continue -> snapshot .continue/rules/* + .continue/mcpServers/*
//	              -> wipe source specs
//	              -> import continue
//	              -> wipe emit
//	              -> sync continue
//	              -> assert byte-for-byte identical
//
// The fixture covers rules + agents + skills + MCPs (stdio + http)
// so the rules-dir importer and the MCP yaml branch added in #347 are
// both exercised.
func TestContinueRoundTrip_SyncImportSyncIsByteEqual(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	seedContinueRoundTripFixture(t, dir)

	runCmd(t, "sync", "-t", "continue")
	first := snapshotContinueEmit(t, dir)
	if len(first) == 0 {
		t.Fatalf("first sync produced no continue output")
	}

	for _, sub := range []string{"agents", "skills", "rules", "mcps"} {
		if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", sub)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "import", "continue")

	if err := os.RemoveAll(filepath.Join(dir, ".continue")); err != nil {
		t.Fatal(err)
	}

	runCmd(t, "sync", "-t", "continue")
	second := snapshotContinueEmit(t, dir)

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

func seedContinueRoundTripFixture(t *testing.T, dir string) {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(`version: 1
sources:
  agents: .agnostic-ai/agents
  skills: .agnostic-ai/skills
  rules: .agnostic-ai/rules
  mcps: .agnostic-ai/mcps
targets:
  - continue
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

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/mcps"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/mcps/stdio-one.yaml"),
		[]byte("name: stdio-one\ncommand: npx\nargs:\n  - \"-y\"\n  - \"@modelcontextprotocol/server-filesystem\"\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/mcps/stdio-two.yaml"),
		[]byte("name: stdio-two\ncommand: server\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/mcps/http-one.yaml"),
		[]byte("name: http-one\ntype: http\nurl: https://example.test/mcp\n"), 0o644))
}

func snapshotContinueEmit(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	full := filepath.Join(root, ".continue")
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
