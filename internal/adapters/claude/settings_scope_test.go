package claude

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_NotesSettingsKeysProjectScopeIgnores(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{settingsEntry(map[string]any{"x-claude": map[string]any{
		"autoMode":              map[string]any{"soft_deny": []any{"x"}},
		"bashEditDiffEnabled":   true,
		"useAutoModeDuringPlan": false,
		"deniedModels":          []any{"opus"},
		"sandbox":               map[string]any{"allowNestedUserNamespaces": true, "enabled": true},
		"outputStyle":           "Explanatory",
	}})}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	out := buf.String()

	for _, want := range []string{
		"`x-claude.autoMode` on 1 settings has no effect on claude (" + userOrManagedReason + ")",
		"`x-claude.bashEditDiffEnabled: true` on 1 settings has no effect on claude (" + userOrManagedReason + ")",
		"`x-claude.useAutoModeDuringPlan: false` on 1 settings has no effect on claude (" + userOrManagedReason + ")",
		"`x-claude.sandbox.allowNestedUserNamespaces: true` on 1 settings has no effect on claude (" + userOrManagedReason + ")",
		"`x-claude.deniedModels` on 1 settings has no effect on claude (" + managedOnlyReason + ")",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing note %q in:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"outputStyle", "sandbox.enabled"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("note names %s, which project settings can set:\n%s", unwanted, out)
		}
	}

	raw, err := os.ReadFile(".claude/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"autoMode"`, `"useAutoModeDuringPlan"`, `"deniedModels"`, `"allowNestedUserNamespaces"`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("sync must keep writing %s: %s", key, raw)
		}
	}
}

func TestEmit_NoNoteForValuesProjectScopeHonors(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{settingsEntry(map[string]any{"x-claude": map[string]any{
		"useAutoModeDuringPlan": true,
		"bashEditDiffEnabled":   false,
		"sandbox":               map[string]any{"allowNestedUserNamespaces": false},
	}})}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if out := buf.String(); strings.Contains(out, "x-claude.") {
		t.Errorf("unexpected note:\n%s", out)
	}
}

func TestProjectIgnoredSettings_CountsEachSpec(t *testing.T) {
	entries := []spec.Entry{
		settingsEntry(map[string]any{"x-claude": map[string]any{"modelPicker": map[string]any{}}}),
		settingsEntry(map[string]any{"x-claude": map[string]any{"modelPicker": map[string]any{}}}),
		settingsEntry(map[string]any{"model": "opus"}),
	}
	got := projectIgnoredSettings(entries, emit.SettingsCustomKeys(entries, target))
	if len(got) != 1 || got[0].field != "x-claude.modelPicker" || got[0].count != 2 {
		t.Fatalf("got %+v", got)
	}
}

func TestProjectIgnoredSettings_NamesOnlyTheMergedValue(t *testing.T) {
	entries := []spec.Entry{
		settingsEntry(map[string]any{"x-claude": map[string]any{
			"useAutoModeDuringPlan": false,
			"sandbox":               map[string]any{"allowNestedUserNamespaces": true},
		}}),
		settingsEntry(map[string]any{"x-claude": map[string]any{
			"useAutoModeDuringPlan": true,
			"sandbox":               map[string]any{"enabled": true},
		}}),
	}
	if got := projectIgnoredSettings(entries, emit.SettingsCustomKeys(entries, target)); len(got) != 0 {
		t.Fatalf("a later spec replaced both values, got %+v", got)
	}
}

func TestProjectIgnoredSettings_WorktreeLocationIsUserOnly(t *testing.T) {
	entries := []spec.Entry{settingsEntry(map[string]any{"x-claude": map[string]any{
		"worktree": map[string]any{"location": "~/wt", "baseRef": "main"},
	}})}
	got := projectIgnoredSettings(entries, emit.SettingsCustomKeys(entries, target))
	if len(got) != 1 || got[0].field != "x-claude.worktree.location" || got[0].reason != userOnlyReason {
		t.Fatalf("got %+v", got)
	}
}

func TestEmit_AutoContinueProjectSettingExplainsFallback(t *testing.T) {
	for _, tc := range []struct {
		name     string
		custom   map[string]any
		wantNote bool
	}{
		{name: "unset", custom: map[string]any{}},
		{name: "true", custom: map[string]any{"autoContinueAtUsageLimit": true}, wantNote: true},
		{name: "false", custom: map[string]any{"autoContinueAtUsageLimit": false}, wantNote: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			buf := swapNoteWarner(t)
			entries := []spec.Entry{settingsEntry(map[string]any{"x-claude": tc.custom})}
			if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
				t.Fatal(err)
			}
			emit.FlushCoverageNotes()
			note := buf.String()
			if !tc.wantNote {
				if strings.Contains(note, "autoContinueAtUsageLimit") {
					t.Errorf("unexpected note: %s", note)
				}
				return
			}
			for _, want := range []string{"autoContinueAtUsageLimit", "turns automatic continuation off", "user settings, --settings, and managed settings leave it unset"} {
				if !strings.Contains(note, want) {
					t.Errorf("note lacks %q: %s", want, note)
				}
			}
			if strings.Contains(note, "has no effect") || strings.Contains(note, "ignores it") {
				t.Errorf("a restrictive project setting is not inert: %s", note)
			}
			raw, err := os.ReadFile(".claude/settings.json")
			if err != nil {
				t.Fatal(err)
			}
			var settings map[string]any
			if err := json.Unmarshal(raw, &settings); err != nil {
				t.Fatal(err)
			}
			if got := settings["autoContinueAtUsageLimit"]; got != tc.custom["autoContinueAtUsageLimit"] {
				t.Errorf("emitted value = %v, want %v", got, tc.custom["autoContinueAtUsageLimit"])
			}
		})
	}
}
