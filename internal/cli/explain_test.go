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

func setupExplainFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := `version: 1
sources:
  agents: agents
  skills: skills
  rules: rules
  hooks: hooks
  mcps: mcps
targets:
  - claude
  - codex
`
	mustWrite := func(p, body string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(filepath.Join(dir, "agnostic-ai.yaml"), cfg)
	mustWrite(filepath.Join(dir, "rules", "no-console-log.md"), `---
name: no-console-log
description: No console.log in shipped code.
globs: "**/*"
alwaysApply: true
---

Do not commit console.log statements.
`)
	// Add a sibling rule so contributions are clearly section-level
	// rather than full-file in shared docs like AGENTS.md.
	mustWrite(filepath.Join(dir, "rules", "other.md"), `---
name: other
description: Sibling rule.
---

Sibling body.
`)
	return dir
}

func TestExplain_HumanOutputListsConfiguredAndExtras(t *testing.T) {
	dir := setupExplainFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"explain", "rules/no-console-log.md"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "rules/no-console-log.md →") {
		t.Errorf("missing header arrow: %s", got)
	}
	if !strings.Contains(got, "[claude] CLAUDE.md") {
		t.Errorf("expected configured claude contribution: %s", got)
	}
	if !strings.Contains(got, "[codex] AGENTS.md") {
		t.Errorf("expected configured codex contribution: %s", got)
	}
	if !strings.Contains(got, "would emit if enabled:") {
		t.Errorf("expected extras header for non-configured targets: %s", got)
	}
	if !strings.Contains(got, "[cursor]") {
		t.Errorf("expected cursor in would-emit-if-enabled: %s", got)
	}
}

func TestExplain_FullFileForUniqueTargets(t *testing.T) {
	dir := setupExplainFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"explain", "rules/no-console-log.md"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	// Cursor writes one .mdc file per rule; removing the spec drops that
	// file entirely, so the contribution should be classified as "full".
	if !strings.Contains(got, ".cursor/rules/no-console-log.mdc (full file)") {
		t.Errorf("expected full-file classification for cursor mdc: %s", got)
	}
}

func TestExplain_JSONOutputSchema(t *testing.T) {
	dir := setupExplainFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"explain", "rules/no-console-log.md", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var got explainOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if got.Version != "1" {
		t.Errorf("version: want 1, got %q", got.Version)
	}
	if got.Command != "explain" {
		t.Errorf("command: want explain, got %q", got.Command)
	}
	if got.Spec.Kind != "rule" || got.Spec.Name != "no-console-log" {
		t.Errorf("spec ref wrong: %+v", got.Spec)
	}
	if len(got.Contributions) == 0 {
		t.Errorf("expected at least one configured contribution")
	}
	if len(got.WouldEmitIfEnabled) == 0 {
		t.Errorf("expected at least one would-emit-if-enabled entry")
	}
	for _, c := range got.Contributions {
		if c.Mode != "full" && c.Mode != "section" {
			t.Errorf("unexpected mode %q in %+v", c.Mode, c)
		}
	}
}

func TestExplain_DoesNotWriteToDisk(t *testing.T) {
	dir := setupExplainFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"explain", "rules/no-console-log.md"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err == nil {
		t.Error("explain must not write CLAUDE.md to disk")
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err == nil {
		t.Error("explain must not write AGENTS.md to disk")
	}
}

func TestExplain_SpecNotFound(t *testing.T) {
	dir := setupExplainFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"explain", "rules/missing.md"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

func TestExplain_AgentSpec(t *testing.T) {
	dir := setupExplainFixture(t)
	if err := os.MkdirAll(filepath.Join(dir, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	agentBody := `---
name: code-reviewer
description: Reviews diffs.
tools: [Read, Grep]
---
You review code.
`
	if err := os.WriteFile(filepath.Join(dir, "agents", "code-reviewer.md"), []byte(agentBody), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	silence(t)

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"explain", "agents/code-reviewer.md"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, ".claude/agents/code-reviewer.md (full file)") {
		t.Errorf("expected claude per-agent file as full contribution: %s", got)
	}
}
