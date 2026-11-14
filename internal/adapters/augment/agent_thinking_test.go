package augment

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func emitOneAgent(t *testing.T, meta map[string]any) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "rev", Meta: meta, Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	return readFile(t, filepath.Join(dir, ".augment/agents/rev.md"))
}

func TestEmit_Agent_EffortBecomesThinking(t *testing.T) {
	for _, level := range effortLevels {
		t.Run(level, func(t *testing.T) {
			got := emitOneAgent(t, map[string]any{"description": "r", "effort": level})
			want := "thinking: " + level + "\n"
			if level == "off" {
				want = "thinking: \"off\"\n"
			}
			if !strings.Contains(got, want) {
				t.Errorf("want %q in:\n%s", want, got)
			}
			if strings.Contains(got, "effort:") {
				t.Errorf("effort must not be written:\n%s", got)
			}
			if n := emit.PendingCoverageNotesCount(); n != 0 {
				t.Errorf("a valid level needs no note, got %d", n)
			}
		})
	}
}

func TestEmit_Agent_EffortOutsideLevelsKeepsNote(t *testing.T) {
	for _, value := range []any{"max", 8000} {
		got := emitOneAgent(t, map[string]any{"description": "r", "effort": value})
		if strings.Contains(got, "thinking:") || strings.Contains(got, "effort:") {
			t.Errorf("effort %v must not be written:\n%s", value, got)
		}
		if n := emit.PendingCoverageNotesCount(); n != 1 {
			t.Errorf("effort %v: want one note, got %d", value, n)
		}
	}
}

func TestEmit_Agent_XAugmentThinkingWinsOverEffort(t *testing.T) {
	got := emitOneAgent(t, map[string]any{
		"description": "r", "effort": "high",
		"x-augment": map[string]any{"thinking": "low"},
	})
	if strings.Count(got, "thinking:") != 1 || !strings.Contains(got, "thinking: low\n") {
		t.Errorf("x-augment.thinking must win, once:\n%s", got)
	}
	if n := emit.PendingCoverageNotesCount(); n != 0 {
		t.Errorf("want no note, got %d", n)
	}
}
