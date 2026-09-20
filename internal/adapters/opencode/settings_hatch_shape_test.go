package opencode

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// `x-opencode.permission` is excluded from the general merge because
// buildPermissions reads it itself, and it read a value of the wrong
// shape as absent. The author's policy did nothing, no note fired, and
// the translated rules shipped in its place (#976).
func TestEmit_SettingsCustomPermissionWrongShapeNotes(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)
	entries := []spec.Entry{{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{
		"permissions": map[string]any{"deny": []any{"Bash(rm:*)"}},
		// OpenCode's own key takes a map of tool name to rule.
		"x-opencode": map[string]any{"permission": []any{"bash"}},
	}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	note := buf.String()
	for _, want := range []string{"x-opencode.permission", "opencode", "an object"} {
		if !strings.Contains(note, want) {
			t.Errorf("expected the note to mention %q, got: %s", want, note)
		}
	}
	permission, _ := projectConfig(t, dir)["permission"].(map[string]any)
	bash, _ := permission["bash"].(map[string]any)
	if bash["rm *"] != "deny" {
		t.Errorf("permission = %#v, want the translated deny rule", permission)
	}
}
