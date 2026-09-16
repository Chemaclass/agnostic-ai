package junie

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
	path := filepath.Join(dir, ".junie/config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"effort":"high","brave":false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []spec.Entry{{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{"model": "sonnet"}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(readFile(t, path)), &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "sonnet" || got["effort"] != "high" || got["brave"] != false {
		t.Errorf("project settings were not merged: %#v", got)
	}
}
