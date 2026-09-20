package windsurf

import (
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// `x-windsurf.permissions` is excluded from the general merge because
// entryRules reads it itself, and it read a value of the wrong shape
// as absent. The author's policy did nothing, no note fired, and the
// translated rules shipped in its place (#976).
func TestEmit_SettingsCustomPermissionsWrongShapeNotes(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })

	entries := []spec.Entry{{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{
		"permissions": map[string]any{"deny": []any{"Bash(rm:*)"}},
		// Devin's own key takes an object of three rule lists.
		"x-windsurf": map[string]any{"permissions": []any{"Exec(rm)"}},
	}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	note := buf.String()
	for _, want := range []string{"x-windsurf.permissions", "windsurf", "an object"} {
		if !strings.Contains(note, want) {
			t.Errorf("expected the note to mention %q, got: %s", want, note)
		}
	}
	permissions, _ := settingsDoc(t, dir)["permissions"].(map[string]any)
	if want := []any{"Exec(rm)"}; !reflect.DeepEqual(permissions["deny"], want) {
		t.Errorf("deny = %#v, want the translated rule %#v", permissions["deny"], want)
	}
}
