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

func TestEmit_NotesSettingsKeysProjectScopeIgnores(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{settingsEntry(map[string]any{"x-claude": map[string]any{
		"autoMode":              map[string]any{"soft_deny": []any{"x"}},
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
	got := projectIgnoredSettings(entries)
	if len(got) != 1 || got[0].field != "x-claude.modelPicker" || got[0].count != 2 {
		t.Fatalf("got %+v", got)
	}
}
