package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Antigravity's vendor-documented subagent path is `.agents/agents/`,
// the same directory codex swept wholesale as its own pre-v0.26 legacy
// agents tree. Both files carry the provenance header, so the sweep
// deleted antigravity's output and whichever adapter ran last decided
// whether the project had subagents at all (#638).
//
// Codex agents are TOML at both its native and its legacy path, so the
// sweep is narrowed to that extension rather than moving antigravity
// off the path its vendor documents.
func TestSync_CodexLegacySweepKeepsAntigravityAgents(t *testing.T) {
	dir := testutil.TempCwd(t)

	writeFile := func(rel, content string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeFile("agnostic-ai.yaml", "version: 1\ntargets: [antigravity, codex]\n")
	writeFile(".agnostic-ai/agents/reviewer.md",
		"---\nname: reviewer\ndescription: Reviews code changes.\n---\nReview the diff.\n")

	silence(t)
	if err := runSync(t); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".agents/agents/reviewer.md")); err != nil {
		t.Errorf("antigravity subagent deleted by codex's legacy sweep: %v", err)
	}
}
