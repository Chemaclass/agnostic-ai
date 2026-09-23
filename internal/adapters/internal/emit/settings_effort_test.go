package emit

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func settingsWithEffort(effort ...any) []spec.Entry {
	var out []spec.Entry
	for _, e := range effort {
		out = append(out, spec.Entry{Kind: spec.KindSettings, Name: "s", Meta: map[string]any{"effort": e}})
	}
	return out
}

func TestSettingsEffort_ResolvesPerTargetThenDefaultAndLastWins(t *testing.T) {
	entries := settingsWithEffort("low", map[string]any{"claude": "xhigh", "default": "high"})
	if got := SettingsEffort(entries, "claude"); got != "xhigh" {
		t.Errorf("claude = %v, want xhigh", got)
	}
	if got := SettingsEffort(entries, "copilot"); got != "high" {
		t.Errorf("copilot = %v, want the map default high", got)
	}
	if got := SettingsEffort(settingsWithEffort("low", map[string]any{"claude": "xhigh"}), "copilot"); got != "low" {
		t.Errorf("a later spec with nothing for copilot must not erase the earlier value, got %v", got)
	}
	if got := SettingsEffort(nil, "claude"); got != nil {
		t.Errorf("no settings = %v, want nil", got)
	}
}

func TestSettingsEffortLevel_NotesAValueTheTargetRejects(t *testing.T) {
	buf := swapWarner(t)
	ResetCoverageNotes()
	t.Cleanup(ResetCoverageNotes)
	levels := []string{"low", "medium", "high", "xhigh"}
	if got := SettingsEffortLevel(settingsWithEffort("high"), "claude", levels); got != "high" {
		t.Errorf("accepted level = %q, want high", got)
	}
	if got := SettingsEffortLevel(settingsWithEffort("max"), "claude", levels); got != "" {
		t.Errorf("rejected level = %q, want empty", got)
	}
	if got := SettingsEffortLevel(settingsWithEffort("anything"), "codex", nil); got != "anything" {
		t.Errorf("an open value set accepts any string, got %q", got)
	}
	FlushCoverageNotes()
	if !strings.Contains(buf.String(), "`effort` on 1 settings has no effect on claude") || !strings.Contains(buf.String(), "max") {
		t.Errorf("expected one note naming max for claude, got:\n%s", buf.String())
	}
}

func TestReportUnsupported_NotesSettingsEffortWithoutRepoKey(t *testing.T) {
	buf := swapWarner(t)
	ResetCoverageNotes()
	t.Cleanup(ResetCoverageNotes)
	b := spec.Bundle{Settings: settingsWithEffort("high")}
	for _, c := range []Capabilities{
		{Target: "gemini", Supports: []spec.Kind{spec.KindSettings}},
		{Target: "claude", Supports: []spec.Kind{spec.KindSettings}, SettingsFields: []string{"effort"}},
	} {
		if err := ReportUnsupported(c, b, OnUnsupportedWarn); err != nil {
			t.Fatal(err)
		}
	}
	FlushCoverageNotes()
	got := buf.String()
	if !strings.Contains(got, "has no effect on gemini (the settings file has no repository effort key)") {
		t.Errorf("gemini must report the dropped effort, got:\n%s", got)
	}
	if strings.Contains(got, "on claude") {
		t.Errorf("claude carries effort, got:\n%s", got)
	}
}
