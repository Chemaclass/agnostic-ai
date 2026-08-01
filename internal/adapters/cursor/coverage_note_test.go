package cursor

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

func swapNoteWarner(t *testing.T) *strings.Builder {
	t.Helper()
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	return buf
}

// Cursor discovers native skill folders, so a skill's bundled payload
// propagates byte-for-byte instead of being dropped with a coverage
// note (the pre-native behavior from #430).
func TestEmit_SkillBundledAssetsPropagate(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	skillDir := filepath.Join(dir, "skills", "alpha")
	if err := os.MkdirAll(filepath.Join(skillDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "scripts", "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindSkill, Name: "alpha", Path: filepath.Join(skillDir, "SKILL.md"), Body: "body"},
	})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".cursor", "skills", "alpha", "scripts", "run.sh"))
	if err != nil {
		t.Fatalf("bundled asset must propagate to the native skill folder: %v", err)
	}
	if string(got) != "#!/bin/sh\n" {
		t.Errorf("asset not byte-identical: %q", got)
	}

	if n := emit.PendingCoverageNotesCount(); n != 0 {
		t.Errorf("native skill emission must buffer no coverage note, count=%d", n)
	}
	emit.FlushCoverageNotes()
	if buf.Len() != 0 {
		t.Errorf("expected no output, got: %s", buf.String())
	}
}

// Regression for the target-audit finding: cursor.com/docs/mcp documents
// no `disabled` (or `enabled`) key anywhere in its MCP server schema —
// only a sidebar UI toggle ("Disabled servers won't load or appear in
// chat"). Writing `disabled: true` into `.cursor/mcp.json` would let a
// user believe the server stopped connecting when Cursor ignores the key
// and keeps using it, so the field must not reach the file, and the drop
// must be loud rather than silent.
func TestEmit_MCP_DisabledHasNoFileBasedEffect(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx", "disabled": true}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".cursor", "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	fs := parsed["mcpServers"].(map[string]any)["fs"].(map[string]any)
	if _, ok := fs["disabled"]; ok {
		t.Errorf("cursor has no file-based disable key; must not emit one: %s", raw)
	}

	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "`disabled` on 1 mcp has no effect on cursor") {
		t.Errorf("expected a field no-op note, got: %s", buf.String())
	}
}
