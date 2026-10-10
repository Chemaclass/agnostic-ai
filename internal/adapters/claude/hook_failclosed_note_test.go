package claude

import (
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const (
	failClosedEventsReason = "Stop, SubagentStop, TaskCompleted, and TeammateIdle"
	failClosedAsyncReason  = "async or asyncRewake"
)

func emitFailClosedHook(t *testing.T, meta map[string]any) (notes, settings string) {
	t.Helper()
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)
	entries := []spec.Entry{{Kind: spec.KindHook, Name: "gate", Meta: meta}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	data, err := os.ReadFile(".claude/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	return buf.String(), string(data)
}

func TestEmit_FailClosedWithoutEffectNotesAndStillWritesOnFailure(t *testing.T) {
	cases := []struct {
		name string
		meta map[string]any
		want string
	}{
		{"Stop", map[string]any{"event": "Stop", "command": "gate.sh"}, failClosedEventsReason},
		{"SubagentStop", map[string]any{"event": "SubagentStop", "command": "gate.sh"}, failClosedEventsReason},
		{"TaskCompleted", map[string]any{"event": "TaskCompleted", "command": "gate.sh"}, failClosedEventsReason},
		{"TeammateIdle", map[string]any{"event": "TeammateIdle", "command": "gate.sh"}, failClosedEventsReason},
		{"async", map[string]any{"event": "PreToolUse", "command": "gate.sh", "async": true}, failClosedAsyncReason},
		{"asyncRewake", map[string]any{"event": "PreToolUse", "command": "gate.sh", "asyncRewake": true}, failClosedAsyncReason},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.meta["failClosed"] = true
			notes, settings := emitFailClosedHook(t, c.meta)
			if !strings.Contains(notes, "`failClosed` on 1 hook has no effect on claude") || !strings.Contains(notes, c.want) {
				t.Errorf("failClosed note missing %q, got %q", c.want, notes)
			}
			if !strings.Contains(settings, `"onFailure": "block"`) {
				t.Errorf("onFailure block not written: %s", settings)
			}
		})
	}
}

func TestEmit_FailClosedWhereItBlocksStaysQuiet(t *testing.T) {
	cases := []struct {
		name string
		meta map[string]any
	}{
		{"PreToolUse", map[string]any{"event": "PreToolUse", "failClosed": true}},
		{"PermissionRequest denies on failure", map[string]any{"event": "PermissionRequest", "failClosed": true}},
		{"async false", map[string]any{"event": "PreToolUse", "failClosed": true, "async": false}},
		{"failClosed false on Stop", map[string]any{"event": "Stop", "failClosed": false}},
		{"async without failClosed", map[string]any{"event": "PreToolUse", "async": true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.meta["command"] = "gate.sh"
			notes, _ := emitFailClosedHook(t, c.meta)
			if strings.Contains(notes, "failClosed") {
				t.Errorf("unexpected failClosed note: %q", notes)
			}
		})
	}
}
