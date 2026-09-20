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

// An `x-opencode` key on a settings spec reaches opencode.json
// untouched, and leaves the managed `model` and `permission` alone (#949).
func TestEmit_SettingsCustomTargetKeysReachTheFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{
		"model":       "provider/model",
		"permissions": map[string]any{"allow": []any{"Bash(go test:*)"}},
		"x-opencode":  map[string]any{"theme": "tokyonight"},
	}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["theme"] != "tokyonight" {
		t.Errorf("x-opencode.theme never reached the file: %#v", got)
	}
	if got["model"] != "provider/model" {
		t.Errorf("model = %#v, want the managed key untouched", got["model"])
	}
	if perms, _ := got["permission"].(map[string]any); len(perms) == 0 {
		t.Errorf("permission = %#v, want the managed key untouched", got["permission"])
	}
	if _, hasX := got["x-opencode"]; hasX {
		t.Errorf("the x-opencode wrapper must not be written: %#v", got)
	}
}

// A settings spec carrying only an `x-opencode` key still writes the
// file: the hatch is a reason to emit, not a passenger on another
// key (#949).
func TestEmit_SettingsCustomTargetKeysAloneWriteTheFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{
		"x-opencode": map[string]any{"theme": "tokyonight"},
	}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "opencode.json"))
	if err != nil {
		t.Fatalf("hatch-only settings spec wrote no file: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["theme"] != "tokyonight" {
		t.Errorf("x-opencode.theme never reached the file: %#v", got)
	}
}
