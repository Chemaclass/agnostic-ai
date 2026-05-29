package integration

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestClaudeRoundTrip_SyncImportSyncIsByteEqual is the claude
// audit's byte-stability gate from #327 acceptance criterion C:
//
//	sync claude -> snapshot .claude/* + .mcp.json
//	            -> wipe source specs (preserve overlay)
//	            -> import claude
//	            -> wipe emit
//	            -> sync claude
//	            -> assert byte-for-byte identical
//
// The fixture covers every claude-supported kind (agents, skills,
// commands, hooks across multiple events, MCPs across stdio/http/
// disabled). Rules flow through sync's project-root CLAUDE.md
// entry-point — same caveat the codex audit (#329) noted — so the
// snapshot excludes the root CLAUDE.md and the round-trip omits
// rules entirely; the entry-point round-trip needs its own harness.
func TestClaudeRoundTrip_SyncImportSyncIsByteEqual(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	seedClaudeRoundTripFixture(t, dir)

	runCmd(t, "sync", "-t", "claude")
	first := snapshotClaudeEmit(t, dir)
	if len(first) == 0 {
		t.Fatalf("first sync produced no claude output")
	}

	// Wipe the source specs so the importer is the only thing that
	// can rebuild them from the emitted .claude/* tree. Keep the
	// agnostic-ai/.sync-state ledger alone so the sweep behavior
	// stays consistent across the two syncs.
	for _, sub := range []string{"agents", "skills", "commands", "mcps", "hooks"} {
		if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", sub)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "import", "claude")

	// `import claude` slices the sync-owned CLAUDE.md by H2 into rule
	// specs. The original kit-sink intentionally has zero rules, so
	// strip the slicer output before the second sync to keep both
	// snapshots over the same kind set. Rules round-trip is covered
	// separately by the entry-point harness (see #329 follow-up).
	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai/rules")); err != nil {
		t.Fatal(err)
	}

	for _, sub := range []string{".claude", ".mcp.json"} {
		if err := os.RemoveAll(filepath.Join(dir, sub)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "sync", "-t", "claude")
	second := snapshotClaudeEmit(t, dir)

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

func seedClaudeRoundTripFixture(t *testing.T, dir string) {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(`version: 1
sources:
  agents: .agnostic-ai/agents
  skills: .agnostic-ai/skills
  rules: .agnostic-ai/rules
  hooks: .agnostic-ai/hooks
  mcps: .agnostic-ai/mcps
  commands: .agnostic-ai/commands
targets:
  - claude
gitignore:
  enabled: false
`), 0o644))

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/agents"), 0o755))
	for _, n := range []string{"alpha", "beta", "gamma"} {
		must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/agents", n+".md"),
			[]byte("---\nname: "+n+"\ndescription: agent "+n+"\n---\n\n"+n+" body\n"), 0o644))
	}

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/skills"), 0o755))
	for _, n := range []string{"uno", "dos", "tres"} {
		must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/skills", n), 0o755))
		must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/skills", n, "SKILL.md"),
			[]byte("---\nname: "+n+"\ndescription: skill "+n+"\n---\n\n"+n+" skill body\n"), 0o644))
	}

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/hooks"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/hooks/post-edit.yaml"),
		[]byte("name: post-edit\nevent: PostToolUse\nmatcher: Edit\ncommand: \"gofmt -w\"\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/hooks/pre-write.yaml"),
		[]byte("name: pre-write\nevent: PreToolUse\nmatcher: Write\ncommand: \"echo pre\"\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/hooks/session-start.yaml"),
		[]byte("name: session-start\nevent: SessionStart\ncommand: \"echo session\"\n"), 0o644))

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/commands"), 0o755))
	for _, n := range []string{"cmd-one", "cmd-two", "cmd-three"} {
		must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/commands", n+".md"),
			[]byte("---\ndescription: "+n+"\n---\n\n"+n+" body\n"), 0o644))
	}

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/mcps"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/mcps/stdio-server.yaml"),
		[]byte("name: stdio-server\ncommand: npx\nargs:\n  - \"-y\"\n  - \"@modelcontextprotocol/server-filesystem\"\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/mcps/http-server.yaml"),
		[]byte("name: http-server\ntype: http\nurl: https://example.test/mcp\n"), 0o644))
}

// snapshotClaudeEmit reads every file under .claude/ and .mcp.json.
// The project-root CLAUDE.md (owned by sync, not the claude adapter)
// is excluded so the snapshot reflects only the adapter's direct
// output.
func snapshotClaudeEmit(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, r := range []string{".claude", ".mcp.json"} {
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
