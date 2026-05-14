package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportFromAider_NoConventionsMd(t *testing.T) {
	dir := t.TempDir()
	if err := importFromAider(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "rules"))
	if len(entries) != 0 {
		t.Errorf("expected empty rules/, got %d entries", len(entries))
	}
	if _, err := os.Stat(filepath.Join(dir, agnosticMainFile)); !os.IsNotExist(err) {
		t.Errorf("expected no AGNOSTIC_AI.md when CONVENTIONS.md absent: %v", err)
	}
}

func TestImportFromAider_MirrorsConventionsMd(t *testing.T) {
	dir := t.TempDir()
	body := "# Conventions\n\nTop-level.\n\n## go-style\n\ngofmt clean.\n"
	writeFile(t, filepath.Join(dir, aiderMainFile), body)
	if err := importFromAider(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, agnosticMainFile))
	if err != nil {
		t.Fatalf("missing %s: %v", agnosticMainFile, err)
	}
	if string(got) != body {
		t.Errorf("AGNOSTIC_AI.md not byte-identical to CONVENTIONS.md. got %q", got)
	}
}

func TestImportFromAider_SlicesH2(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, aiderMainFile), "## go-style\n\ngofmt clean.\n\n## commits\n\nConventional commits.\n")
	if err := importFromAider(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"go-style", "commits"} {
		data, err := os.ReadFile(filepath.Join(dir, "rules", name+".md"))
		if err != nil {
			t.Fatalf("missing rules/%s.md: %v", name, err)
		}
		if !strings.Contains(string(data), "name: "+name) {
			t.Errorf("expected name: %s in %s", name, data)
		}
	}
}
