package spec

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func TestLoadBundle_RulePathsReadsAsGlobs(t *testing.T) {
	dir := t.TempDir()
	rules := filepath.Join(dir, "rules")
	if err := os.MkdirAll(rules, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"paths.md":    "---\nname: paths\npaths:\n  - resources/js/**\ndescription: d\n---\nBody.\n",
		"same.md":     "---\nname: same\npaths: [\"a/**\"]\nglobs: \"a/**\"\n---\nBody.\n",
		"off.md":      "---\nname: off\npaths: []\n---\nBody.\n",
		"conflict.md": "---\nname: conflict\npaths: [\"a/**\"]\nglobs: [\"b/**\"]\n---\nBody.\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(rules, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b, err := LoadBundle(dir, &config.Config{Sources: config.Sources{Rules: "rules"}})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Entry{}
	for _, r := range b.Rules {
		byName[r.Name] = r
	}

	p := byName["paths"]
	if got := p.Globs(); got != "resources/js/**" {
		t.Errorf("paths rule globs = %q, want resources/js/**", got)
	}
	if _, kept := p.Meta["paths"]; !kept {
		t.Error("paths should stay for Claude and Cline")
	}
	if want := []string{"name", "paths", "description"}; !reflect.DeepEqual(p.MetaKeys, want) {
		t.Errorf("MetaKeys = %v, want %v", p.MetaKeys, want)
	}
	if !reflect.DeepEqual(p.LoadedKeys("paths"), []string{"paths", "globs"}) || p.PathsConflictGlobs() {
		t.Error("paths rule should report its paths as globs, with no conflict")
	}

	s := byName["same"]
	if s.Globs() != "a/**" || s.PathsConflictGlobs() || len(s.LoadedKeys("paths")) != 1 {
		t.Errorf("matching paths and globs should be left alone: %v", s.Meta)
	}

	c := byName["conflict"]
	if !c.PathsConflictGlobs() {
		t.Errorf("differing paths and globs should stay for validate to reject: %v", c.Meta)
	}

	off := byName["off"]
	if _, set := off.Meta["globs"]; set {
		t.Errorf("an empty paths must not become globs: %v", off.Meta)
	}
}
