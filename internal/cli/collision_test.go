package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// codex + amp both default to a shared AGENTS.md entry-point. They no
// longer collide: sync owns the entry-point write and deduplicates so a
// single AGENTS.md is written with the canonical pointer body.
func TestSync_SharedEntryPoint_Deduped(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "codex,amp"})
	if err := root.Execute(); err != nil {
		t.Fatalf("codex+amp should not collide on shared AGENTS.md: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("expected AGENTS.md written once: %v", err)
	}
	if !strings.Contains(string(got), "AI Project Conventions") {
		t.Errorf("AGENTS.md should carry the canonical pointer body, got:\n%s", got)
	}
}

func TestSync_LegacyRulesFileCollision(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	// A codex-only fenced section makes the two merged AGENTS.md bodies
	// diverge. Byte-identical output at a shared path dedupes instead.
	if err := os.WriteFile(filepath.Join(dir, ".agnostic-ai", "rules", "r2.md"),
		[]byte("---\nname: r2\n---\nshared\n\n::target codex\ncodex only\n::end\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "agnostic-ai.yaml")
	if err := os.WriteFile(cfgPath, []byte(`version: 1
gitignore:
  enabled: true
outputs:
  codex:
    rules-file: AGENTS.md
  amp:
    rules-file: AGENTS.md
`), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "codex,amp"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected collision error when codex and amp both write legacy concat to AGENTS.md")
	}
	if !strings.Contains(err.Error(), "output collision") {
		t.Errorf("expected 'output collision' in error, got: %v", err)
	}
}

// Identical bytes at a shared path are the dedup sync exists for, not a
// conflict: codex + amp pointing rules-file at the same AGENTS.md render
// the same merged body when no per-target fences diverge, and the same
// applies to the shared .agents/skills tree both emit by default.
func TestSync_SharedPathIdenticalContent_NoCollision(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	cfgPath := filepath.Join(dir, "agnostic-ai.yaml")
	if err := os.WriteFile(cfgPath, []byte(`version: 1
gitignore:
  enabled: true
outputs:
  codex:
    rules-file: AGENTS.md
  amp:
    rules-file: AGENTS.md
`), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "codex,amp"})
	if err := root.Execute(); err != nil {
		t.Fatalf("identical shared-path content should dedupe, not collide: %v", err)
	}
}

func TestSync_NoCollisionForDisjointTargets(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude,codex"})
	if err := root.Execute(); err != nil {
		t.Errorf("claude+codex should not collide: %v", err)
	}
}

func TestSync_CollisionPolicyPreferSpec_SkipsCheck(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	cfgPath := filepath.Join(dir, "agnostic-ai.yaml")
	if err := os.WriteFile(cfgPath, []byte(`version: 1
gitignore:
  enabled: false
sync:
  collision-policy: prefer-spec
outputs:
  codex:
    rules-file: AGENTS.md
  amp:
    rules-file: AGENTS.md
`), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "codex,amp"})
	// prefer-spec skips collision pre-flight; sync should succeed.
	if err := root.Execute(); err != nil {
		t.Errorf("prefer-spec should skip collision check, got: %v", err)
	}
}

// claude and qoder both default to the project-root .mcp.json with the
// identical mcpServers schema (target-audit 2026-08-01: qoder MCP is a
// dedup of the literal path claude.go already writes, not a new file).
// Enabling both must write .mcp.json once, not raise an output
// collision.
func TestSync_ClaudeAndQoder_SharedMCPFile_Deduped(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	if err := os.MkdirAll(filepath.Join(dir, ".agnostic-ai", "mcps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agnostic-ai", "mcps", "fs.yaml"),
		[]byte("name: fs\ncommand: npx\nargs: [\"-y\", \"@modelcontextprotocol/server-filesystem\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude,qoder"})
	if err := root.Execute(); err != nil {
		t.Fatalf("claude+qoder should dedupe the shared .mcp.json, not collide: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatalf("expected .mcp.json written once: %v", err)
	}
	if !strings.Contains(string(got), `"fs"`) || !strings.Contains(string(got), `"command": "npx"`) {
		t.Errorf("expected the fs server in .mcp.json, got:\n%s", got)
	}
}

func TestSync_CollisionPolicyFail_HardError(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	// Diverge the merged bodies; identical bytes would dedupe, not fail.
	if err := os.WriteFile(filepath.Join(dir, ".agnostic-ai", "rules", "r2.md"),
		[]byte("---\nname: r2\n---\nshared\n\n::target codex\ncodex only\n::end\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "agnostic-ai.yaml")
	if err := os.WriteFile(cfgPath, []byte(`version: 1
gitignore:
  enabled: false
sync:
  collision-policy: fail
outputs:
  codex:
    rules-file: AGENTS.md
  amp:
    rules-file: AGENTS.md
`), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "codex,amp"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected collision error with policy=fail")
	}
	if !strings.Contains(err.Error(), "output collision") {
		t.Errorf("expected 'output collision' in error, got: %v", err)
	}
}
