package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestImportCrush_RoundTripFixedPoint emits a kit-sink bundle to crush,
// wipes the source specs, imports the emitted tree back, then re-emits.
// The second emit must byte-match the first: import reconstructs rules
// (from the inlined `## Rules` block in AGENTS.md), skills (from
// `.agents/skills/`), and MCP servers (from `crush.json`).
func TestImportCrush_RoundTripFixedPoint(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [crush]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r1.md"),
		"---\nname: r1\n---\n\nrule one body\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r2.md"),
		"---\nname: r2\n---\n\nrule two body\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "my-skill", "SKILL.md"),
		"---\nname: my-skill\ndescription: An example skill\n---\n\nSkill body here.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "stdio-server.yaml"),
		"name: stdio-server\ncommand: npx\nargs:\n  - -y\n  - \"@modelcontextprotocol/server-filesystem\"\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "http-server.yaml"),
		"name: http-server\ntype: http\nurl: https://example.test/mcp\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "sse-server.yaml"),
		"name: sse-server\ntype: sse\nurl: https://example.test/sse\n")

	execCLI(t, "sync", "-t", "crush")
	first := snapshotEmitted(t, dir)
	if _, ok := first["crush.json"]; !ok {
		t.Fatalf("first emit produced no crush.json: %v", keys(first))
	}
	if _, ok := first[".agents/skills/my-skill/SKILL.md"]; !ok {
		t.Fatalf("first emit produced no skill folder: %v", keys(first))
	}

	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
		t.Fatalf("wipe source specs: %v", err)
	}
	execCLI(t, "import", "crush")

	// Rules come from the inlined `## Rules` block in AGENTS.md.
	r1 := readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r1.md"))
	if !strings.Contains(r1, "name: r1") || !strings.Contains(r1, "rule one body") {
		t.Errorf("rule r1 not reconstructed from AGENTS.md:\n%s", r1)
	}
	r2 := readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r2.md"))
	if !strings.Contains(r2, "rule two body") {
		t.Errorf("rule r2 not reconstructed:\n%s", r2)
	}
	// Skill folder round-trips (with its description).
	skill := readFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "my-skill", "SKILL.md"))
	if !strings.Contains(skill, "description: An example skill") || !strings.Contains(skill, "Skill body here.") {
		t.Errorf("skill not reconstructed:\n%s", skill)
	}
	// MCP: crush.json carries an explicit `type` on all three transports.
	stdio := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "stdio-server.yaml"))
	if !strings.Contains(stdio, "type: stdio") || !strings.Contains(stdio, "command: npx") {
		t.Errorf("stdio mcp not reconstructed:\n%s", stdio)
	}
	httpMCP := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "http-server.yaml"))
	if !strings.Contains(httpMCP, "type: http") || !strings.Contains(httpMCP, "url: https://example.test/mcp") {
		t.Errorf("http mcp not reconstructed:\n%s", httpMCP)
	}
	// sse must round-trip as its own type, not collapse into http: crush
	// routes the two to different transports (see #586).
	sseMCP := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "sse-server.yaml"))
	if !strings.Contains(sseMCP, "type: sse") || !strings.Contains(sseMCP, "url: https://example.test/sse") {
		t.Errorf("sse mcp not reconstructed:\n%s", sseMCP)
	}

	execCLI(t, "sync", "-t", "crush")
	second := snapshotEmitted(t, dir)
	assertEmittedEqual(t, first, second)
}

// TestImportCrush_UnknownStaysRejected guards the wiring: crush is a
// known source, but a typo near it still fails fast in a multi-source
// import.
func TestImportCrush_KnownSourceWiredIn(t *testing.T) {
	if !isKnownImportSource("crush") {
		t.Error("crush should be a known import source")
	}
	if !isKnownImportSource("kiro") {
		t.Error("kiro should be a known import source")
	}
	sources := importSources()
	for _, want := range []string{"crush", "kiro"} {
		if !strings.Contains(sources, want) {
			t.Errorf("importSources() missing %q: %s", want, sources)
		}
	}
}
