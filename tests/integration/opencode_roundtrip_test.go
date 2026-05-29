package integration

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestOpencodeRoundTrip_SyncImportSyncIsByteEqual is the opencode
// audit's byte-stability gate from #334 acceptance criterion C.
// Scope: agents + MCPs. Rules round-trip via the sync-owned
// .opencode/AGENTS.md entry-point (same carve-out as codex/claude).
func TestOpencodeRoundTrip_SyncImportSyncIsByteEqual(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	seedOpencodeRoundTripFixture(t, dir)

	runCmd(t, "sync", "-t", "opencode")
	first := snapshotOpencodeEmit(t, dir)
	if len(first) == 0 {
		t.Fatalf("first sync produced no opencode output")
	}

	for _, sub := range []string{"agents", "mcps"} {
		if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", sub)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "import", "opencode")

	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai/rules")); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{".opencode", "opencode.json"} {
		if err := os.RemoveAll(filepath.Join(dir, p)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "sync", "-t", "opencode")
	second := snapshotOpencodeEmit(t, dir)

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

func seedOpencodeRoundTripFixture(t *testing.T, dir string) {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(`version: 1
sources:
  agents: .agnostic-ai/agents
  mcps: .agnostic-ai/mcps
targets:
  - opencode
gitignore:
  enabled: false
`), 0o644))

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/agents"), 0o755))
	for _, n := range []string{"alpha", "beta", "gamma"} {
		must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/agents", n+".md"),
			[]byte("---\nname: "+n+"\ndescription: agent "+n+"\n---\n\n"+n+" body\n"), 0o644))
	}

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/mcps"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/mcps/stdio-server.yaml"),
		[]byte("name: stdio-server\ncommand: npx\nargs:\n  - \"-y\"\n  - \"@modelcontextprotocol/server-filesystem\"\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/mcps/http-server.yaml"),
		[]byte("name: http-server\ntype: http\nurl: https://example.test/mcp\n"), 0o644))
}

func snapshotOpencodeEmit(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, r := range []string{".opencode/commands", "opencode.json"} {
		full := filepath.Join(root, r)
		info, err := os.Stat(full)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("stat %s: %v", full, err)
		}
		if !info.IsDir() {
			data, err := os.ReadFile(full)
			if err != nil {
				t.Fatalf("read %s: %v", full, err)
			}
			out[r] = string(data)
			continue
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
	}
	return out
}
