package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func setupRenderFixture(t *testing.T) string {
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
  - cursor
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
	return dir
}

func TestRender_SingleTargetCursor(t *testing.T) {
	dir := setupRenderFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"render", "rules/no-console-log.md", "--target", "cursor"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "# target: cursor") {
		t.Errorf("expected target header, got:\n%s", got)
	}
	if !strings.Contains(got, ".cursor/rules/no-console-log.mdc") {
		t.Errorf("expected cursor output path, got:\n%s", got)
	}
	if !strings.Contains(got, "Do not commit console.log statements.") {
		t.Errorf("expected rule body, got:\n%s", got)
	}
}

func TestRender_MultiTarget(t *testing.T) {
	dir := setupRenderFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"render", "rules/no-console-log.md", "--target", "claude,cursor"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "CLAUDE.md") {
		t.Errorf("missing CLAUDE.md output: %s", got)
	}
	if !strings.Contains(got, ".cursor/rules/no-console-log.mdc") {
		t.Errorf("missing cursor output: %s", got)
	}
}

func TestRender_DefaultsToConfigTargets(t *testing.T) {
	dir := setupRenderFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"render", "rules/no-console-log.md"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "# target: claude") || !strings.Contains(got, "# target: cursor") {
		t.Errorf("expected both configured targets, got:\n%s", got)
	}
}

func TestRender_SpecNotFound(t *testing.T) {
	dir := setupRenderFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"render", "rules/missing.md"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

func TestRender_SpecOutsideSources(t *testing.T) {
	dir := setupRenderFixture(t)
	stray := filepath.Join(dir, "stray.md")
	if err := os.WriteFile(stray, []byte("---\nname: stray\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"render", "stray.md"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "not loaded as a spec") {
		t.Fatalf("expected stray-file error, got %v", err)
	}
}

func TestRender_DoesNotWriteToDisk(t *testing.T) {
	dir := setupRenderFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"render", "rules/no-console-log.md", "--target", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err == nil {
		t.Error("render must not write CLAUDE.md to disk")
	}
}
