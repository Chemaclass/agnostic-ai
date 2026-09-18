package antigravity

import (
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

// Antigravity documents four rule activation modes in prose (Manual,
// Always on, Model decision, Glob pattern) and names no frontmatter key,
// no file format, and no example for any of them
// (antigravity.google/docs/rules-workflows?tab=ide). So a rule asking
// for anything other than always-on emits as a bare file and turns
// always-on. Guessing Windsurf's `trigger:`/`globs:` spelling on the
// lineage resemblance would write a key no vendor page backs, so the
// drop stays, and this test holds it loud (#865).
func TestEmit_Rule_ActivationNotesFieldNoOp(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "py-style", Body: "body", Meta: map[string]any{
			"globs":       "**/*.py",
			"alwaysApply": false,
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, ".agents", "rules", "py-style.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "trigger:") || strings.Contains(string(raw), "globs:") {
		t.Errorf("no vendor page names a rule activation key; must not guess one: %s", raw)
	}

	emit.FlushCoverageNotes()
	out := buf.String()
	if !strings.Contains(out, "`globs` on 1 rule has no effect on antigravity") {
		t.Errorf("expected a globs field no-op note, got: %s", out)
	}
	if !strings.Contains(out, "`alwaysApply` on 1 rule has no effect on antigravity") {
		t.Errorf("expected an alwaysApply field no-op note, got: %s", out)
	}
}

// An always-on rule loses nothing, because a bare file is already
// always-on. Noting it would train users to ignore the note.
func TestEmit_Rule_AlwaysOnNotesNothing(t *testing.T) {
	testutil.TempCwd(t)
	swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "house", Body: "body"},
		{Kind: spec.KindRule, Name: "always", Body: "body", Meta: map[string]any{"alwaysApply": true}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if n := emit.PendingCoverageNotesCount(); n != 0 {
		t.Errorf("expected no coverage note for always-on rules, got %d", n)
	}
}

// `x-antigravity` overrides the generic frontmatter, so a rule that
// clears its globs there has nothing left to lose.
func TestEmit_Rule_TargetOverrideClearsGlobs(t *testing.T) {
	testutil.TempCwd(t)
	swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "py-style", Body: "body", Meta: map[string]any{
			"globs":          "**/*.py",
			"x-antigravity":  map[string]any{"globs": ""},
			"someOtherThing": 1,
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if n := emit.PendingCoverageNotesCount(); n != 0 {
		t.Errorf("expected no coverage note when the target override clears globs, got %d", n)
	}
}
