package cli

import (
	"bytes"
	"encoding/json"
	"os"
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
	claudeOnlyCompanionProjectSync(t, "sync")
}

func claudeOnlyCompanionProjectSync(t *testing.T, syncArgs ...string) {
	t.Helper()
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	testutil.Chdir(t, t.TempDir())
	silence(t)
	captureLogOut(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeFile(t, "AGENTS.md", "Hand written rules.\n")
	writeFile(t, "CLAUDE.md", "@AGENTS.md\n")
	for _, args := range [][]string{{"import", "claude"}, syncArgs} {
		root := NewRootCmd("test")
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
}

func runCompanionCmd(t *testing.T, args ...string) string {
	t.Helper()
	root := NewRootCmd("test")
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return out.String()
}

func requireExists(t *testing.T, path string, want bool) {
	t.Helper()
	_, err := os.Stat(path)
	if got := err == nil; got != want {
		t.Errorf("%s exists = %v, want %v", path, got, want)
	}
}

func TestCleanup_ClaudeOnlyCompanionListsAgentsBak(t *testing.T) {
	claudeOnlyCompanionProjectSync(t, "sync", "--backup")
	logs := captureLogOut(t)
	out := runCompanionCmd(t, "cleanup", "--dry-run") + logs.String()
	if !strings.Contains(out, "AGENTS.md.bak") {
		t.Errorf("cleanup --dry-run should list AGENTS.md.bak:\n%s", out)
	}
	requireExists(t, "AGENTS.md.bak", true)
	requireExists(t, "CLAUDE.md.bak", true)
}

func TestCleanup_ClaudeOnlyCompanionRemovesAgentsBak(t *testing.T) {
	claudeOnlyCompanionProjectSync(t, "sync", "--backup")
	runCompanionCmd(t, "cleanup")
	requireExists(t, "AGENTS.md.bak", false)
	requireExists(t, "CLAUDE.md.bak", false)
	requireExists(t, "AGENTS.md", true)
}

func TestRevert_ClaudeOnlyCompanionRestoresAgents(t *testing.T) {
	claudeOnlyCompanionProjectSync(t, "sync", "--backup")
	wantAgents := readFile(t, "AGENTS.md.bak")
	wantClaude := readFile(t, "CLAUDE.md.bak")
	runCompanionCmd(t, "revert")
	if got := readFile(t, "AGENTS.md"); got != wantAgents {
		t.Errorf("AGENTS.md = %q, want %q", got, wantAgents)
	}
	if got := readFile(t, "CLAUDE.md"); got != wantClaude {
		t.Errorf("CLAUDE.md = %q, want %q", got, wantClaude)
	}
	requireExists(t, "AGENTS.md.bak", false)
	requireExists(t, "CLAUDE.md.bak", false)
}

func TestRevert_ClaudeOnlyCompanionJSONListsAgentsOnce(t *testing.T) {
	claudeOnlyCompanionProjectSync(t, "sync", "--backup")
	before := readFile(t, "AGENTS.md")
	out := runCompanionCmd(t, "revert", "--json", "--dry-run")
	var report struct {
		Writes []struct {
			Path   string `json:"path"`
			Action string `json:"action"`
		} `json:"writes"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	count := 0
	for _, w := range report.Writes {
		if w.Path == "AGENTS.md" {
			count++
			if w.Action != "restore" {
				t.Errorf("AGENTS.md action = %q, want restore", w.Action)
			}
		}
	}
	if count != 1 {
		t.Errorf("AGENTS.md appears %d times in writes, want 1:\n%s", count, out)
	}
	if got := readFile(t, "AGENTS.md"); got != before {
		t.Errorf("dry-run changed AGENTS.md")
	}
	requireExists(t, "AGENTS.md.bak", true)
}

func TestRevert_ClaudeOnlyCompanionKeepsHandWrittenAgentsWithoutBak(t *testing.T) {
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	testutil.Chdir(t, t.TempDir())
	silence(t)
	captureLogOut(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeFile(t, "AGENTS.md", "Hand written rules.\n")
	writeFile(t, "CLAUDE.md", "@AGENTS.md\n")
	runCompanionCmd(t, "import", "claude")
	runCompanionCmd(t, "revert")
	if got := readFile(t, "AGENTS.md"); got != "Hand written rules.\n" {
		t.Errorf("revert changed a hand-written AGENTS.md: %q", got)
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

func TestSync_UnmanagedAgentsMDKeepsCLAUDEmdAsTheImport(t *testing.T) {
	for _, targets := range []string{"[claude]", "[claude, codex]"} {
		t.Run(targets, func(t *testing.T) {
			t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
			testutil.Chdir(t, t.TempDir())
			silence(t)
			captureLogOut(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: "+targets+"\nsync:\n  unmanaged: [AGENTS.md]\n")
			writeFile(t, "AGENTS.md", "Hand written rules.\n")
			writeFile(t, "CLAUDE.md", "@AGENTS.md\n")
			for _, args := range [][]string{{"import", "claude"}, {"sync"}, {"sync", "--check"}} {
				root := NewRootCmd("test")
				root.SetArgs(args)
				if err := root.Execute(); err != nil {
					t.Fatalf("%v: %v", args, err)
				}
			}
			got := readFile(t, "CLAUDE.md")
			if !strings.Contains(got, "@AGENTS.md") {
				t.Errorf("CLAUDE.md should stay the import, got:\n%s", got)
			}
		})
	}
}
