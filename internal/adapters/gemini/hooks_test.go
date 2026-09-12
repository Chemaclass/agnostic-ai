package gemini

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_HooksReachNativeLoader(t *testing.T) {
	dir := testutil.TempCwd(t)
	bundle := spec.NewBundle([]spec.Entry{{Kind: spec.KindHook, Name: "check", Meta: map[string]any{
		"event": "BeforeTool", "matcher": "write_file", "command": []any{"echo first", "echo second"},
		"timeout": 5, "description": "Check changes", "x-gemini": map[string]any{"sequential": true},
	}}})
	if err := New().Emit(emit.NewSession(), bundle, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Hooks map[string][]struct {
			Matcher    string
			Sequential bool
			Hooks      []struct {
				Type, Command, Description string
				Timeout                    int
			}
		}
	}
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, ".gemini/settings.json"))), &settings); err != nil {
		t.Fatal(err)
	}
	groups := settings.Hooks["BeforeTool"]
	if len(groups) != 1 || len(groups[0].Hooks) != 2 {
		t.Fatalf("native loader needs one definition with two nested handlers, got %+v", groups)
	}
	if groups[0].Matcher != "write_file" || !groups[0].Sequential {
		t.Errorf("definition lost its matcher or execution order: %+v", groups[0])
	}
	for i, command := range []string{"echo first", "echo second"} {
		hook := groups[0].Hooks[i]
		if hook.Type != "command" || hook.Command != command || hook.Timeout != 5000 || hook.Description != "Check changes" {
			t.Errorf("handler %d lost native fields: %+v", i, hook)
		}
	}
}
