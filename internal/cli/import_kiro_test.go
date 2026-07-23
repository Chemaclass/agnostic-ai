package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// execCLI drives one agnostic-ai command in-process, failing the test on
// error. Output is expected to be silenced by the caller.
func execCLI(t *testing.T, args ...string) {
	t.Helper()
	root := NewRootCmd("test")
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
}

// snapshotEmitted reads every emitted file under root (skipping the
// `.agnostic-ai/` source tree and agnostic-ai.yaml) into a map keyed by
// slash-form relative path, so two syncs can be compared for a
// round-trip fixed point.
func snapshotEmitted(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
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
		rel = filepath.ToSlash(rel)
		if rel == "agnostic-ai.yaml" || strings.HasPrefix(rel, ".agnostic-ai/") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

// assertEmittedEqual fails the test when two emitted-tree snapshots
// differ, naming the first offending path.
func assertEmittedEqual(t *testing.T, want, got map[string]string) {
	t.Helper()
	for rel, wantBody := range want {
		gotBody, ok := got[rel]
		if !ok {
			t.Errorf("re-emit dropped %s", rel)
			continue
		}
		if gotBody != wantBody {
			t.Errorf("re-emit diverged at %s\n--- first ---\n%s\n--- second ---\n%s",
				rel, wantBody, gotBody)
		}
	}
	for rel := range got {
		if _, ok := want[rel]; !ok {
			t.Errorf("re-emit added %s", rel)
		}
	}
}

// TestImportKiro_RoundTripFixedPoint emits a kit-sink bundle to kiro,
// wipes the source specs, imports the emitted tree back, then re-emits.
// The second emit must byte-match the first: import reconstructs every
// spec the kiro adapter can carry.
func TestImportKiro_RoundTripFixedPoint(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [kiro]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r-go.md"),
		"---\nname: r-go\nglobs: \"**/*.go\"\n---\n\nGo rule body here.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r-always.md"),
		"---\nname: r-always\n---\n\nAlways rule body here.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "my-agent.md"),
		"---\nname: my-agent\n---\n\nAgent body here.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "my-skill", "SKILL.md"),
		"---\nname: my-skill\ndescription: An example skill\n---\n\nSkill body here.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "stdio-server.yaml"),
		"name: stdio-server\ncommand: npx\nargs:\n  - -y\n  - \"@modelcontextprotocol/server-filesystem\"\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "http-server.yaml"),
		"name: http-server\ntype: http\nurl: https://example.test/mcp\n")

	execCLI(t, "sync", "-t", "kiro")
	first := snapshotEmitted(t, dir)
	if _, ok := first[".kiro/steering/r-go.md"]; !ok {
		t.Fatalf("first emit produced no kiro steering files: %v", keys(first))
	}

	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
		t.Fatalf("wipe source specs: %v", err)
	}
	execCLI(t, "import", "kiro")

	// Rules: fileMatch -> globs, always -> unscoped.
	rGo := readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r-go.md"))
	if !strings.Contains(rGo, "globs:") || !strings.Contains(rGo, "**/*.go") {
		t.Errorf("r-go rule lost its glob on import:\n%s", rGo)
	}
	if !strings.Contains(rGo, "Go rule body here.") {
		t.Errorf("r-go rule lost its body:\n%s", rGo)
	}
	rAlways := readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r-always.md"))
	if strings.Contains(rAlways, "globs:") {
		t.Errorf("always rule should be unscoped (no globs):\n%s", rAlways)
	}
	// Agent and skill round-trip through steering-file prefixes.
	agent := readFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "my-agent.md"))
	if !strings.Contains(agent, "name: my-agent") || !strings.Contains(agent, "Agent body here.") {
		t.Errorf("agent not reconstructed:\n%s", agent)
	}
	skill := readFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "my-skill", "SKILL.md"))
	if !strings.Contains(skill, "description: An example skill") {
		t.Errorf("skill lost its description:\n%s", skill)
	}
	// MCP: mcpServers stdio (no type) + http (type http).
	stdio := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "stdio-server.yaml"))
	if !strings.Contains(stdio, "command: npx") {
		t.Errorf("stdio mcp not reconstructed:\n%s", stdio)
	}
	httpMCP := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "http-server.yaml"))
	if !strings.Contains(httpMCP, "type: http") || !strings.Contains(httpMCP, "url: https://example.test/mcp") {
		t.Errorf("http mcp not reconstructed:\n%s", httpMCP)
	}

	execCLI(t, "sync", "-t", "kiro")
	second := snapshotEmitted(t, dir)
	assertEmittedEqual(t, first, second)
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
