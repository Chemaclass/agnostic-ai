package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestLoad_ReadsAbsoluteSourceDirectory(t *testing.T) {
	root := t.TempDir()
	testutil.Chdir(t, root)
	external := t.TempDir()
	path := filepath.Join(external, "style.md")
	if err := os.WriteFile(path, []byte("---\nname: style\n---\nUse plain words.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := LoadBundle(".", &config.Config{Sources: config.Sources{Rules: external}})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Rules) != 1 || b.Rules[0].Path != path {
		t.Errorf("absolute source rules = %#v", b.Rules)
	}
}
