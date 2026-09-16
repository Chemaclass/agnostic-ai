package qoder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_SettingsWritesModelPermissionsAndPreservesKeys(t *testing.T) {
	dir := testutil.TempCwd(t)
	path := filepath.Join(dir, ".qoder/settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"theme":"dark","model":{"temperature":0.2},"permissions":{"defaultMode":"ask"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"model": "first", "permissions": map[string]any{"allow": []any{"Read(**)", "Bash(go test:*)"}, "deny": []any{"Bash(rm:*)"}}}},
		{Kind: spec.KindSettings, Name: "project", Meta: map[string]any{"model": "second", "permissions": map[string]any{"allow": []any{"Read(**)"}, "ask": []any{"Bash(git push:*)"}}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(readFile(t, path)), &got); err != nil {
		t.Fatal(err)
	}
	model := got["model"].(map[string]any)
	permissions := got["permissions"].(map[string]any)
	if model["name"] != "second" || model["temperature"] != 0.2 {
		t.Errorf("model = %#v", model)
	}
	if permissions["defaultMode"] != "ask" {
		t.Errorf("native permission sibling was not preserved: %#v", permissions)
	}
	if model["name"] != "second" || got["theme"] != "dark" {
		t.Errorf("project settings were not merged: %#v", got)
	}
	if !reflect.DeepEqual(permissions["allow"], []any{"Read(**)", "Bash(go test:*)"}) {
		t.Errorf("allow = %#v", permissions["allow"])
	}
}
