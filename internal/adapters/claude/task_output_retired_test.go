package claude

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

// A config that still sets taskOutputMaxChars gets a note, and the key an
// earlier sync wrote leaves settings.json (#1381).
func TestEmit_DropsRetiredTaskOutputMaxChars(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	if err := os.MkdirAll(".claude", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".claude", "settings.json"),
		[]byte(`{"taskOutputMaxChars": 128000, "model": "opus"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	n := 128000
	cfg := &config.Config{Outputs: map[string]config.Output{
		"claude": {Settings: &config.ClaudeSettings{TaskOutputMaxChars: &n}},
	}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	emit.FlushCoverageNotes()

	raw, err := os.ReadFile(filepath.Join(".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc["taskOutputMaxChars"]; ok {
		t.Errorf("taskOutputMaxChars kept: %s", raw)
	}
	if doc["model"] != "opus" {
		t.Errorf("unrelated key lost: %s", raw)
	}
	if !strings.Contains(buf.String(), "taskOutputMaxChars") {
		t.Errorf("no note about taskOutputMaxChars:\n%s", buf.String())
	}
}

// A fresh project whose config sets only the retired key gets the note and
// no settings.json: there is nothing to remove and nothing else to write.
func TestEmit_RetiredKeyAloneWritesNoSettingsFile(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	n := 128000
	cfg := &config.Config{Outputs: map[string]config.Output{
		"claude": {Settings: &config.ClaudeSettings{TaskOutputMaxChars: &n}},
	}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(".claude", "settings.json")); !os.IsNotExist(err) {
		t.Errorf("settings.json written for a retired key alone: %v", err)
	}
}

// A settings.json that cannot be parsed fails the sync instead of skipping
// the retired-key cleanup unseen.
func TestEmit_RetiredKeyWithUnparsableSettingsFails(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	if err := os.MkdirAll(".claude", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".claude", "settings.json"), []byte(`{"taskOutputMaxChars": `), 0o644); err != nil {
		t.Fatal(err)
	}
	n := 128000
	cfg := &config.Config{Outputs: map[string]config.Output{
		"claude": {Settings: &config.ClaudeSettings{TaskOutputMaxChars: &n}},
	}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err == nil {
		t.Error("emit succeeded over an unparsable settings.json")
	}
}
