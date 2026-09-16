package opencode

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

func TestEmit_SettingsWritesProjectModelAndPreservesKeys(t *testing.T) {
	dir := testutil.TempCwd(t)
	path := filepath.Join(dir, "opencode.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark","small_model":"fast"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []spec.Entry{{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{"model": "provider/model"}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(readFile(t, path)), &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "provider/model" || got["theme"] != "dark" || got["small_model"] != "fast" {
		t.Errorf("project settings were not merged: %#v", got)
	}
}
