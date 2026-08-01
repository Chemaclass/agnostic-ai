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

func setupWhyFixture(t *testing.T) string {
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
	mustWrite(filepath.Join(dir, "rules", "other.md"), `---
name: other
description: Sibling rule.
---

Sibling body.
`)
	return dir
}

func TestWhy_HumanOutputReportsAdapterAndSource(t *testing.T) {
	dir := setupWhyFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"why", ".cursor/rules/no-console-log.mdc"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, ".cursor/rules/no-console-log.mdc") {
		t.Errorf("missing file header: %s", got)
	}
	if !strings.Contains(got, "adapter: cursor") {
		t.Errorf("missing adapter line: %s", got)
	}
	if !strings.Contains(got, "no-console-log") {
		t.Errorf("missing source name: %s", got)
	}
	if !strings.Contains(got, "(rules/no-console-log.md)") {
		t.Errorf("missing source path: %s", got)
	}
}

func TestWhy_UnknownFileEmitsActionableError(t *testing.T) {
	dir := setupWhyFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	// Pre-create the sync state so the error path is "not tracked", not
	// "no sync state".
	if err := os.MkdirAll(filepath.Join(dir, ".agnostic-ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agnostic-ai", ".sync-state"),
		[]byte(`{"synced_at":"2026-01-01T00:00:00Z","files_changed":1}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"why", "some/random/path.md"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for untracked file")
	}
	if !strings.Contains(err.Error(), "not synced or not tracked") {
		t.Errorf("expected not-tracked message, got %v", err)
	}
}

func TestWhy_MissingStateFileSuggestsSync(t *testing.T) {
	dir := setupWhyFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	// No .sync-state present; an unknown path should surface the
	// "no sync state" hint.
	root := NewRootCmd("test")
	root.SetArgs([]string{"why", "some/random/path.md"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when sync state missing")
	}
	if !strings.Contains(err.Error(), "no sync state found") {
		t.Errorf("expected no-sync-state message, got %v", err)
	}
	if !strings.Contains(err.Error(), "agnostic-ai sync") {
		t.Errorf("expected sync hint in error, got %v", err)
	}
}

func TestWhy_JSONOutputSchema(t *testing.T) {
	dir := setupWhyFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	if err := os.MkdirAll(filepath.Join(dir, ".agnostic-ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agnostic-ai", ".sync-state"),
		[]byte(`{"synced_at":"2026-01-01T00:00:00Z","files_changed":1}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"why", ".cursor/rules/no-console-log.mdc", "--format", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got whyOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if got.Version != "1" {
		t.Errorf("version: want 1, got %q", got.Version)
	}
	if got.Command != "why" {
		t.Errorf("command: want why, got %q", got.Command)
	}
	if got.Target != "cursor" {
		t.Errorf("target: want cursor, got %q", got.Target)
	}
	if got.File == "" {
		t.Errorf("file: missing")
	}
	if len(got.Sources) == 0 {
		t.Errorf("expected at least one source")
	}
	if got.LastSync == nil {
		t.Errorf("expected last_sync present")
	}
	if got.OutputKeys == nil {
		t.Errorf("output_keys must be a JSON array, not null")
	}
}

func TestWhy_OutputKeysReportConfiguredOverrides(t *testing.T) {
	dir := setupWhyFixture(t)
	// Override the cursor rules-dir so the path uses a custom segment.
	cfg := `version: 1
sources:
  rules: rules
targets:
  - cursor
outputs:
  cursor:
    rules-dir: custom-rules
`
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	silence(t)

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"why", "custom-rules/no-console-log.mdc", "--format", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got whyOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	found := false
	for _, k := range got.OutputKeys {
		if k == "outputs.cursor.rules-dir" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected outputs.cursor.rules-dir in keys, got %v", got.OutputKeys)
	}
}

// why on an entry-point file (AGENTS.md) credits the rule specs inlined
// into it, even though no adapter's Emit writes that file.
func TestWhy_EntryPointFileCreditsInlinedRules(t *testing.T) {
	dir := setupWhyFixture(t)
	cfg := `version: 1
sources:
  rules: rules
targets:
  - codex
`
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	silence(t)

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"why", "AGENTS.md", "--format", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got whyOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if got.Target != "codex" {
		t.Errorf("expected codex as the entry-point consumer, got %q", got.Target)
	}
	names := map[string]bool{}
	for _, s := range got.Sources {
		names[s.Name] = true
		if s.Mode != "section" {
			t.Errorf("inlined rule must be a section source, got %q for %s", s.Mode, s.Name)
		}
	}
	for _, want := range []string{"no-console-log", "other"} {
		if !names[want] {
			t.Errorf("expected inlined rule %q in sources, got %v", want, got.Sources)
		}
	}
}

// TestWhy_RootAGENTSMdNotConfusedWithJunieMirror guards against a
// regression the junie adapter's own `.junie/AGENTS.md` entry-point
// mirror could reintroduce: findEmittingAdapter runs every registered
// adapter's capture output (regardless of the project's `targets:`
// list), and `.junie/AGENTS.md` shares a basename with the root
// `AGENTS.md` this test queries. The basename-only fallback must not
// let that unrelated nested file outrank the dedicated entry-point
// resolution for an exact top-level query.
func TestWhy_RootAGENTSMdNotConfusedWithJunieMirror(t *testing.T) {
	dir := setupWhyFixture(t)
	cfg := `version: 1
sources:
  rules: rules
targets:
  - codex
`
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	silence(t)

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"why", "AGENTS.md", "--format", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got whyOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if got.Target != "codex" {
		t.Errorf("root AGENTS.md must resolve to codex (the inlined-rules entry-point consumer), not junie's unrelated .junie/AGENTS.md mirror; got %q", got.Target)
	}
}

// TestWhy_JunieAGENTSMdMirrorResolvesToJunie confirms the exact-path
// query for junie's own entry-point mirror still resolves correctly:
// the fix for the collision above must not make `.junie/AGENTS.md`
// itself untraceable.
func TestWhy_JunieAGENTSMdMirrorResolvesToJunie(t *testing.T) {
	dir := setupWhyFixture(t)
	cfg := `version: 1
sources:
  rules: rules
targets:
  - junie
`
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	silence(t)

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"why", ".junie/AGENTS.md", "--format", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got whyOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if got.Target != "junie" {
		t.Errorf("expected junie as the .junie/AGENTS.md emitter, got %q", got.Target)
	}
}
