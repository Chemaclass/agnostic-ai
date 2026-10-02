package config_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestResolveSourceAlias_LinkedSourceResolvesExistingDirectory(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "data.txt"), []byte("Shared data.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	alias := "linked-root"
	testutil.DirectoryAlias(t, target, alias)
	resolved, err := config.ResolveSourceAlias(alias)
	if err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(alias)
	if err != nil {
		t.Fatal(err)
	}
	if resolved == alias || resolved == abs {
		t.Errorf("resolved root = %q, still the alias", resolved)
	}
	data, err := os.ReadFile(filepath.Join(resolved, "data.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "Shared data.\n" {
		t.Errorf("resolved data = %q", data)
	}
	if _, err := config.ResolveSourceAlias(filepath.Join(alias, "missing", "tail")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing tail error = %v, want missing path without fallback", err)
	}
}
