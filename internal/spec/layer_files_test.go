package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func TestLoadLayered_FilesReplaceAddAndHideSettingsWithoutWriting(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "settings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(dir, "native.yaml")
	gone := filepath.Join(dir, "old.yaml")
	for path, body := range map[string]string{native: "name: native\neffort: low\n", gone: "name: old\nmodel: m\n"} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	added := filepath.Join(dir, "defaults.yaml")
	layer := Layer{Name: "project", Root: root, Sources: config.Sources{Settings: "settings"}, Files: map[string][]byte{
		native: []byte("name: native\neffort: high\n"),
		gone:   nil,
		added:  []byte("name: defaults\nmodel: opus\n"),
	}}
	b, err := LoadLayered([]Layer{layer})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range b.Settings {
		names = append(names, e.Name)
	}
	if len(b.Settings) != 2 || b.Settings[0].Name != "defaults" || b.Settings[0].Path != added || b.Settings[0].Meta["model"] != "opus" || b.Settings[1].Meta["effort"] != "high" {
		t.Fatalf("settings = %v, want defaults then native with the planned values", names)
	}
	if data, _ := os.ReadFile(native); string(data) != "name: native\neffort: low\n" {
		t.Errorf("native.yaml changed on disk: %q", data)
	}
	if _, err := os.Stat(added); !os.IsNotExist(err) {
		t.Errorf("defaults.yaml was written: %v", err)
	}
}
