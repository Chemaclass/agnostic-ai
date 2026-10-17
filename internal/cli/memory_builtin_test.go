package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func setupMemoryProject(t *testing.T, targets string) string {
	t.Helper()
	return setupMemoryConfig(t, "version: 1\ntargets: ["+targets+"]\nbuiltins: [memory]\n")
}

func setupMemoryConfig(t *testing.T, cfg string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	silence(t)
	return dir
}

// geminiSharesAgents points Gemini CLI at the AGENTS.md Codex reads.
const geminiSharesAgents = "version: 1\ntargets: [gemini, codex]\nbuiltins: [memory]\noutputs:\n  gemini:\n    file: AGENTS.md\n"

// The Gemini memory tiers rule is for Gemini CLI alone, so a file Codex
// also reads leaves it out with a note instead of failing sync (#2103).
func TestSync_LeavesTheGeminiMemoryRuleOutOfAFileCodexReads(t *testing.T) {
	setupMemoryConfig(t, geminiSharesAgents)
	log := captureLog(t)

	notes := syncNotes(t)

	agents := readFile(t, "AGENTS.md")
	if strings.Contains(agents, "gemini-memory-tiers") || !strings.Contains(agents, "builtin:shared-memory-policy") {
		t.Errorf("AGENTS.md should carry the shared memory rule alone:\n%s", agents)
	}
	want := "note: gemini: built-in rule gemini-memory-tiers is left out of AGENTS.md, which codex also reads"
	if !slices.ContainsFunc(notes, func(n string) bool { return strings.Contains(n, want) }) {
		t.Errorf("no note %q in:\n%s", want, strings.Join(notes, "\n"))
	}
	if strings.Contains(log.String(), "gemini-memory-tiers") {
		t.Errorf("sync lists the rule it left out as one Gemini CLI reads:\n%s", log.String())
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}
}

// Only a built-in, which the project cannot edit, is left out. A project
// rule that one reader of a shared file gets still stops sync.
func TestSync_StopsOnAProjectRuleOnlySomeReadersOfAFileGet(t *testing.T) {
	setupMemoryConfig(t, geminiSharesAgents)
	writeFile(t, filepath.Join(".agnostic-ai", "rules", "gemini-only.md"), "---\nname: gemini-only\nalwaysApply: true\ntargets: [gemini]\n---\n\nGemini only.\n")

	out, err := runCLI(t, "sync")

	if err == nil || !strings.Contains(err.Error(), "target-specific root rules differ between readers") {
		t.Errorf("sync = %v, want the shared-file error\n%s", err, out)
	}
}

func TestRender_ShowsTheGeminiMemoryRuleWhereSyncWritesIt(t *testing.T) {
	for _, tc := range []struct {
		name, cfg string
		inGemini  bool
	}{
		{"own file", "version: 1\ntargets: [gemini, codex]\nbuiltins: [memory]\n", true},
		{"shared file", geminiSharesAgents, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupMemoryConfig(t, tc.cfg)

			out, _ := runCLI(t, "render", "builtin:gemini-memory-tiers")

			if strings.Contains(out, "GEMINI.md") != tc.inGemini || strings.Contains(out, "AGENTS.md") {
				t.Errorf("render should show the rule in GEMINI.md only when Gemini CLI reads it alone:\n%s", out)
			}
		})
	}
}

func TestExplain_CreditsTheGeminiMemoryRuleOnlyWhereSyncWritesIt(t *testing.T) {
	for _, tc := range []struct {
		name, cfg string
		inGemini  bool
	}{
		{"own file", "version: 1\ntargets: [gemini, codex]\nbuiltins: [memory]\n", true},
		{"shared file", geminiSharesAgents, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupMemoryConfig(t, tc.cfg)

			out, _ := runCLI(t, "explain", "builtin:gemini-memory-tiers")

			if strings.Contains(out, "GEMINI.md") != tc.inGemini || strings.Contains(out, "AGENTS.md") {
				t.Errorf("explain should credit the rule to GEMINI.md only when Gemini CLI reads it alone:\n%s", out)
			}
		})
	}
}

func TestWhy_LeavesTheGeminiMemoryRuleOutOfAFileCodexReads(t *testing.T) {
	setupMemoryConfig(t, geminiSharesAgents)
	runSyncOK(t)

	got := runWhyJSON(t, "AGENTS.md")

	names := map[string]bool{}
	for _, s := range got.Sources {
		names[s.Name] = true
	}
	if names["gemini-memory-tiers"] || !names["shared-memory-policy"] {
		t.Errorf("why AGENTS.md should credit the shared memory rule alone: %+v", got.Sources)
	}
}

// sync --global writes one block that every tool reads, so a built-in
// rule for one tool stays out of it instead of stopping the sync.
func TestSyncGlobal_LeavesTheGeminiMemoryRuleOut(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [gemini]\nbuiltins: [memory]\n")

	if _, warnings, err := runGlobalAgentTest(); err != nil {
		t.Fatalf("sync --global: %v\n%s", err, warnings)
	}

	got := readText(t, filepath.Join(home, ".gemini", "GEMINI.md"))
	if strings.Contains(got, "gemini-memory-tiers") || !strings.Contains(got, "shared-memory-policy") {
		t.Errorf("~/.gemini/GEMINI.md should carry the shared memory rule alone:\n%s", got)
	}
	if out, err := runCLI(t, "validate", "--global"); err != nil {
		t.Errorf("validate --global: %v\n%s", err, out)
	}
}

func TestWhy_CreditsTheSharedMemoryImport(t *testing.T) {
	setupMemoryProject(t, "claude")

	got := runWhyJSON(t, "CLAUDE.md")
	if len(got.Sources) == 0 || got.Sources[0].Mode != "section" {
		t.Fatalf("AGNOSTIC_AI.md should be one section of CLAUDE.md: %+v", got.Sources)
	}
	for _, s := range got.Sources {
		if s.Name == "shared memory import" && s.Builtin != nil && s.Builtin.Name == "memory" {
			return
		}
	}
	t.Errorf("no memory built-in source: %+v", got.Sources)
}

func TestRender_ResolvesTheBuiltinSourcesSyncStamps(t *testing.T) {
	setupMemoryProject(t, "claude")

	for selector, want := range map[string]string{
		"builtin:shared-memory-policy": ".claude/rules/shared-memory-policy.md",
		"builtin:shared-memory":        ".claude/skills/shared-memory/SKILL.md",
	} {
		var out strings.Builder
		root := NewRootCmd("test")
		root.SetOut(&out)
		root.SetArgs([]string{"render", selector})
		if err := root.Execute(); err != nil {
			t.Errorf("render %s: %v", selector, err)
			continue
		}
		if !strings.Contains(out.String(), want) {
			t.Errorf("render %s did not render %s:\n%s", selector, want, out.String())
		}
	}
}

// Cursor gets the built-in rule as an .mdc file and, through Codex, in
// AGENTS.md. Both report one source, and neither marks it not emitted.
func TestExplainFile_ReportsABuiltinRuleUnderOneSource(t *testing.T) {
	setupMemoryProject(t, "cursor, codex")

	got := explainFileJSON(t, "main.go")
	sources := map[string]bool{}
	for _, it := range got.Instructions {
		if strings.Contains(it.Source, "shared-memory") || strings.Contains(it.Source, "builtins") {
			sources[it.Source] = true
			if it.Status == contextNotEmitted {
				t.Errorf("built-in rule reported not emitted: %+v", it)
			}
		}
	}
	if len(sources) != 1 || !sources["builtin:shared-memory-policy"] {
		t.Errorf("want only builtin:shared-memory-policy, got %v", sources)
	}
}
