package crush

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

func TestEmit_HooksWriteReleasedLifecycleEvents(t *testing.T) {
	dir := testutil.TempCwd(t)
	events := []string{
		"PreToolUse", "PostToolUse", "PostToolUseFailure", "PermissionRequest", "PermissionResult",
		"SessionStart", "SessionEnd", "TurnStart", "TurnEnd", "StopFailure", "Interrupt", "PreCompact", "PostCompact",
	}
	var entries []spec.Entry
	for _, event := range events {
		entries = append(entries, spec.Entry{Kind: spec.KindHook, Name: strings.ToLower(event), Meta: map[string]any{
			"event": event, "command": "echo " + event,
		}})
	}
	entries = append(entries,
		spec.Entry{Kind: spec.KindHook, Name: "snake", Meta: map[string]any{"event": "session_start", "command": "echo snake"}},
		spec.Entry{Kind: spec.KindHook, Name: "mixed", Meta: map[string]any{"event": "tUrN_eNd", "command": "echo mixed"}},
	)
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Hooks map[string][]map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "crush.json"))), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Hooks) != len(events) {
		t.Errorf("got %d native events, want %d", len(doc.Hooks), len(events))
	}
	for _, event := range events {
		if got := doc.Hooks[event]; len(got) == 0 || got[0]["command"] != "export AGNOSTIC_AI_TARGET=crush; echo "+event {
			t.Errorf("%s: missing native hook: %v", event, got)
		}
	}
	for _, event := range []string{"SessionStart", "TurnEnd"} {
		if len(doc.Hooks[event]) != 2 {
			t.Errorf("%s: alias did not join canonical event: %v", event, doc.Hooks[event])
		}
	}
}

func TestEmit_HooksRejectMatchersOnEventsWithoutTools(t *testing.T) {
	for _, event := range []string{"SessionStart", "SessionEnd", "TurnStart", "TurnEnd", "StopFailure", "Interrupt", "PreCompact", "PostCompact"} {
		t.Run(event, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			path := filepath.Join(dir, "crush.json")
			const original = "{\"options\":{\"debug\":true}}\n"
			if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
				t.Fatal(err)
			}
			entry := spec.Entry{Kind: spec.KindHook, Name: "notify", Path: "hooks/notify.yaml", Meta: map[string]any{
				"event": event, "matcher": "^bash$", "command": "echo audit",
			}}
			err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false)
			if err == nil || !strings.Contains(err.Error(), "hooks/notify.yaml") || !strings.Contains(err.Error(), event) || !strings.Contains(err.Error(), "matcher") {
				t.Errorf("expected a source-specific matcher error for %s, got %v", event, err)
			}
			if got := readFile(t, path); got != original {
				t.Errorf("invalid hook changed existing config: %s", got)
			}
		})
	}
}

func TestEmit_HooksKeepMatchersOnToolEvents(t *testing.T) {
	for _, event := range []string{"PreToolUse", "PostToolUse", "PostToolUseFailure", "PermissionRequest", "PermissionResult"} {
		t.Run(event, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			entry := spec.Entry{Kind: spec.KindHook, Name: "audit", Meta: map[string]any{
				"event": event, "matcher": "^bash$", "command": "echo audit",
			}}
			if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false); err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, filepath.Join(dir, "crush.json")); !strings.Contains(got, `"matcher": "^bash$"`) {
				t.Errorf("%s lost its matcher: %s", event, got)
			}
		})
	}
}
