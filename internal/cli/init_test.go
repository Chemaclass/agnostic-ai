package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScaffold_CreatesLayout(t *testing.T) {
	dir := t.TempDir()
	if err := scaffold(dir); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"agents", "skills", "rules", "hooks"} {
		if _, err := os.Stat(filepath.Join(dir, d)); err != nil {
			t.Errorf("missing %s", d)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "agnostic.config.yaml")); err != nil {
		t.Error("missing agnostic.config.yaml")
	}
}

func TestScaffold_RefusesIfConfigExists(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agnostic.config.yaml"),
		[]byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := scaffold(dir); err == nil {
		t.Error("expected error when config already exists")
	}
}
