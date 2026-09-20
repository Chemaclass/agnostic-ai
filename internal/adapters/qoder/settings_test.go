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

// An `x-qoder` key on a settings spec reaches `.qoder/settings.json`
// untouched, and leaves the managed `model` and `permissions` alone (#949).
func TestEmit_SettingsCustomTargetKeysReachTheFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{
		"model":       "qoder-max",
		"permissions": map[string]any{"allow": []any{"Bash(go test:*)"}},
		"x-qoder":     map[string]any{"enableAllProjectMcpServers": true},
	}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".qoder/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["enableAllProjectMcpServers"] != true {
		t.Errorf("x-qoder.enableAllProjectMcpServers never reached the file: %#v", got)
	}
	model, _ := got["model"].(map[string]any)
	if model["name"] != "qoder-max" {
		t.Errorf("model = %#v, want the managed key untouched", got["model"])
	}
	perms, _ := got["permissions"].(map[string]any)
	if allow, _ := perms["allow"].([]any); len(allow) != 1 {
		t.Errorf("permissions = %#v, want the managed key untouched", got["permissions"])
	}
	if _, hasX := got["x-qoder"]; hasX {
		t.Errorf("the x-qoder wrapper must not be written: %#v", got)
	}
}

// An `x-qoder.permissions` block joins the translated one key by key,
// so a Qoder-only deny rule adds to the deny list instead of taking
// the translated rules with it (#966).
func TestEmit_SettingsCustomPermissionsJoinTheTranslatedOnes(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{
		"permissions": map[string]any{
			"deny":  []any{"Bash(rm:*)"},
			"allow": []any{"Bash(go test:*)"},
		},
		"x-qoder": map[string]any{
			"permissions": map[string]any{"deny": []any{"AuthorOnly"}},
		},
	}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".qoder/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	perms, _ := got["permissions"].(map[string]any)
	wantDeny := []any{"Bash(rm:*)", "AuthorOnly"}
	if !reflect.DeepEqual(perms["deny"], wantDeny) {
		t.Errorf("deny = %#v, want %#v", perms["deny"], wantDeny)
	}
	if allow, _ := perms["allow"].([]any); len(allow) != 1 {
		t.Errorf("allow = %#v, want the translated allow list kept", perms["allow"])
	}
}
