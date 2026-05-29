package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/cli"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSync_EmitsAllTargets(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)

	root := cli.NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	expected := []string{
		".agnostic-ai/AGNOSTIC_AI.md",
		"CLAUDE.md",
		".claude/rules/sample-rule.md",
		"AGENTS.md",
		"GEMINI.md",
		"CONVENTIONS.md",
		".github/copilot-instructions.md",
		".cursor/rules/sample-rule.mdc",
		".clinerules/sample-rule.md",
		".windsurf/rules/sample-rule.md",
		".continue/rules/sample-rule.md",
		".claude/agents/sample-agent.md",
		".claude/settings.json",
	}
	for _, f := range expected {
		path := filepath.Join(dir, f)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("missing expected output %s: %v", f, err)
		}
	}
	// Every per-target entry-point shares the canonical pointer body
	// (byte-identical to AGNOSTIC_AI.md).
	body, err := os.ReadFile(filepath.Join(dir, ".agnostic-ai/AGNOSTIC_AI.md"))
	if err != nil {
		t.Fatalf("read AGNOSTIC_AI.md: %v", err)
	}
	for _, f := range []string{"CLAUDE.md", "AGENTS.md", "GEMINI.md", "CONVENTIONS.md", ".github/copilot-instructions.md"} {
		got, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if string(got) != string(body) {
			t.Errorf("%s body should match AGNOSTIC_AI.md byte-for-byte", f)
		}
	}
}

func TestSync_SingleTarget(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)

	root := cli.NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".claude/rules/sample-rule.md")); err != nil {
		t.Error("expected .claude/rules/sample-rule.md to exist")
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Errorf("expected CLAUDE.md entry-point when -t claude: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err == nil {
		t.Error("expected AGENTS.md NOT to exist when -t claude")
	}
}

func TestSync_AddsStateFileToGitignore(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)

	root := cli.NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	gitignorePath := filepath.Join(dir, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	if !strings.Contains(string(content), ".agnostic-ai/.sync-state") {
		t.Errorf("expected .agnostic-ai/.sync-state in .gitignore, got: %s", content)
	}
}

func TestSync_LedgerSweepsOrphanedAdapterOutput(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)

	// First sync establishes the baseline ledger: every adapter writes
	// for the single rule + agent spec the fixture defines.
	root := cli.NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	dropTarget := filepath.Join(dir, ".claude/agents/sample-agent.md")
	if _, err := os.Stat(dropTarget); err != nil {
		t.Fatalf("expected baseline output present: %v", err)
	}

	// Remove the source spec that produced the file. A naive sync would
	// stop writing the agent output but leave the prior copy on disk
	// indefinitely. The ledger sweep should catch and remove it.
	if err := os.Remove(filepath.Join(dir, "agents/sample-agent.md")); err != nil {
		t.Fatalf("remove spec: %v", err)
	}

	root = cli.NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if _, err := os.Stat(dropTarget); !os.IsNotExist(err) {
		t.Errorf("expected orphan adapter output removed, stat err=%v", err)
	}
}

func TestSync_LedgerLeavesUserAuthoredFilesAlone(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)

	root := cli.NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Replace a known-managed file with hand-authored content (no
	// agnostic provenance marker). Even though the path was in the
	// prior ledger and is no longer in the current write set after
	// removing the source spec, the ledger sweep must not delete it.
	managed := filepath.Join(dir, ".claude/agents/sample-agent.md")
	if err := os.WriteFile(managed, []byte("hand authored, keep me\n"), 0o644); err != nil {
		t.Fatalf("seed user file: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "agents/sample-agent.md")); err != nil {
		t.Fatalf("remove spec: %v", err)
	}

	root = cli.NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	got, err := os.ReadFile(managed)
	if err != nil {
		t.Fatalf("user file removed by sweep: %v", err)
	}
	if string(got) != "hand authored, keep me\n" {
		t.Errorf("user file modified: %q", got)
	}
}

func TestValidate_OK(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)

	root := cli.NewRootCmd("test")
	root.SetArgs([]string{"validate"})
	if err := root.Execute(); err != nil {
		t.Fatalf("validate failed: %v", err)
	}
}

func TestList_PrintsEntries(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)

	root := cli.NewRootCmd("test")
	root.SetArgs([]string{"list"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestInit_ScaffoldsLayout(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	root := cli.NewRootCmd("test")
	root.SetArgs([]string{"init"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	for _, d := range []string{"agents", "skills", "rules", "hooks", "mcps"} {
		if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", d)); err != nil {
			t.Errorf("expected dir .agnostic-ai/%s to exist", d)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "agnostic-ai.yaml")); err != nil {
		t.Error("expected agnostic-ai.yaml")
	}
}

// setupFixture creates a fresh project tree with one of each spec kind.
func setupFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte("version: 1\ngitignore:\n  enabled: true\n"), 0o644))

	must(t, os.MkdirAll(filepath.Join(dir, "agents"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "agents", "sample-agent.md"),
		[]byte(`---
name: sample-agent
description: A sample agent.
---

Sample agent body.
`), 0o644))

	must(t, os.MkdirAll(filepath.Join(dir, "rules"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "rules", "sample-rule.md"),
		[]byte(`---
name: sample-rule
description: A sample rule.
alwaysApply: true
---

Be terse. Avoid filler.
`), 0o644))

	must(t, os.MkdirAll(filepath.Join(dir, "hooks"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "hooks", "sample-hook.yaml"),
		[]byte(`name: sample-hook
description: Sample hook.
event: PostToolUse
matcher: "Edit"
command: "echo edited"
`), 0o644))

	return dir
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil && !strings.Contains(err.Error(), "no such") {
		t.Fatal(err)
	}
}
