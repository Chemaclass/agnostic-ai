package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportFromCline_RefusesIfConfigExists(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agnostic.config.yaml"), "version: 1\n")
	if err := importFromCline(dir); err == nil {
		t.Error("expected error when config already exists")
	}
}

func TestImportFromCline_ReclassifiesByPrefix(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".clinerules", "style.md"), "# style\n\nUse 2-space indent.\n")
	writeFile(t, filepath.Join(dir, ".clinerules", "agent-reviewer.md"), "# Agent: reviewer\n\nReview the diff.\n")
	writeFile(t, filepath.Join(dir, ".clinerules", "skill-validator.md"), "# Skill: validator\n\nValidate input.\n")

	if err := importFromCline(dir); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		path    string
		mustHas string
	}{
		{"rules/style.md", "Use 2-space indent."},
		{"agents/reviewer.md", "Review the diff."},
		{"skills/validator.md", "Validate input."},
	}
	for _, c := range cases {
		out, err := os.ReadFile(filepath.Join(dir, c.path))
		if err != nil {
			t.Errorf("missing %s: %v", c.path, err)
			continue
		}
		if !strings.Contains(string(out), c.mustHas) {
			t.Errorf("%s body missing %q, got:\n%s", c.path, c.mustHas, out)
		}
	}
}

func TestImportFromCline_StripsLeadingHeading(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".clinerules", "style.md"),
		"# style\n\nbody line\n")
	if err := importFromCline(dir); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(filepath.Join(dir, "rules", "style.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if strings.Contains(got, "# style\n") {
		t.Errorf("expected leading heading stripped, got:\n%s", got)
	}
	if !strings.Contains(got, "name: style") {
		t.Errorf("expected name frontmatter, got:\n%s", got)
	}
	if !strings.Contains(got, "body line") {
		t.Errorf("expected body preserved, got:\n%s", got)
	}
}

func TestImportFromCline_PreservesScopeSubdirs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".clinerules", "backend", "auth.md"),
		"# auth\n\nauth body\n")
	if err := importFromCline(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "backend", "auth.md")); err != nil {
		t.Errorf("expected scope preserved at rules/backend/auth.md: %v", err)
	}
}

func TestImportFromCline_NoSourceDir(t *testing.T) {
	dir := t.TempDir()
	if err := importFromCline(dir); err != nil {
		t.Fatal(err)
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic.config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfg), "- cline") {
		t.Errorf("expected cline target, got:\n%s", cfg)
	}
}

func TestImportFromWindsurf_ReadsWindsurfRulesDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".windsurf", "rules", "style.md"),
		"# style\n\nwindsurf body\n")
	if err := importFromWindsurf(dir); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(filepath.Join(dir, "rules", "style.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "windsurf body") {
		t.Errorf("body not imported, got:\n%s", out)
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, "agnostic.config.yaml"))
	if !strings.Contains(string(cfg), "- windsurf") {
		t.Errorf("expected windsurf target, got:\n%s", cfg)
	}
}

func TestImportFromContinue_ReadsContinueRulesDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".continue", "rules", "style.md"),
		"# style\n\ncontinue body\n")
	if err := importFromContinue(dir); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(filepath.Join(dir, "rules", "style.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "continue body") {
		t.Errorf("body not imported, got:\n%s", out)
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, "agnostic.config.yaml"))
	if !strings.Contains(string(cfg), "- continue") {
		t.Errorf("expected continue target, got:\n%s", cfg)
	}
}

func TestInitCmd_FromClineRoutes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".clinerules", "routed.md"), "# routed\n\nbody\n")
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"init", "--from", "cline"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "routed.md")); err != nil {
		t.Error("expected rules/routed.md after init --from cline")
	}
}

func TestInitCmd_FromWindsurfRoutes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".windsurf", "rules", "routed.md"), "# routed\n\nbody\n")
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"init", "--from", "windsurf"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "routed.md")); err != nil {
		t.Error("expected rules/routed.md after init --from windsurf")
	}
}

func TestInitCmd_FromContinueRoutes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".continue", "rules", "routed.md"), "# routed\n\nbody\n")
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"init", "--from", "continue"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "routed.md")); err != nil {
		t.Error("expected rules/routed.md after init --from continue")
	}
}

func TestStripLeadingHeading_NoHeading(t *testing.T) {
	got := stripLeadingHeading("plain body\n")
	if got != "plain body\n" {
		t.Errorf("unexpected change: %q", got)
	}
}

func TestClassifyRulesDirFile(t *testing.T) {
	cases := map[string]struct {
		kindDir, baseName string
	}{
		"style.md":             {"rules", "style"},
		"agent-reviewer.md":    {"agents", "reviewer"},
		"skill-validator.md":   {"skills", "validator"},
		"backend/auth.md":      {"rules", "auth"},
		"backend/agent-foo.md": {"agents", "foo"},
	}
	for in, want := range cases {
		k, n := classifyRulesDirFile(in)
		if k != want.kindDir || n != want.baseName {
			t.Errorf("%s: got (%q,%q), want (%q,%q)", in, k, n, want.kindDir, want.baseName)
		}
	}
}
