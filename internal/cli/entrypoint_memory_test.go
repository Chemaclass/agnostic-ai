package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const memoryImport = "\n@.agnostic-ai/memory/MEMORY.md\n"

func TestWriteAgnosticEntryPoints_ClaudeImportsSharedMemory(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeAgnosticFile(t, "# Project\n")
	cfg := &config.Config{Targets: []string{"claude"}, Builtins: []string{"memory"}}

	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, spec.Bundle{}, cfg.Targets, false); err != nil {
		t.Fatal(err)
	}
	claude, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if !strings.Contains(string(claude), memoryImport) {
		t.Errorf("CLAUDE.md has no memory import:\n%s", claude)
	}
}

func TestWriteAgnosticEntryPoints_NoMemoryImportWithoutTheBuiltin(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeAgnosticFile(t, "# Project\n")
	cfg := &config.Config{Targets: []string{"claude"}}

	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, spec.Bundle{}, cfg.Targets, false); err != nil {
		t.Fatal(err)
	}
	claude, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if strings.Contains(string(claude), "agnostic-ai:memory") {
		t.Errorf("CLAUDE.md has a memory block without the built-in:\n%s", claude)
	}
}

// AGENTS.md readers such as Codex cannot follow `@` lines, so only the
// Claude Code companion carries the import, and it stays a companion.
func TestWriteAgnosticEntryPoints_MemoryImportKeepsTheClaudeCompanion(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeAgnosticFile(t, "# Project\n\nShared.\n")
	cfg := &config.Config{Targets: []string{"claude", "codex"}, Builtins: []string{"memory"}}

	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, spec.Bundle{}, cfg.Targets, false); err != nil {
		t.Fatal(err)
	}
	claude, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	got := header.Strip(string(claude))
	if !strings.HasPrefix(strings.TrimSpace(got), "@AGENTS.md") || !strings.Contains(got, memoryImport) || strings.Contains(got, "Shared.") {
		t.Errorf("CLAUDE.md should be the companion plus the memory import:\n%s", got)
	}
	agents, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if strings.Contains(string(agents), "agnostic-ai:memory") {
		t.Errorf("AGENTS.md carries the memory block:\n%s", agents)
	}
}
