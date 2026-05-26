package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestValidate_AllowsMissingNameForMarkdownSpecs(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\n")
	mustWriteFile(t, filepath.Join(dir, "rules", "no-name.md"),
		"---\ndescription: a rule\n---\nbody\n")
	testutil.Chdir(t, dir)

	root := NewRootCmd("test")
	root.SetArgs([]string{"validate"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})
	if err := root.Execute(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	got := out.String()
	if strings.Contains(got, "issue(s) found") {
		t.Errorf("expected no issues, got %q", got)
	}
}

func TestValidate_FixLeavesOptionalNameOmitted(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\n")
	specPath := filepath.Join(dir, "rules", "no-name.md")
	mustWriteFile(t, specPath, "---\ndescription: a rule\n---\nbody\n")
	testutil.Chdir(t, dir)

	root := NewRootCmd("test")
	root.SetArgs([]string{"validate", "--fix"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})
	if err := root.Execute(); err != nil {
		t.Fatalf("validate --fix: %v", err)
	}

	if !strings.Contains(out.String(), "fixed 0 issue") {
		t.Errorf("expected fixed report, got %q", out.String())
	}

	patched, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"description: a rule", "body"}
	for _, w := range want {
		if !strings.Contains(string(patched), w) {
			t.Errorf("patched file missing %q:\n%s", w, patched)
		}
	}
	if strings.Contains(string(patched), "name: no-name") {
		t.Errorf("optional name should not be injected:\n%s", patched)
	}
}

func TestValidate_AllowsNestedSkillMissingName(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\n")
	skillPath := filepath.Join(dir, "skills", "validator", "SKILL.md")
	mustWriteFile(t, skillPath, "---\ndescription: a skill\n---\nbody\n")
	testutil.Chdir(t, dir)

	root := NewRootCmd("test")
	root.SetArgs([]string{"validate", "--fix"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	patched, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(patched), "name: validator") {
		t.Errorf("optional skill name should not be injected:\n%s", patched)
	}
}

func TestValidate_NoIssuesWhenNamePresent(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\n")
	mustWriteFile(t, filepath.Join(dir, "rules", "good.md"),
		"---\nname: good\n---\nbody\n")
	testutil.Chdir(t, dir)

	root := NewRootCmd("test")
	root.SetArgs([]string{"validate"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "issue(s) found") {
		t.Errorf("expected no issues, got %q", out.String())
	}
}

func TestInjectFrontmatterName_AddsToExistingBlock(t *testing.T) {
	in := []byte("---\ndescription: x\n---\nbody\n")
	got, err := injectFrontmatterName(in, "foo")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "name: foo") {
		t.Errorf("missing injected name:\n%s", got)
	}
	if !strings.Contains(string(got), "description: x") {
		t.Errorf("preexisting field lost:\n%s", got)
	}
	if !strings.HasSuffix(string(got), "body\n") {
		t.Errorf("body lost: %q", got)
	}
}

func TestInjectFrontmatterName_CreatesBlockIfMissing(t *testing.T) {
	in := []byte("body only\n")
	got, err := injectFrontmatterName(in, "foo")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), "---\n") {
		t.Errorf("expected leading ---, got %q", got)
	}
	if !strings.Contains(string(got), "name: foo") {
		t.Errorf("missing name: %q", got)
	}
	if !strings.HasSuffix(string(got), "body only\n") {
		t.Errorf("body lost: %q", got)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
