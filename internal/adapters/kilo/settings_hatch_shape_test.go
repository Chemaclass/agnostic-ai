package kilo

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// `x-kilo.permission` is excluded from the general merge because
// settingsPermission reads it itself, and it read a value of the wrong
// shape as absent. The author's policy did nothing, no note fired, and
// the translated rules shipped in its place (#976).
func TestEmit_SettingsCustomPermissionWrongShapeNotes(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })

	entries := []spec.Entry{{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{
		"permissions": map[string]any{"deny": []any{"Bash(rm:*)"}},
		// Kilo's own key takes a map of tool name to rule.
		"x-kilo": map[string]any{"permission": []any{"bash"}},
	}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	note := buf.String()
	for _, want := range []string{"x-kilo.permission", "kilo", "an object"} {
		if !strings.Contains(note, want) {
			t.Errorf("expected the note to mention %q, got: %s", want, note)
		}
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "kilo.jsonc"))), &got); err != nil {
		t.Fatal(err)
	}
	permission, _ := got["permission"].(map[string]any)
	bash, _ := permission["bash"].(map[string]any)
	if bash["rm *"] != "deny" {
		t.Errorf("permission = %#v, want the translated deny rule", got["permission"])
	}
}
