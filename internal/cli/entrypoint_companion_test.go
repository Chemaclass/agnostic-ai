package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A Claude-only project whose CLAUDE.md imports AGENTS.md keeps that
// layout: sync writes the shared body to AGENTS.md and CLAUDE.md stays
// `@AGENTS.md` plus its Claude-only text, so an edit to AGENTS.md still
// reaches Claude Code.
func TestSync_ClaudeOnlyKeepsTheAgentsCompanion(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeFile(t, "AGENTS.md", "# Project\n\nShared instructions.\n")
	writeFile(t, "CLAUDE.md", "@AGENTS.md\n\nClaude only.\n")
	for _, args := range [][]string{{"import", "claude"}, {"sync"}} {
		root := NewRootCmd("test")
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	claude := readFile(t, "CLAUDE.md")
	if !strings.Contains(claude, "@AGENTS.md") || !strings.Contains(claude, "Claude only.") || strings.Contains(claude, "Shared instructions.") {
		t.Errorf("CLAUDE.md should import AGENTS.md and keep its own text:\n%s", claude)
	}
	agents := readFile(t, filepath.Join("AGENTS.md"))
	if !strings.Contains(agents, "Shared instructions.") || strings.Contains(agents, "Claude only.") {
		t.Errorf("AGENTS.md should hold the shared body alone:\n%s", agents)
	}
}
