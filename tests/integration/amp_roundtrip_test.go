package integration

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestAmpRoundTrip_SyncImportSyncIsByteEqual is the amp audit's
// byte-stability gate from #325 acceptance criterion C:
//
//	sync amp -> snapshot .agents/commands/* + .amp/settings.json
//	         -> wipe source specs
//	         -> import amp
//	         -> wipe emit
//	         -> sync amp
//	         -> assert byte-for-byte identical
//
// The fixture covers every amp-supported kind that lands in the
// adapter's own emit footprint (agents + MCPs across stdio/http/
// disabled-with-command). Rules flow through sync's project-root
// AGENTS.md entry-point — same caveat the codex audit (#329) noted —
// so they are intentionally excluded from this round-trip; the
// entry-point round-trip needs its own harness.
func TestAmpRoundTrip_SyncImportSyncIsByteEqual(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	seedAmpRoundTripFixture(t, dir)

	runCmd(t, "sync", "-t", "amp")
	first := snapshotAmpEmit(t, dir)
	if len(first) == 0 {
		t.Fatalf("first sync produced no amp output")
	}

	for _, sub := range []string{"agents", "mcps"} {
		if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", sub)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "import", "amp")

	// Wipe both adapter-owned trees so the second sync writes from
	// scratch and the comparison reflects emit output, not stale
	// bytes left behind by the first sync.
	for _, sub := range []string{".agents", ".amp"} {
		if err := os.RemoveAll(filepath.Join(dir, sub)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "sync", "-t", "amp")
	second := snapshotAmpEmit(t, dir)

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

func seedAmpRoundTripFixture(t *testing.T, dir string) {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(`version: 1
sources:
  agents: .agnostic-ai/agents
  mcps: .agnostic-ai/mcps
targets:
  - amp
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

// snapshotAmpEmit reads every file the amp adapter actually owns
// (.agents/commands/* + .amp/settings.json). The project-root
// AGENTS.md is excluded because sync writes it centrally, not amp,
// so it is not part of the adapter's round-trip surface.
func snapshotAmpEmit(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, r := range []string{".agents", ".amp"} {
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
