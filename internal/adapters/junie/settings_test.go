package junie

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// TestEmit_SettingsNotesPermissionsAreUserTierOnly covers the gap #917
// filed: a portable permission policy reached nothing here and said
// nothing either.
//
// Junie's project tier is real for other fields, which is what makes
// the absence a documented fact rather than an unchecked one. Its one
// rule-based approvals file, allowlist.json, is documented at
// ~/.junie/ alone, and the project-tier .junie/config.json field list
// has no allow, deny, or ask key.
func TestEmit_SettingsNotesPermissionsAreUserTierOnly(t *testing.T) {
	dir := testutil.TempCwd(t)
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })

	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"allow": []any{"Read(**)"},
			"ask":   []any{"Bash(git push:*)"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()

	note := buf.String()
	for _, want := range []string{"`permissions`", "junie", "allowlist.json"} {
		if !strings.Contains(note, want) {
			t.Errorf("expected coverage note to mention %q, got: %s", want, note)
		}
	}
	// No model in the spec, so nothing should reach the config file.
	if _, err := os.Stat(filepath.Join(dir, ".junie", "config.json")); err == nil {
		t.Error("a permissions-only settings spec wrote .junie/config.json")
	}
}

// An `x-junie` key on a settings spec reaches `.junie/config.json`
// untouched, and leaves `model` alone (#949).
func TestEmit_SettingsCustomTargetKeysReachTheFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{
		"model":   "sonnet",
		"x-junie": map[string]any{"brave": true},
	}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, ".junie/config.json"))), &got); err != nil {
		t.Fatal(err)
	}
	if got["brave"] != true {
		t.Errorf("x-junie.brave never reached the file: %#v", got)
	}
	if got["model"] != "sonnet" {
		t.Errorf("model = %#v, want the managed key untouched", got["model"])
	}
	if _, hasX := got["x-junie"]; hasX {
		t.Errorf("the x-junie wrapper must not be written: %#v", got)
	}
}
