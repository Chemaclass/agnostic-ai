package claude

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Cursor runs .claude/settings.json hooks and reads only `command`, so an
// exec-form hook runs there as a bare interpreter.
func TestEmit_NotesCursorRunsExecFormHooksWithoutArgs(t *testing.T) {
	for _, tc := range []struct {
		targets []string
		want    bool
	}{
		{[]string{"claude", "cursor"}, true},
		{[]string{"claude", "codex"}, false},
	} {
		testutil.TempCwd(t)
		warnings := &strings.Builder{}
		prev := emit.Warner
		emit.Warner = warnings
		emit.ResetCoverageNotes()
		entries := []spec.Entry{
			{Kind: spec.KindHook, Name: "a", Meta: map[string]any{"event": "PreToolUse", "command": "node", "args": []any{"a.js"}}},
			{Kind: spec.KindHook, Name: "b", Meta: map[string]any{"event": "Stop", "command": "node", "args": []any{"b.js"}}},
		}
		err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{Targets: tc.targets}, false)
		emit.FlushCoverageNotes()
		emit.Warner = prev
		if err != nil {
			t.Fatal(err)
		}
		got := warnings.String()
		if has := strings.Contains(got, "Cursor runs .claude/settings.json hooks without args"); has != tc.want {
			t.Errorf("targets %v: note present = %v, want %v:\n%s", tc.targets, has, tc.want, got)
		}
		if tc.want && strings.Count(got, "without args") != 1 {
			t.Errorf("want one note per run, got:\n%s", got)
		}
	}
}
