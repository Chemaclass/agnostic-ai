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

const (
	memoryImport         = "\n@.agnostic-ai/memory/MEMORY.md\n"
	personalMemoryImport = "\n@.agnostic-ai/local/memory/MEMORY.md\n"
)

// Claude Code's auto memory loads the personal index from the folder
// sync names in autoMemoryDirectory, so CLAUDE.md imports the project
// index alone.
func TestWriteAgnosticEntryPoints_ClaudeImportsTheProjectMemoryAlone(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeAgnosticFile(t, "# Project\n")
	cfg := &config.Config{Targets: []string{"claude"}, Builtins: []string{"memory"}}

	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, spec.Bundle{}, cfg.Targets, false); err != nil {
		t.Fatal(err)
	}
	claude, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if !strings.Contains(string(claude), memoryImport) || strings.Contains(string(claude), "local/memory") {
		t.Errorf("CLAUDE.md should import the project memory alone:\n%s", claude)
	}
}

func TestWriteAgnosticEntryPoints_ClaudeImportsPersonalMemoryWhenAutoMemoryMissesIt(t *testing.T) {
	for name, setup := range map[string]func(t *testing.T){
		"off in the environment": func(t *testing.T) { t.Setenv("CLAUDE_CODE_DISABLE_AUTO_MEMORY", "1") },
		"off in local settings": func(t *testing.T) {
			writeFile(t, filepath.Join(".claude", "settings.local.json"), `{"autoMemoryEnabled": false}`)
		},
		"off in user settings": func(t *testing.T) {
			user := t.TempDir()
			t.Setenv("CLAUDE_CONFIG_DIR", user)
			writeFile(t, filepath.Join(user, "settings.json"), `{"autoMemoryEnabled": false}`)
		},
		"a folder the user set elsewhere": func(t *testing.T) {
			writeFile(t, filepath.Join(".claude", "settings.local.json"), `{"autoMemoryDirectory": "~/notes"}`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			writeAgnosticFile(t, "# Project\n")
			setup(t)
			cfg := &config.Config{Targets: []string{"claude"}, Builtins: []string{"memory"}}

			if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, spec.Bundle{}, cfg.Targets, false); err != nil {
				t.Fatal(err)
			}
			claude, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
			if !strings.Contains(string(claude), memoryImport) || !strings.Contains(string(claude), personalMemoryImport) {
				t.Errorf("CLAUDE.md should import both memory indexes:\n%s", claude)
			}
		})
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
// Claude Code companion carries the import, and the block alone does not
// turn the companion into a full copy. (Any always-on rule inlined into
// AGENTS.md does, the memory rule included.)
func TestWriteAgnosticEntryPoints_MemoryBlockAloneKeepsTheClaudeCompanion(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeAgnosticFile(t, "# Project\n\nShared.\n")
	cfg := &config.Config{Targets: []string{"claude", "codex"}, Builtins: []string{"memory"}}

	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, spec.Bundle{}, cfg.Targets, false); err != nil {
		t.Fatal(err)
	}
	claude, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	got := header.Strip(string(claude))
	if !strings.HasPrefix(strings.TrimSpace(got), "@AGENTS.md") || !strings.Contains(got, memoryImport) || strings.Contains(got, personalMemoryImport) || strings.Contains(got, "Shared.") {
		t.Errorf("CLAUDE.md should be the companion plus the project memory import:\n%s", got)
	}
	agents, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if strings.Contains(string(agents), "agnostic-ai:memory") {
		t.Errorf("AGENTS.md carries the memory block:\n%s", agents)
	}
}

// A project whose CLAUDE.md already imports AGENTS.md keeps that layout
// with the memory rule loaded, and the import lands in the companion.
func TestWriteAgnosticEntryPoints_ClaudeWritesAgentsWithTheMemoryRule(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeAgnosticFile(t, "# Project\n\nShared.\n")
	for name, body := range map[string]string{"CLAUDE.md": "@AGENTS.md\n", "AGENTS.md": "# Project\n\nShared.\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rule := spec.Entry{Kind: spec.KindRule, Name: "shared-memory-policy", Layer: "builtin", Body: "Save facts.", Meta: map[string]any{"alwaysApply": true}}
	cfg := &config.Config{Targets: []string{"claude"}, Builtins: []string{"memory"}}

	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, spec.Bundle{Rules: []spec.Entry{rule}}, cfg.Targets, false); err != nil {
		t.Fatal(err)
	}
	claude, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	got := header.Strip(string(claude))
	if !strings.HasPrefix(strings.TrimSpace(got), "@AGENTS.md") || !strings.Contains(got, memoryImport) {
		t.Errorf("CLAUDE.md should stay the companion and import memory:\n%s", got)
	}
	agents, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if !strings.Contains(string(agents), "Shared.") || strings.Contains(string(agents), "agnostic-ai:memory") {
		t.Errorf("AGENTS.md = %q", agents)
	}
}
