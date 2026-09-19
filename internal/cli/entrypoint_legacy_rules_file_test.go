package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Claude Code v2.1.277 reads AGENTS.md as project instructions whenever
// no CLAUDE.md exists at or above the working directory. Pointing
// `outputs.claude.rules-file` at anything other than CLAUDE.md used to
// drop claude from entry-point distribution entirely, so a project with
// codex enabled shipped an AGENTS.md and no CLAUDE.md and Claude Code
// loaded codex's file, rule bodies routed away from claude included.
func TestSync_LegacyRulesFileOffEntryPoint_StillWritesCLAUDEmd(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	if err := os.WriteFile(filepath.Join(dir, ".agnostic-ai", "rules", "codex-only.md"),
		[]byte("---\nname: codex-only\ntarget: codex\n---\nSECRET-CODEX-ONLY\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(`version: 1
targets: [claude, codex]
outputs:
  claude:
    rules-file: .claude/RULES.md
`), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync: %v", err)
	}

	main, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("CLAUDE.md must exist so Claude Code never falls back to AGENTS.md: %v", err)
	}
	if !strings.Contains(string(main), "AI Project Conventions") {
		t.Errorf("CLAUDE.md should carry the canonical pointer body, got:\n%s", main)
	}
	if strings.Contains(string(main), "SECRET-CODEX-ONLY") {
		t.Errorf("a codex-routed rule must never reach CLAUDE.md, got:\n%s", main)
	}
	// The concatenated rules file is on no documented Claude Code load
	// path, so the pointer body wires it in with a single `@`-import.
	if !strings.Contains(string(main), "@.claude/RULES.md") {
		t.Errorf("CLAUDE.md should import the configured rules-file, got:\n%s", main)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "RULES.md")); err != nil {
		t.Fatalf("the adapter still owns the concatenated rules file: %v", err)
	}
}

// `outputs.claude.rules-file: CLAUDE.md` is the documented legacy form:
// the adapter owns that exact path, so sync must keep skipping the
// pointer-body write or the two writers collide on one file.
func TestSync_LegacyRulesFileOnEntryPoint_AdapterStillOwnsIt(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(`version: 1
targets: [claude]
outputs:
  claude:
    rules-file: CLAUDE.md
`), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if !strings.Contains(string(got), "rule body") {
		t.Errorf("CLAUDE.md should hold the concatenated rule bodies, got:\n%s", got)
	}
	if strings.Contains(string(got), "AI Project Conventions") {
		t.Errorf("the pointer body must not overwrite the adapter's write, got:\n%s", got)
	}
}

// A target that inlines rule bodies into its entry point (gemini) and
// also sets a rules-file gets the pointer body back, but not a second
// copy of every rule: the adapter owns rule delivery in that layout.
func TestSync_LegacyRulesFileOffEntryPoint_DoesNotInlineRulesTwice(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(`version: 1
targets: [gemini]
outputs:
  gemini:
    rules-file: GEMINI-rules.md
`), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "GEMINI.md"))
	if err != nil {
		t.Fatalf("GEMINI.md must exist: %v", err)
	}
	if !strings.Contains(string(got), "AI Project Conventions") {
		t.Errorf("GEMINI.md should carry the canonical pointer body, got:\n%s", got)
	}
	if strings.Contains(string(got), "rule body") {
		t.Errorf("rule bodies belong in GEMINI-rules.md only, got:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "GEMINI-rules.md")); err != nil {
		t.Fatalf("the adapter still owns the concatenated rules file: %v", err)
	}
}
