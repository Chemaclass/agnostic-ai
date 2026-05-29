package integration

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestCodexRoundTrip_SyncImportSyncIsByteEqual is the codex audit's
// byte-stability gate from #329 acceptance criterion C:
//
//	sync codex -> snapshot .codex/* + legacy rules-file
//	             -> wipe source specs
//	             -> import codex
//	             -> sync codex
//	             -> assert byte-for-byte identical
//
// The fixture covers every codex-supported spec kind (agents, skills,
// rules via the legacy rules-file opt-in, hooks across multiple
// events, commands, MCPs across stdio + http + disabled) so a future
// regression in any byte-stable round-trip path (frontmatter key
// order, hook event order, agent TOML keys, MCP table layout, overlay
// capture) trips here instead of silently shipping.
func TestCodexRoundTrip_SyncImportSyncIsByteEqual(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	seedCodexRoundTripFixture(t, dir)

	runCmd(t, "sync", "-t", "codex")
	first := snapshotCodexEmit(t, dir)
	if len(first) == 0 {
		t.Fatalf("first sync produced no codex output")
	}

	// Wipe the source specs so the importer is the only thing that
	// can rebuild them from the emitted .codex/* tree.
	for _, sub := range []string{"agents", "skills", "hooks", "mcps", "commands"} {
		if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", sub)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "import", "codex")

	// Wipe the emitted tree too so the second sync writes from scratch
	// and the comparison reflects emit output, not stale bytes.
	if err := os.RemoveAll(filepath.Join(dir, ".codex")); err != nil {
		t.Fatal(err)
	}

	runCmd(t, "sync", "-t", "codex")
	second := snapshotCodexEmit(t, dir)

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

func seedCodexRoundTripFixture(t *testing.T, dir string) {
	t.Helper()
	// Rules and the legacy `outputs.codex.rules-file` are intentionally
	// excluded from this round-trip: rules in codex flow through the
	// project-root AGENTS.md entry-point (owned by `sync`, not the
	// adapter), so a round-trip of the rules text needs a separate
	// entry-point harness. See follow-up issue tracked under #329.
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(`version: 1
sources:
  agents: .agnostic-ai/agents
  skills: .agnostic-ai/skills
  hooks: .agnostic-ai/hooks
  mcps: .agnostic-ai/mcps
  commands: .agnostic-ai/commands
targets:
  - codex
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
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/mcps/disabled-server.yaml"),
		[]byte("name: disabled-server\ncommand: x\ndisabled: true\n"), 0o644))
}

// snapshotCodexEmit reads every file under .codex/ and returns a
// relative-path -> bytes map. The .agnostic-ai/ source tree and the
// project-root AGENTS.md (owned by sync, not the codex adapter) are
// excluded so the snapshot reflects only the adapter's direct output.
func snapshotCodexEmit(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	roots := []string{".codex"}
	for _, r := range roots {
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

// unifiedDiffLines returns a side-by-side per-line diff that high-
// lights every line that differs between a and b. Lines that match
// are collapsed to an `=` marker so the failure output stays small.
func unifiedDiffLines(a, b string) string {
	la := strings.Split(a, "\n")
	lb := strings.Split(b, "\n")
	n := len(la)
	if len(lb) > n {
		n = len(lb)
	}
	var sb strings.Builder
	for i := 0; i < n; i++ {
		var x, y string
		if i < len(la) {
			x = la[i]
		}
		if i < len(lb) {
			y = lb[i]
		}
		if x == y {
			continue
		}
		fmt.Fprintf(&sb, "  line %d:\n    -%q\n    +%q\n", i+1, x, y)
	}
	if sb.Len() == 0 {
		return "(no line-level diff; check trailing whitespace or final newline)"
	}
	return sb.String()
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
