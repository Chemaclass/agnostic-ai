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
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err == nil {
		t.Error("expected AGENTS.md NOT to exist when -t claude")
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
		[]byte("version: 1\n"), 0o644))

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
