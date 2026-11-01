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

func claudeOnlyCompanionProject(t *testing.T) {
	t.Helper()
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	testutil.Chdir(t, t.TempDir())
	silence(t)
	captureLogOut(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeFile(t, "AGENTS.md", "Hand written rules.\n")
	writeFile(t, "CLAUDE.md", "@AGENTS.md\n")
	for _, args := range [][]string{{"import", "claude"}, {"sync"}} {
		root := NewRootCmd("test")
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
}

func TestCheck_ClaudeOnlyCompanionAgentsMDIsNotALeftover(t *testing.T) {
	claudeOnlyCompanionProject(t)
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--check"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync --check after sync: %v", err)
	}
	reports, err := collectDriftWithEntryPointTargets(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range reports {
		if len(r.Leftover) > 0 {
			t.Errorf("%s reports leftovers %v", r.Target, r.Leftover)
		}
	}
}

func TestStatus_ClaudeOnlyCompanionShowsNoDrift(t *testing.T) {
	claudeOnlyCompanionProject(t)
	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	drift, _, err := captureAllAndDiff(cfg.Targets, cfg, b)
	if err != nil {
		t.Fatal(err)
	}
	if drift != 0 {
		t.Errorf("status counts %d drifted files, want 0", drift)
	}
}

func TestDoctorFix_KeepsClaudeOnlyCompanionAgentsMD(t *testing.T) {
	claudeOnlyCompanionProject(t)
	root := NewRootCmd("test")
	root.SetIn(strings.NewReader(""))
	root.SetArgs([]string{"doctor", "--fix"})
	_ = root.Execute()
	if got := readFile(t, "AGENTS.md"); !strings.Contains(got, "Hand written rules.") {
		t.Errorf("doctor --fix lost AGENTS.md text:\n%s", got)
	}
}

func TestCheck_AgentsMDIsLeftoverOnceCLAUDEmdStopsImportingIt(t *testing.T) {
	claudeOnlyCompanionProject(t)
	writeFile(t, "CLAUDE.md", "Full text, no import.\n")
	reports, err := collectDriftWithEntryPointTargets(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range reports {
		for _, p := range r.Leftover {
			if p == "AGENTS.md" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("AGENTS.md should be a leftover once CLAUDE.md stops importing it: %+v", reports)
	}
}
