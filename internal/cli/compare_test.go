package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func setupCompareFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"agnostic-ai.yaml": "targets: [claude, cursor]\n",
		".agnostic-ai/agents/reviewer.md": `---
name: reviewer
description: Reviews diffs.
tools: [Read, Grep]
model: sonnet
---

Review the diff.
`,
		".agnostic-ai/agents/claude-only.md": `---
name: claude-only
description: Claude helper.
target-exclude: cursor
model: opus
---

Help.
`,
		".agnostic-ai/rules/go-style.md": `---
name: go-style
description: Go conventions.
globs: "**/*.go"
alwaysApply: false
---

Use gofmt.
`,
		".agnostic-ai/rules/backend/api.md": `---
name: api
description: API rules.
globs: "backend/**/*.go"
---

Wrap errors.
`,
		".agnostic-ai/rules/always.md": `---
name: always
description: Always on.
---

Be kind.
`,
	}
	for rel, body := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runCompare(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"compare"}, args...))
	err := root.Execute()
	return out.String(), err
}

func compareJSON(t *testing.T, args ...string) compareOutput {
	t.Helper()
	raw, err := runCompare(t, append(args, "--json")...)
	if err != nil {
		t.Fatal(err)
	}
	var got compareOutput
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, raw)
	}
	return got
}

func findCompareResult(t *testing.T, out compareOutput, path, field, target string) compareResult {
	t.Helper()
	for _, s := range out.Specs {
		if s.Path != path {
			continue
		}
		for _, f := range s.Fields {
			if f.Field != field {
				continue
			}
			for _, r := range f.Results {
				if r.Target == target {
					return r
				}
			}
		}
	}
	t.Fatalf("no result for %s %s %s in %+v", path, field, target, out.Specs)
	return compareResult{}
}

func TestCompare_ReportsPreservedFieldOnBothTargets(t *testing.T) {
	testutil.Chdir(t, setupCompareFixture(t))
	silence(t)
	out := compareJSON(t, "claude", "cursor")

	for target, path := range map[string]string{
		"claude": ".claude/agents/reviewer.md",
		"cursor": ".cursor/agents/reviewer.md",
	} {
		r := findCompareResult(t, out, ".agnostic-ai/agents/reviewer.md", "model", target)
		if r.Status != statusPreserved {
			t.Errorf("%s model: status %q, want preserved", target, r.Status)
		}
		if len(r.Paths) != 1 || r.Paths[0] != path {
			t.Errorf("%s model: paths %v, want [%s]", target, r.Paths, path)
		}
	}
}

func TestCompare_ReportsKnownLossWithAdapterReason(t *testing.T) {
	testutil.Chdir(t, setupCompareFixture(t))
	silence(t)
	out := compareJSON(t, "claude", "cursor")

	if r := findCompareResult(t, out, ".agnostic-ai/agents/reviewer.md", "tools", "claude"); r.Status != statusPreserved {
		t.Errorf("claude tools: status %q, want preserved", r.Status)
	}
	r := findCompareResult(t, out, ".agnostic-ai/agents/reviewer.md", "tools", "cursor")
	if r.Status != statusUnsupported {
		t.Errorf("cursor tools: status %q, want unsupported", r.Status)
	}
	if !strings.Contains(r.Reason, "readonly: true") {
		t.Errorf("cursor tools: reason %q must carry the adapter's next step", r.Reason)
	}
}

func TestCompare_ReportsTranslatedRuleActivation(t *testing.T) {
	testutil.Chdir(t, setupCompareFixture(t))
	silence(t)
	out := compareJSON(t, "claude", "cursor")

	r := findCompareResult(t, out, ".agnostic-ai/rules/go-style.md", "globs", "claude")
	if r.Status != statusTranslated {
		t.Errorf("claude globs: status %q, want translated (written as paths)", r.Status)
	}
	if len(r.Paths) != 1 || r.Paths[0] != ".claude/rules/go-style.md" {
		t.Errorf("claude globs: paths %v", r.Paths)
	}
	if r := findCompareResult(t, out, ".agnostic-ai/rules/go-style.md", "globs", "cursor"); r.Status != statusPreserved {
		t.Errorf("cursor globs: status %q, want preserved", r.Status)
	}
}

func TestCompare_ReportsRenamedValuesUnderTheSameKeyAsTranslated(t *testing.T) {
	testutil.Chdir(t, setupCompareFixture(t))
	silence(t)
	out := compareJSON(t, "claude", "gemini")

	// Gemini keeps the `tools` key but rewrites Read and Grep to its own
	// tool names, so the key alone must not read as preserved.
	r := findCompareResult(t, out, ".agnostic-ai/agents/reviewer.md", "tools", "gemini")
	if r.Status != statusTranslated {
		t.Errorf("gemini tools: status %q, want translated", r.Status)
	}
	if len(r.Paths) != 1 || r.Paths[0] != ".gemini/agents/reviewer.md" {
		t.Errorf("gemini tools: paths %v", r.Paths)
	}
}

