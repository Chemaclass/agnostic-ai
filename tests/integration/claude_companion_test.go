package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A CLAUDE.md that imports the AGENTS.md beside it is how Claude Code
// reads AGENTS.md. Import keeps its own text for Claude only, and sync
// replaces the nested companion instead of failing (#1336).
func TestImportAll_ClaudeCompanionOfAgentsMD(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	files := map[string]string{
		"agnostic-ai.yaml":       "version: 1\ntargets: [claude, codex, cursor]\n",
		"AGENTS.md":              "# Project\n\nRoot guidance.\n",
		"CLAUDE.md":              "# CLAUDE.md\n\n@AGENTS.md\n\n## Claude Code specifics\n\n- Use the Dashboard launch config.\n",
		".codex/config.toml":     "",
		"services/api/AGENTS.md": "# API\n\nUse integer minor units.\n",
		"services/api/CLAUDE.md": "@AGENTS.md\n",
	}
	for name, body := range files {
		must(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755))
		must(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}

	runCmd(t, "import", "all")

	assertContains(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"),
		"Root guidance.", "::target claude\n## Claude Code specifics\n\n- Use the Dashboard launch config.\n::end")
	assertNoFileContains(t, filepath.Join(dir, ".agnostic-ai", "rules"), "@AGENTS.md")
	assertNoFileContains(t, filepath.Join(dir, ".agnostic-ai", "rules"), "Dashboard")

	runCmd(t, "sync")
	runCmd(t, "sync", "--check")

	// With another tool writing AGENTS.md, CLAUDE.md imports it and adds
	// only the Claude Code block, the form the project wrote by hand.
	assertContains(t, filepath.Join(dir, "CLAUDE.md"), "@AGENTS.md", "Use the Dashboard launch config.")
	assertContains(t, filepath.Join(dir, "AGENTS.md"), "Root guidance.")
	assertNoFileContains(t, filepath.Join(dir, "CLAUDE.md"), "Root guidance.")
	assertContains(t, filepath.Join(dir, ".claude", "rules", "services", "api", "api.md"), "Use integer minor units.")
	if _, err := os.Stat(filepath.Join(dir, "services", "api", "CLAUDE.md")); !os.IsNotExist(err) {
		t.Errorf("services/api/CLAUDE.md should be replaced by .claude/rules output, stat err = %v", err)
	}
	for _, out := range []string{"AGENTS.md", "services", ".codex", ".cursor"} {
		assertNoFileContains(t, filepath.Join(dir, out), "@AGENTS.md")
		assertNoFileContains(t, filepath.Join(dir, out), "Dashboard")
	}
}

// Each nested CLAUDE.md becomes one scoped rule, and a nested companion
// reads as its AGENTS.md, so sync writes both to .claude/rules/ and
// replaces the companion (#1427).
func TestImportClaude_NestedCLAUDEmdBecomesScopedRules(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	files := map[string]string{
		"agnostic-ai.yaml":       "version: 1\ntargets: [claude]\n",
		"CLAUDE.md":              "# Root\n\nroot text\n",
		"src/a/CLAUDE.md":        "# Module A\n\nA facts.\n",
		"services/api/AGENTS.md": "# API\n\nUse integer minor units.\n",
		"services/api/CLAUDE.md": "@AGENTS.md\n",
	}
	for name, body := range files {
		must(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755))
		must(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}

	runCmd(t, "import", "claude")

	assertContains(t, filepath.Join(dir, ".agnostic-ai", "rules", "a.md"), "scope: src/a", "A facts.")
	assertContains(t, filepath.Join(dir, ".agnostic-ai", "rules", "api.md"), "scope: services/api", "Use integer minor units.")
	assertNoFileContains(t, filepath.Join(dir, ".agnostic-ai", "rules"), "root text")

	runCmd(t, "sync")
	runCmd(t, "sync", "--check")

	assertContains(t, filepath.Join(dir, ".claude", "rules", "src", "a", "a.md"), "A facts.")
	assertContains(t, filepath.Join(dir, ".claude", "rules", "services", "api", "api.md"), "Use integer minor units.")
	if _, err := os.Stat(filepath.Join(dir, "services", "api", "CLAUDE.md")); !os.IsNotExist(err) {
		t.Errorf("services/api/CLAUDE.md should be replaced by .claude/rules output, stat err = %v", err)
	}
}
