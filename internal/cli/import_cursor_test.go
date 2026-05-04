package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportFromCursor_RefusesIfConfigExists(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agnostic.config.yaml"), "version: 1\n")
	if err := importFromCursor(dir); err == nil {
		t.Error("expected error when config already exists")
	}
}

func TestImportFromCursor_TranslatesRules(t *testing.T) {
	dir := t.TempDir()
	mdc := `---
description: Always use Conventional Commits.
globs: "**/*"
alwaysApply: true
---

Use ` + "`feat:`" + `, ` + "`fix:`" + `, etc.
`
	writeFile(t, filepath.Join(dir, ".cursor", "rules", "conventional-commits.mdc"), mdc)

	if err := importFromCursor(dir); err != nil {
		t.Fatal(err)
	}

	out, err := os.ReadFile(filepath.Join(dir, "rules", "conventional-commits.md"))
	if err != nil {
		t.Fatalf("missing rules/conventional-commits.md: %v", err)
	}
	got := string(out)
	for _, want := range []string{
		"name: conventional-commits",
		"description: Always use Conventional Commits.",
		"alwaysApply: true",
		"Use `feat:`, `fix:`, etc.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output, got:\n%s", want, got)
		}
	}
}

func TestImportFromCursor_NoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".cursor", "rules", "loose.mdc"), "Just plain body.\n")

	if err := importFromCursor(dir); err != nil {
		t.Fatal(err)
	}

	out, err := os.ReadFile(filepath.Join(dir, "rules", "loose.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if !strings.Contains(got, "name: loose") {
		t.Errorf("expected name: loose in frontmatter, got:\n%s", got)
	}
	if !strings.Contains(got, "Just plain body.") {
		t.Errorf("expected body preserved, got:\n%s", got)
	}
}

func TestImportFromCursor_PreservesUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	mdc := `---
description: scoped
globs: "src/**/*.ts"
alwaysApply: false
customKey: keep-me
---

body
`
	writeFile(t, filepath.Join(dir, ".cursor", "rules", "scoped.mdc"), mdc)

	if err := importFromCursor(dir); err != nil {
		t.Fatal(err)
	}

	out, err := os.ReadFile(filepath.Join(dir, "rules", "scoped.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, want := range []string{
		"globs: src/**/*.ts",
		"customKey: keep-me",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output, got:\n%s", want, got)
		}
	}
}

func TestImportFromCursor_NoCursorDir(t *testing.T) {
	dir := t.TempDir()
	if err := importFromCursor(dir); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "rules"))
	if len(entries) != 0 {
		t.Errorf("expected empty rules/, got %d entries", len(entries))
	}
}

func TestImportFromCursor_WritesCursorOnlyConfig(t *testing.T) {
	dir := t.TempDir()
	if err := importFromCursor(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "agnostic.config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "- cursor") {
		t.Errorf("expected cursor target, got %s", data)
	}
	if strings.Contains(string(data), "- claude") {
		t.Errorf("expected only cursor target, got %s", data)
	}
}

func TestImportFromCursor_SkipsNonMdc(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".cursor", "rules", "keep.mdc"), "body\n")
	writeFile(t, filepath.Join(dir, ".cursor", "rules", "ignore.txt"), "ignored\n")

	if err := importFromCursor(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "keep.md")); err != nil {
		t.Errorf("expected keep.md, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "ignore.md")); err == nil {
		t.Error("did not expect ignore.md")
	}
}

func TestInitCmd_FromCursorRoutes(t *testing.T) {
	dir := t.TempDir()
	mdc := `---
description: routed
---

body
`
	writeFile(t, filepath.Join(dir, ".cursor", "rules", "routed.mdc"), mdc)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"init", "--from", "cursor"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "routed.md")); err != nil {
		t.Error("expected rules/routed.md after init --from cursor")
	}
}