func TestCompare_ReportsTargetExclusionWithNextStep(t *testing.T) {
	testutil.Chdir(t, setupCompareFixture(t))
	silence(t)
	out := compareJSON(t, "claude", "cursor")

	r := findCompareResult(t, out, ".agnostic-ai/agents/claude-only.md", "model", "cursor")
	if r.Status != statusExcluded {
		t.Errorf("cursor model: status %q, want excluded", r.Status)
	}
	if !strings.Contains(r.Next, ".agnostic-ai/agents/claude-only.md") {
		t.Errorf("exclusion next step must point at the source: %q", r.Next)
	}
	if r := findCompareResult(t, out, ".agnostic-ai/agents/claude-only.md", "model", "claude"); r.Status != statusPreserved {
		t.Errorf("claude model: status %q, want preserved", r.Status)
	}
}

func TestCompare_ReportsScopeLimitation(t *testing.T) {
	testutil.Chdir(t, setupCompareFixture(t))
	silence(t)
	out := compareJSON(t, "claude", "codex")

	if r := findCompareResult(t, out, ".agnostic-ai/rules/backend/api.md", "scope", "claude"); r.Status != statusTranslated {
		t.Errorf("claude scope: status %q, want translated", r.Status)
	}
	r := findCompareResult(t, out, ".agnostic-ai/rules/backend/api.md", "scope", "codex")
	if r.Status != statusExcluded {
		t.Errorf("codex scope: status %q, want excluded", r.Status)
	}
	if !strings.Contains(r.Reason, "narrower file filters") {
		t.Errorf("codex scope: reason %q must name the limitation", r.Reason)
	}
	// An unscoped rule's globs reach codex nowhere: the rule body lands in
	// AGENTS.md and the filter is not written.
	if r := findCompareResult(t, out, ".agnostic-ai/rules/go-style.md", "globs", "codex"); r.Status != statusUnsupported {
		t.Errorf("codex globs: status %q, want unsupported", r.Status)
	}
}

func TestCompare_OrdersSpecsAndFieldsStablyAndStatesCoverage(t *testing.T) {
	testutil.Chdir(t, setupCompareFixture(t))
	silence(t)
	first, err := runCompare(t, "claude", "cursor")
	if err != nil {
		t.Fatal(err)
	}
	second, err := runCompare(t, "claude", "cursor")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("output is not deterministic:\n%s\n---\n%s", first, second)
	}
	for _, want := range []string{
		"coverage: agent fields and rule scope/activation only",
		"agent claude-only  .agnostic-ai/agents/claude-only.md",
		"tools (differs)",
	} {
		if !strings.Contains(first, want) {
			t.Errorf("missing %q in:\n%s", want, first)
		}
	}
	order := []string{
		".agnostic-ai/agents/claude-only.md",
		".agnostic-ai/agents/reviewer.md",
		".agnostic-ai/rules/backend/api.md",
		".agnostic-ai/rules/go-style.md",
	}
	last := -1
	for _, p := range order {
		i := strings.Index(first, p)
		if i <= last {
			t.Errorf("%s out of order in:\n%s", p, first)
		}
		last = i
	}
	if strings.Contains(first, "rules/always.md") {
		t.Errorf("a rule without scope or activation fields must not be listed:\n%s", first)
	}
	reviewer := first[strings.Index(first, "agents/reviewer.md"):]
	if strings.Index(reviewer, "  tools") > strings.Index(reviewer, "  model") {
		t.Errorf("fields must follow source order:\n%s", reviewer)
	}
}

func TestCompare_WritesNothing(t *testing.T) {
	dir := setupCompareFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	before := snapshotTree(t, dir)
	if _, err := runCompare(t, "claude", "cursor"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCompare(t, "codex", "gemini", "--json"); err != nil {
		t.Fatal(err)
	}
	if after := snapshotTree(t, dir); !reflect.DeepEqual(after, before) {
		t.Errorf("compare changed the filesystem:\nbefore: %v\nafter: %v", before, after)
	}
}

func TestCompare_RejectsInvalidTargets(t *testing.T) {
	testutil.Chdir(t, setupCompareFixture(t))
	silence(t)
	cases := map[string][]string{
		"unknown target": {"claude", "nope"},
		"same target":    {"cursor", "cursor"},
		"one target":     {"claude"},
	}
	for name, args := range cases {
		if _, err := runCompare(t, args...); err == nil {
			t.Errorf("%s: expected an error for %v", name, args)
		}
	}
}

func TestCompare_IgnoresReaderConflictsWithTargetsOutsideThePair(t *testing.T) {
	dir := setupCompareFixture(t)
	// codex writes backend/AGENTS.md; kiro would load it globally if both
	// synced together. compare writes nothing, so that is no reason to fail.
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("targets: [claude, codex]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	silence(t)
	if _, err := runCompare(t, "claude", "kiro"); err != nil {
		t.Errorf("compare claude kiro: %v", err)
	}
}

func TestCompare_FailsOnInvalidSpec(t *testing.T) {
	dir := setupCompareFixture(t)
	bad := "---\nname: bad\nscope: ../outside\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(dir, ".agnostic-ai", "rules", "bad.md"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	silence(t)
	if _, err := runCompare(t, "claude", "cursor"); err == nil {
		t.Error("expected an error for a rule scope outside the project")
	}
}

func TestCompare_FailsOnInvalidProjectConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("targets: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	silence(t)
	if _, err := runCompare(t, "claude", "cursor"); err == nil {
		t.Error("expected an error for an invalid agnostic-ai.yaml")
	}
}
