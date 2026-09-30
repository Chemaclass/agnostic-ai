package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportFromClaude_NestedCLAUDEmdBecomesOneScopedRule(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := captureSummary(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWrite(t, filepath.Join(dir, "CLAUDE.md"), "# Root\n\nroot text\n")
	mustWrite(t, filepath.Join(dir, "src", "a", "CLAUDE.md"), "# Module A\n\nA facts.\n\n## Tests\n\nRun make test.\n")

	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatalf("import: %v", err)
	}

	want := "---\nname: a\nglobs: src/a/**\nscope: src/a\n---\n\n# Module A\n\nA facts.\n\n## Tests\n\nRun make test.\n"
	if got := mustRead(t, filepath.Join(dir, "rules", "a.md")); got != want {
		t.Errorf("rules/a.md = %q, want %q", got, want)
	}
	if entries := mustReadDir(t, filepath.Join(dir, "rules")); len(entries) != 1 {
		t.Errorf("want only rules/a.md, got %d files", len(entries))
	}
	if !strings.Contains(buf.String(), "delete src/a/CLAUDE.md with `agnostic-ai doctor --fix`") {
		t.Errorf("summary should tell the user to delete the nested file, got:\n%s", buf.String())
	}
}

func TestImportFromClaude_HandWrittenRootCLAUDEmdFeedsOnlyAgnosticMain(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	mustWrite(t, filepath.Join(dir, "CLAUDE.md"), "# Root\n\nroot text\n")

	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatalf("import: %v", err)
	}

	if got := mustRead(t, filepath.Join(dir, agnosticMainFile)); !strings.Contains(got, "root text") {
		t.Errorf("AGNOSTIC_AI.md should hold the root text, got %q", got)
	}
	if entries := mustReadDir(t, filepath.Join(dir, "rules")); len(entries) != 0 {
		t.Errorf("the root CLAUDE.md must not also become a rule, got %d files", len(entries))
	}
}

// A nested CLAUDE.md that only imports its AGENTS.md is how Claude Code
// reads that file, so the scope's rule is the AGENTS.md text, and sync
// then replaces the companion (#1336).
func TestImportFromClaude_NestedCompanionImportsItsAgentsMD(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	mustWrite(t, filepath.Join(dir, "services", "api", "AGENTS.md"), "# API\n\nUse integer minor units.\n")
	mustWrite(t, filepath.Join(dir, "services", "api", "CLAUDE.md"), "# CLAUDE.md\n\n@AGENTS.md\n")

	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatalf("import: %v", err)
	}

	want := "---\nname: api\nglobs: services/api/**\nscope: services/api\n---\n\n# API\n\nUse integer minor units.\n"
	if got := mustRead(t, filepath.Join(dir, "rules", "api.md")); got != want {
		t.Errorf("rules/api.md = %q, want %q", got, want)
	}
}

func TestImportFromClaude_NestedCompanionFencesItsClaudeText(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	mustWrite(t, filepath.Join(dir, "services", "api", "AGENTS.md"), "# API\n\nUse integer minor units.\n")
	mustWrite(t, filepath.Join(dir, "services", "api", "CLAUDE.md"), "@AGENTS.md\n\nUse the API launch config.\n")

	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatalf("import: %v", err)
	}

	want := "---\nname: api\nglobs: services/api/**\nscope: services/api\n---\n\n# API\n\nUse integer minor units.\n\n::target claude\nUse the API launch config.\n::end\n"
	if got := mustRead(t, filepath.Join(dir, "rules", "api.md")); got != want {
		t.Errorf("rules/api.md = %q, want %q", got, want)
	}
}

// When codex imports in the same run, it owns the nested AGENTS.md.
func TestImportFromClaude_NestedCompanionLeavesAgentsMDToCodex(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	mustWrite(t, filepath.Join(dir, "services", "api", "AGENTS.md"), "# API\n\nUse integer minor units.\n")
	mustWrite(t, filepath.Join(dir, "services", "api", "CLAUDE.md"), "@AGENTS.md\n")
	mustWrite(t, filepath.Join(dir, "services", "web", "AGENTS.md"), "# Web\n\nUse server components.\n")
	mustWrite(t, filepath.Join(dir, "services", "web", "CLAUDE.md"), "@AGENTS.md\n\nUse the web launch config.\n")
	setImportRunSources([]string{"claude", "codex"})
	t.Cleanup(func() { setImportRunSources(nil) })

	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatalf("import: %v", err)
	}

	entries := mustReadDir(t, filepath.Join(dir, "rules"))
	if len(entries) != 1 || entries[0].Name() != "web-claude.md" {
		t.Fatalf("want only rules/web-claude.md, got %v", entries)
	}
	want := "---\nname: web-claude\nglobs: services/web/**\nscope: services/web\n---\n\n::target claude\nUse the web launch config.\n::end\n"
	if got := mustRead(t, filepath.Join(dir, "rules", "web-claude.md")); got != want {
		t.Errorf("rules/web-claude.md = %q, want %q", got, want)
	}
}

// .claude/rules/ is the single source for rules once it exists: sync
// writes each nested CLAUDE.md there, and reading both would duplicate.
func TestImportFromClaude_ClaudeRulesDirOutranksNestedCLAUDEmd(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	mustWrite(t, filepath.Join(dir, ".claude", "rules", "src", "a", "a.md"), "---\nname: a\npaths:\n  - src/a/**\n---\n\nA facts.\n")
	mustWrite(t, filepath.Join(dir, "src", "a", "CLAUDE.md"), "A facts.\n")

	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatalf("import: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "rules", "a.md")); !os.IsNotExist(err) {
		t.Errorf("nested CLAUDE.md should not import next to .claude/rules/, stat err = %v", err)
	}
}
