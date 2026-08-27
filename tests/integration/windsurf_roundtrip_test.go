package integration

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestWindsurfRoundTrip_SyncImportSyncIsByteEqual is the windsurf
// audit's byte-stability gate from #336 acceptance criterion C. Skills
// live under the shared .agents/skills/ tree (folder-per-skill), not
// .devin/rules/, since a flat file there never loads as a skill
// (docs.devin.ai/desktop/cascade/skills); the snapshot and wipe steps
// below cover both trees. Workflows excluded: the shared rules-dir
// importer reclassifies by filename prefix, so a workflow at
// .windsurf/workflows/<agent>.md would re-import as a rule and double
// the spec set (same carve-out as the cline audit). MCP servers land
// in .devin/mcp_config.json (#587): the fixture seeds one stdio and
// one remote server so the transport-vs-type rename `import windsurf`
// applies on the way in (see importWindsurfMCP) round-trips too.
func TestWindsurfRoundTrip_SyncImportSyncIsByteEqual(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	seedWindsurfRoundTripFixture(t, dir)

	runCmd(t, "sync", "-t", "windsurf")
	first := snapshotWindsurfEmit(t, dir)
	if len(first) == 0 {
		t.Fatalf("first sync produced no windsurf output")
	}
	if !anyPathUnder(first, ".agents/skills/") {
		t.Fatalf("first sync produced no windsurf skill folders: %v", sortedKeys(first))
	}
	mcpBody, ok := first[".devin/mcp_config.json"]
	if !ok {
		t.Fatalf("first sync produced no .devin/mcp_config.json: %v", sortedKeys(first))
	}
	if !strings.Contains(mcpBody, `"transport": "http"`) {
		t.Fatalf("first sync's mcp file missing transport: http:\n%s", mcpBody)
	}
	if _, ok := first["backend/.devin/rules/auth.md"]; !ok {
		t.Fatalf("first sync produced no scoped rule at backend/.devin/rules/auth.md: %v", sortedKeys(first))
	}
	if body := first[".devin/rules/globbed.md"]; !strings.Contains(body, "trigger: glob") {
		t.Fatalf("first sync's non-always-on rule missing trigger frontmatter:\n%s", body)
	}

	for _, sub := range []string{"agents", "skills", "rules", "mcps"} {
		if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", sub)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "import", "windsurf")

	// `backend` holds the scoped rules dir, so it is emit output too and
	// has to go before the second sync rebuilds from the imported specs.
	for _, sub := range []string{".devin", ".agents", "backend"} {
		if err := os.RemoveAll(filepath.Join(dir, sub)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "sync", "-t", "windsurf")
	second := snapshotWindsurfEmit(t, dir)

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

func seedWindsurfRoundTripFixture(t *testing.T, dir string) {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(`version: 1
sources:
  agents: .agnostic-ai/agents
  skills: .agnostic-ai/skills
  rules: .agnostic-ai/rules
targets:
  - windsurf
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
	// A scoped rule emits to backend/.devin/rules/ and a non-always-on
	// one carries `trigger` frontmatter (#628). Both are new discovery
	// and translation paths on the import side, so the round-trip has
	// to cover them.
	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/rules/backend"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/rules/backend/auth.md"),
		[]byte("---\nname: auth\n---\n\nauth body\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/rules/globbed.md"),
		[]byte("---\nname: globbed\ndescription: Test conventions\nglobs: '**/*.test.ts'\nalwaysApply: false\n---\n\nglobbed body\n"), 0o644))

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/mcps"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/mcps/fs.yaml"),
		[]byte("name: fs\ncommand: npx\nargs: [\"-y\", \"@modelcontextprotocol/server-filesystem\"]\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/mcps/remote.yaml"),
		[]byte("name: remote\ntype: http\nurl: https://mcp.example.test/mcp\n"), 0o644))
}

// snapshotWindsurfEmit reads every file under a .devin/ (rules, agents)
// or .agents/ (skill folders) directory anywhere in the tree and
// returns a relative-path -> bytes map. Scoped rules put a .devin/ dir
// in a project sub-directory (#628), so matching only at the root would
// miss them. The spec tree is skipped: it is input, not output.
func snapshotWindsurfEmit(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".agnostic-ai" {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		slash := "/" + filepath.ToSlash(rel)
		if !strings.Contains(slash, "/.devin/") && !strings.Contains(slash, "/.agents/") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[strings.TrimPrefix(slash, "/")] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}
