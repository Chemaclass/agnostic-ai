package qoder

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// An `x-qoder.hooks` block joins the generated one event by event. The
// generated block is an ordered document, not a plain map, so the
// shape comparison read the two as incompatible and the hatch replaced
// every translated hook without merging, under a note that called two
// objects a shape conflict (#976).
func TestEmit_SettingsCustomHooksJoinTheGeneratedBlock(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
			"event": "PreToolUse", "matcher": "Bash", "command": "hooks/guard.sh",
		}},
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"x-qoder": map[string]any{
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
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, ".qoder/settings.json"))), &got); err != nil {
		t.Fatal(err)
	}
	hooks, ok := got["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("hooks = %#v, want an object", got["hooks"])
	}
	if _, held := hooks["PreToolUse"]; !held {
		t.Errorf("the translated hook was replaced by the hatch: %#v", hooks)
	}
	if _, held := hooks["SessionStart"]; !held {
		t.Errorf("the hatch event never reached the file: %#v", hooks)
	}
	emit.FlushCoverageNotes()
	if note := buf.String(); note != "" {
		t.Errorf("two hook objects merge, so nothing is conflicted, got: %s", note)
	}
}

// An `x-qoder.hooks` event the specs already produce merges under that
// event rather than taking the whole block with it.
func TestEmit_SettingsCustomHooksMergeTheSameEvent(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
			"event": "PreToolUse", "matcher": "Bash", "command": "hooks/guard.sh",
		}},
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"x-qoder": map[string]any{
			"hooks": map[string]any{
				"PreToolUse": []any{map[string]any{
					"matcher": "Write",
					"hooks":   []any{map[string]any{"type": "command", "command": "hooks/authored.sh"}},
				}},
			},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, ".qoder/settings.json"))), &got); err != nil {
		t.Fatal(err)
	}
	hooks, _ := got["hooks"].(map[string]any)
	groups, _ := hooks["PreToolUse"].([]any)
	if len(groups) != 2 {
		t.Fatalf("PreToolUse = %#v, want the translated group and the authored one", hooks["PreToolUse"])
	}
	seen := map[string]bool{}
	for _, g := range groups {
		group, _ := g.(map[string]any)
		matcher, _ := group["matcher"].(string)
		seen[matcher] = true
	}
	for _, want := range []string{"Bash", "Write"} {
		if !seen[want] {
			t.Errorf("matcher %q missing from the union: %#v", want, groups)
		}
	}
}
