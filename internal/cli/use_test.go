package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func claudeOnlyProject(t *testing.T) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "CLAUDE.md", "# My project\n\n## Conventions\n\nUse pnpm. Never touch prod.\n")
	mustWriteFile(t, ".claude/skills/review/SKILL.md", "---\nname: review\ndescription: Review a PR.\n---\nReview carefully.\n")
}

// One command takes a Claude-only repository to Claude plus Codex from
// one source, keeping what it had (#1613).
func TestUse_StartsAProjectFromWhatTheRepositoryHas(t *testing.T) {
	claudeOnlyProject(t)
	log := captureLog(t)

	if out, err := runCLI(t, "use", "codex"); err != nil {
		t.Fatalf("use codex: %v\n%s", err, out)
	}

	if got := readFile(t, "AGENTS.md"); !strings.Contains(got, "Never touch prod.") {
		t.Errorf("AGENTS.md lacks the project's instructions:\n%s", got)
	}
	if _, err := os.Stat(".agents/skills/review/SKILL.md"); err != nil {
		t.Errorf("codex did not get the skill: %v", err)
	}
	if cfg := readFile(t, "agnostic-ai.yaml"); !strings.Contains(cfg, "claude") || !strings.Contains(cfg, "codex") {
		t.Errorf("targets miss claude or codex:\n%s", cfg)
	}
	for _, want := range []string{"codex now reads, from .agnostic-ai/:", "1 skill", "review"} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("summary lacks %q:\n%s", want, log.String())
		}
	}
}

func TestUse_AgainChangesNothing(t *testing.T) {
	claudeOnlyProject(t)
	if out, err := runCLI(t, "use", "codex"); err != nil {
		t.Fatalf("use codex: %v\n%s", err, out)
	}
	before := snapshotFiles(t, "AGENTS.md", "CLAUDE.md", "agnostic-ai.yaml")
	log := captureLog(t)

	if out, err := runCLI(t, "use", "codex"); err != nil {
		t.Fatalf("use codex again: %v\n%s", err, out)
	}

	if after := snapshotFiles(t, "AGENTS.md", "CLAUDE.md", "agnostic-ai.yaml"); after["AGENTS.md"] != before["AGENTS.md"] || after["agnostic-ai.yaml"] != before["agnostic-ai.yaml"] {
		t.Error("a second use changed the project")
	}
	if !strings.Contains(log.String(), "codex already in use") {
		t.Errorf("second use did not say so:\n%s", log.String())
	}
}

// In an existing project, use adds the tool and imports its own config
// before the sync would write over it.
func TestUse_AddsAToolToAnExistingProjectAndKeepsItsConfig(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", "# My project\n\nUse pnpm.\n")
	runSyncOK(t)
	mustWriteFile(t, "AGENTS.md", "# Agents\n\n## Reviews\n\nKeep PRs small.\n")

	if out, err := runCLI(t, "use", "codex"); err != nil {
		t.Fatalf("use codex: %v\n%s", err, out)
	}

	got := readFile(t, "AGENTS.md")
	for _, want := range []string{"Use pnpm.", "Keep PRs small."} {
		if !strings.Contains(got, want) {
			t.Errorf("AGENTS.md lacks %q:\n%s", want, got)
		}
	}
}

func TestUse_RejectsAMistypedTool(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	_, err := runCLI(t, "use", "codx")
	if err == nil || !strings.Contains(err.Error(), "did you mean codex?") {
		t.Errorf("err = %v, want a did-you-mean", err)
	}
	if _, err := os.Stat("agnostic-ai.yaml"); err == nil {
		t.Error("use wrote a config for a mistyped tool")
	}
}

// A run that stopped before importing finishes on the next try, though
// the tool is already in targets.
func TestUse_ARetryImportsWhatAnEarlierRunLeft(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	mustWriteFile(t, "AGENTS.md", "# Agents\n\n## Reviews\n\nKeep PRs small.\n")

	if out, err := runCLI(t, "use", "codex"); err != nil {
		t.Fatalf("use codex: %v\n%s", err, out)
	}

	if got := readFile(t, ".agnostic-ai/AGNOSTIC_AI.md"); !strings.Contains(got, "Keep PRs small.") {
		t.Errorf("use did not import the left-over AGENTS.md:\n%s", got)
	}
}

// jules has no importer; its AGENTS.md is folded in as root instructions.
func TestUse_AToolWithoutAnImporterKeepsItsInstructions(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "AGENTS.md", "# Agents\n\n## Reviews\n\nKeep PRs small.\n")

	if out, err := runCLI(t, "use", "jules"); err != nil {
		t.Fatalf("use jules: %v\n%s", err, out)
	}

	if got := readFile(t, "AGENTS.md"); !strings.Contains(got, "Keep PRs small.") {
		t.Errorf("AGENTS.md lost its instructions:\n%s", got)
	}
}
