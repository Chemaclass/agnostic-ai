package gemini

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_SettingsPreservesNativeModelSiblings(t *testing.T) {
	testutil.TempCwd(t)
	if err := os.MkdirAll(".gemini", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaultSettingsFile, []byte(`{"model":{"name":"old","maxSessionTurns":15},"ui":{"theme":"dark"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{"model": "gemini-2.5-pro", "x-gemini": map[string]any{"model": map[string]any{"name": "native-model", "compressionThreshold": 0.6}, "general": map[string]any{"previewFeatures": true}}}}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(defaultSettingsFile)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	model := doc["model"].(map[string]any)
	if model["name"] != "native-model" || model["maxSessionTurns"] != float64(15) || model["compressionThreshold"] != 0.6 {
		t.Errorf("model = %#v", model)
	}
	if doc["ui"] == nil || doc["general"] == nil {
		t.Errorf("missing settings: %s", raw)
	}
}

func TestEmit_SettingsModelAndPermissionsCoverage(t *testing.T) {
	testutil.TempCwd(t)
	notes := swapNoteWarner(t)
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{"model": "gemini-2.5-pro", "permissions": map[string]any{"deny": []any{"Bash(rm *)"}}}}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(defaultSettingsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"name": "gemini-2.5-pro"`) || strings.Contains(string(raw), `"permissions"`) {
		t.Errorf("settings: %s", raw)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(notes.String(), "permissions") {
		t.Errorf("missing coverage: %s", notes.String())
	}
}

func TestEmit_SettingsMergesMemoryWithCustomAndNativeDirectories(t *testing.T) {
	testutil.TempCwd(t)
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.MkdirAll(".gemini", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaultSettingsFile, []byte(`{"context":{"includeDirectories":["native"],"fileName":"CUSTOM.md"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Builtins:  []string{emit.MemoryBuiltin},
		Memory:    config.MemoryConfig{Personal: config.PersonalMemoryRepo},
		Gitignore: config.Gitignore{Enabled: true},
	}
	dir, err := emit.PersonalMemoryDirFor(cfg, defaultSettingsFile, target)
	if err != nil {
		t.Fatal(err)
	}
	dir = filepath.ToSlash(dir)
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{
		"x-gemini": map[string]any{"context": map[string]any{
			"includeDirectories": []any{"custom"}, "discoveryMaxDirs": 25,
		}},
	}}})
	prior := emit.PriorMergedKeys
	t.Cleanup(func() { emit.PriorMergedKeys = prior })
	var claims []emit.MergedKey
	emit.PriorMergedKeys = func(string) []emit.MergedKey { return claims }
	for range 2 {
		sess := emit.NewSession()
		sess.StartDetailedRecording()
		if err := emitSettings(sess, b, cfg, nil, defaultSettingsFile, false); err != nil {
			t.Fatal(err)
		}
		writes := sess.StopDetailedRecording()
		if len(writes) != 1 {
			t.Fatalf("writes = %v", writes)
		}
		claims = writes[0].Keys
		raw, err := os.ReadFile(defaultSettingsFile)
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Context struct {
				IncludeDirectories []string
				FileName           string
				DiscoveryMaxDirs   int
			}
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		got := slices.Clone(doc.Context.IncludeDirectories)
		slices.Sort(got)
		want := []string{"native", "custom", dir}
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("directories = %v, want %v", got, want)
		}
		if doc.Context.FileName != "CUSTOM.md" || doc.Context.DiscoveryMaxDirs != 25 {
			t.Errorf("context siblings lost: %s", raw)
		}
	}
	if _, _, err := emit.NewSession().ReleaseMergedJSON(defaultSettingsFile, claims, false, false, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(defaultSettingsFile)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Context struct {
			IncludeDirectories []string
			FileName           string
		}
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(doc.Context.IncludeDirectories, []string{"native"}) || doc.Context.FileName != "CUSTOM.md" {
		t.Errorf("release removed native settings: %s", raw)
	}
}
