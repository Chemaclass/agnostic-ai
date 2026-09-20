package augment

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// An `x-augment.hooks` block joins the generated one event by event.
// The generated block is an ordered document, not a plain map, so the
// shape comparison read the two as incompatible and the hatch replaced
// every translated hook without merging (#976).
func TestEmit_SettingsCustomHooksJoinTheGeneratedBlock(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
			"event": "PreToolUse", "matcher": "launch-process", "command": "hooks/guard.sh",
		}},
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"x-augment": map[string]any{
			"hooks": map[string]any{
				"SessionStart": []any{map[string]any{
					"hooks": []any{map[string]any{"type": "command", "command": "hooks/start.sh"}},
				}},
			},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, ".augment/settings.json"))), &got); err != nil {
		t.Fatal(err)
	}
	hooks, ok := got["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("hooks = %#v, want an object", got["hooks"])
	}
	for _, want := range []string{"PreToolUse", "SessionStart"} {
		if _, held := hooks[want]; !held {
			t.Errorf("%q missing from the merged hook block: %#v", want, hooks)
		}
	}
	emit.FlushCoverageNotes()
	if note := buf.String(); strings.Contains(note, "hooks") {
		t.Errorf("two hook objects merge, so nothing is conflicted, got: %s", note)
	}
}

// `x-augment.toolPermissions` is excluded from the general merge
// because buildToolPermissions reads it itself, and it read a value of
// the wrong shape as absent. The author's policy did nothing, no note
// fired, and the translated rules shipped in its place (#976).
func TestEmit_SettingsCustomToolPermissionsWrongShapeNotes(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)
	entries := []spec.Entry{{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{
		"permissions": map[string]any{"deny": []any{"Bash(rm:*)"}},
		// Augment's own key takes an array of rule objects.
		"x-augment": map[string]any{"toolPermissions": map[string]any{"toolName": "terminal"}},
	}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	note := buf.String()
	for _, want := range []string{"x-augment.toolPermissions", "augment", "a list"} {
		if !strings.Contains(note, want) {
			t.Errorf("expected the note to mention %q, got: %s", want, note)
		}
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, ".augment/settings.json"))), &got); err != nil {
		t.Fatal(err)
	}
	rules, _ := got["toolPermissions"].([]any)
	if len(rules) != 1 {
		t.Errorf("toolPermissions = %#v, want only the translated rule", got["toolPermissions"])
	}
}
