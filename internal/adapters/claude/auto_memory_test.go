package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func readLocalSettings(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(".claude", "settings.local.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestEmit_PointsAutoMemoryAtThePersonalStore(t *testing.T) {
	testutil.TempCwd(t)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{Builtins: []string{"memory"}}, false); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(wd, ".agnostic-ai", "local", "memory")
	if got := readLocalSettings(t)["autoMemoryDirectory"]; got != filepath.ToSlash(want) {
		t.Errorf("autoMemoryDirectory = %v, want %s", got, filepath.ToSlash(want))
	}
}

func TestEmit_LeavesAutoMemoryAloneWithoutTheMemoryBuiltin(t *testing.T) {
	testutil.TempCwd(t)

	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(".claude", "settings.local.json")); !os.IsNotExist(err) {
		t.Errorf("settings.local.json written without the memory built-in: %v", err)
	}
}

func TestEmit_KeepsTheUsersOwnAutoMemoryDirectory(t *testing.T) {
	testutil.TempCwd(t)
	if err := os.MkdirAll(".claude", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".claude", "settings.local.json"), []byte(`{"autoMemoryDirectory": "~/notes", "theme": "dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{Builtins: []string{"memory"}}, false); err != nil {
		t.Fatal(err)
	}

	doc := readLocalSettings(t)
	if doc["autoMemoryDirectory"] != "~/notes" || doc["theme"] != "dark" {
		t.Errorf("user settings changed: %v", doc)
	}
}
