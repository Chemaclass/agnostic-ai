package kiro

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

func swapNoteWarner(t *testing.T) *strings.Builder {
	t.Helper()
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	return buf
}

func settingsBundle(meta map[string]any) spec.Bundle {
	return spec.NewBundle([]spec.Entry{{Kind: spec.KindSettings, Name: "defaults", Path: "settings/defaults.yaml", Meta: meta}})
}

func readCLIJSON(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(defaultSettingsFile)
	if err != nil {
		t.Fatalf("read %s: %v", defaultSettingsFile, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse %s: %v\n%s", defaultSettingsFile, err, raw)
	}
	return doc
}

func writeCLIJSON(t *testing.T, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(defaultSettingsFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaultSettingsFile, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEmit_WritesSettingsModelToCLIJSON(t *testing.T) {
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	testutil.TempCwd(t)
	b := settingsBundle(map[string]any{"model": "claude-sonnet-4.6"})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	doc := readCLIJSON(t)
	if len(doc) != 1 || doc["chat.defaultModel"] != "claude-sonnet-4.6" {
		t.Errorf("cli.json = %#v", doc)
	}
}

func TestEmit_SettingsKeepsOtherCLIJSONKeys(t *testing.T) {
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	testutil.TempCwd(t)
	writeCLIJSON(t, `{"chat.defaultAgent": "mine", "chat.outputStyle": "concise"}`)
	b := settingsBundle(map[string]any{"model": "claude-sonnet-4.6"})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	doc := readCLIJSON(t)
	if doc["chat.defaultAgent"] != "mine" || doc["chat.outputStyle"] != "concise" || doc["chat.defaultModel"] != "claude-sonnet-4.6" {
		t.Errorf("cli.json = %#v", doc)
	}
}

func TestEmit_SettingsModelPerTargetWins(t *testing.T) {
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	testutil.TempCwd(t)
	b := settingsBundle(map[string]any{"model": map[string]any{"default": "gpt-5", "kiro": "claude-sonnet-4.6"}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if got := readCLIJSON(t)["chat.defaultModel"]; got != "claude-sonnet-4.6" {
		t.Errorf("chat.defaultModel = %v", got)
	}
}

func TestEmit_SettingsDropsClaudeAlias(t *testing.T) {
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	testutil.TempCwd(t)
	notes := swapNoteWarner(t)
	b := settingsBundle(map[string]any{"model": "sonnet"})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if _, err := os.Stat(defaultSettingsFile); !os.IsNotExist(err) {
		t.Errorf("a Claude alias must not write %s (stat err: %v)", defaultSettingsFile, err)
	}
	if got := emit.PendingCoverageNotesCount(); got != 1 {
		t.Errorf("pending coverage notes = %d, want 1", got)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(notes.String(), "model") || !strings.Contains(notes.String(), "sonnet") {
		t.Errorf("note must name the model: %s", notes.String())
	}
}

func TestEmit_SettingsPermissionsRaiseANote(t *testing.T) {
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	testutil.TempCwd(t)
	notes := swapNoteWarner(t)
	b := settingsBundle(map[string]any{"permissions": map[string]any{"allow": []any{"Bash(go test *)"}}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if _, err := os.Stat(defaultSettingsFile); !os.IsNotExist(err) {
		t.Errorf("permissions alone must not write %s (stat err: %v)", defaultSettingsFile, err)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(notes.String(), "permissions") {
		t.Errorf("missing permissions note: %s", notes.String())
	}
}

func TestEmit_NoSettingsModelLeavesCLIJSONUntouched(t *testing.T) {
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	testutil.TempCwd(t)
	const original = "{\"chat.defaultModel\":   \"auto\",\n  \"chat.defaultAgent\": \"mine\"}\n"
	writeCLIJSON(t, original)
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule"}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	raw, err := os.ReadFile(defaultSettingsFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != original {
		t.Errorf("cli.json changed:\n%s", raw)
	}
}

func TestRendersSpecFile_SettingsNeedAModelKiroCanRead(t *testing.T) {
	cases := map[string]struct {
		meta map[string]any
		want bool
	}{
		"full id":          {map[string]any{"model": "claude-sonnet-4.6"}, true},
		"per-target":       {map[string]any{"model": map[string]any{"default": "sonnet", "kiro": "claude-sonnet-4.6"}}, true},
		"claude alias":     {map[string]any{"model": "sonnet"}, false},
		"permissions only": {map[string]any{"permissions": map[string]any{"allow": []any{"Read"}}}, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := spec.Entry{Kind: spec.KindSettings, Name: "defaults", Meta: tc.meta}
			if got := New().RendersSpecFile(e, &config.Config{}, ""); got != tc.want {
				t.Errorf("RendersSpecFile = %v, want %v", got, tc.want)
			}
		})
	}
	rule := spec.Entry{Kind: spec.KindRule, Name: "r"}
	if !New().RendersSpecFile(rule, &config.Config{}, "") {
		t.Error("a non-settings spec always renders")
	}
}
