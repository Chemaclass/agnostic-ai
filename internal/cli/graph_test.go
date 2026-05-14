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

func setupGraphFixture(t *testing.T) string {
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
---

Body.
`)
	mustWrite(filepath.Join(dir, "rules", "other.md"), `---
name: other
description: Sibling rule.
---

Body.
`)
	mustWrite(filepath.Join(dir, "agents", "code-reviewer.md"), `---
name: code-reviewer
description: Reviews diffs.
---

Body.
`)
	return dir
}

func setupEmptyGraphFixture(t *testing.T) string {
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
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runGraph(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs(append([]string{"graph"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestGraph_TextMatrixIncludesSpecsAndTargets(t *testing.T) {
	dir := setupGraphFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	got := runGraph(t)
	if !strings.Contains(got, "spec") || !strings.Contains(got, "claude") || !strings.Contains(got, "cursor") {
		t.Errorf("expected matrix header, got:\n%s", got)
	}
	if !strings.Contains(got, "no-console-log") {
		t.Errorf("expected spec row no-console-log, got:\n%s", got)
	}
	if !strings.Contains(got, "code-reviewer") {
		t.Errorf("expected spec row code-reviewer, got:\n%s", got)
	}
	if !strings.Contains(got, "rule") {
		t.Errorf("expected rule kind cell, got:\n%s", got)
	}
	if !strings.Contains(got, "agent") {
		t.Errorf("expected agent kind cell, got:\n%s", got)
	}
}

func TestGraph_TextMatrixDeterministic(t *testing.T) {
	dir := setupGraphFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	first := runGraph(t)
	second := runGraph(t)
	if first != second {
		t.Errorf("graph output not deterministic\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	// Sorted specs: code-reviewer < no-console-log < other.
	idxA := strings.Index(first, "code-reviewer")
	idxB := strings.Index(first, "no-console-log")
	idxC := strings.Index(first, "other")
	if idxA < 0 || idxB < 0 || idxC < 0 {
		t.Fatalf("missing spec rows in:\n%s", first)
	}
	if !(idxA < idxB && idxB < idxC) {
		t.Errorf("spec rows not sorted: code-reviewer=%d no-console-log=%d other=%d", idxA, idxB, idxC)
	}
}

func TestGraph_JSONFormatHasEdges(t *testing.T) {
	dir := setupGraphFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	out := runGraph(t, "--format", "json")
	var got []graphEdge
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(got) == 0 {
		t.Fatal("expected at least one edge in JSON output")
	}
	for _, e := range got {
		if e.Spec == "" || e.Kind == "" || e.Target == "" || e.Path == "" {
			t.Errorf("edge has empty field: %+v", e)
		}
	}
}

func TestGraph_MermaidFormatStartsWithGraph(t *testing.T) {
	dir := setupGraphFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	out := runGraph(t, "--format", "mermaid")
	if !strings.HasPrefix(out, "graph LR\n") {
		t.Errorf("mermaid must start with 'graph LR', got:\n%s", out)
	}
	if !strings.Contains(out, "-->") {
		t.Errorf("mermaid must contain edges, got:\n%s", out)
	}
}

func TestGraph_DOTFormatIsDigraph(t *testing.T) {
	dir := setupGraphFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	out := runGraph(t, "--format", "dot")
	if !strings.HasPrefix(out, "digraph agnostic_ai {") {
		t.Errorf("dot must start with 'digraph agnostic_ai {', got:\n%s", out)
	}
	if !strings.Contains(out, "->") {
		t.Errorf("dot must contain edges, got:\n%s", out)
	}
	if !strings.HasSuffix(strings.TrimSpace(out), "}") {
		t.Errorf("dot must end with '}', got:\n%s", out)
	}
}

func TestGraph_TargetFilterNarrowsToOneColumn(t *testing.T) {
	dir := setupGraphFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	out := runGraph(t, "--target", "claude", "--format", "json")
	var edges []graphEdge
	if err := json.Unmarshal([]byte(out), &edges); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(edges) == 0 {
		t.Fatal("expected at least one edge for claude")
	}
	for _, e := range edges {
		if e.Target != "claude" {
			t.Errorf("expected only claude edges, got %q", e.Target)
		}
	}
}

func TestGraph_SpecFilterNarrowsToOneRow(t *testing.T) {
	dir := setupGraphFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	out := runGraph(t, "--spec", "no-console-log", "--format", "json")
	var edges []graphEdge
	if err := json.Unmarshal([]byte(out), &edges); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(edges) == 0 {
		t.Fatal("expected edges for no-console-log")
	}
	for _, e := range edges {
		if e.Spec != "no-console-log" {
			t.Errorf("expected only no-console-log spec, got %q", e.Spec)
		}
	}
}

func TestGraph_KindFilterRule(t *testing.T) {
	dir := setupGraphFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	out := runGraph(t, "--kind", "rule", "--format", "json")
	var edges []graphEdge
	if err := json.Unmarshal([]byte(out), &edges); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(edges) == 0 {
		t.Fatal("expected at least one rule edge")
	}
	for _, e := range edges {
		if e.Kind != "rule" {
			t.Errorf("expected only rule edges, got %q", e.Kind)
		}
	}
}

func TestGraph_EmptyBundleProducesEmptyMatrix(t *testing.T) {
	dir := setupEmptyGraphFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	got := runGraph(t)
	if !strings.Contains(got, "(no spec → target edges to display)") {
		t.Errorf("expected empty-matrix sentinel, got:\n%s", got)
	}

	out := runGraph(t, "--format", "json")
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("expected '[]' for empty JSON, got: %q", out)
	}

	mer := runGraph(t, "--format", "mermaid")
	if strings.TrimSpace(mer) != "graph LR" {
		t.Errorf("expected lone 'graph LR' header, got: %q", mer)
	}
}

func TestGraph_UnknownFormatErrors(t *testing.T) {
	dir := setupGraphFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"graph", "--format", "yaml"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown --format") {
		t.Fatalf("expected unknown-format error, got %v", err)
	}
}
