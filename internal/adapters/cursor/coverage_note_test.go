package cursor

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

// A skill that bundles a non-markdown payload cannot be represented in
// Cursor's flat .mdc rule, so the payload is dropped. The user must be
// told instead of losing the files silently (#430).
func TestEmit_NotesGapWhenSkillBundlesAssets(t *testing.T) {
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
	if err := New().Emit(b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	emit.FlushCoverageNotes()
	want := "1 skill reaches cursor only in the source dir"
	if !strings.Contains(buf.String(), want) {
		t.Errorf("expected coverage note %q, got: %s", want, buf.String())
	}
}

// A skill with no sibling payload loses nothing on Cursor, so no note fires.
func TestEmit_NoGapWhenSkillHasNoAssets(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	skillDir := filepath.Join(dir, "skills", "alpha")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}

	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindSkill, Name: "alpha", Path: filepath.Join(skillDir, "SKILL.md"), Body: "body"},
	})
	if err := New().Emit(b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	if got := emit.PendingCoverageNotesCount(); got != 0 {
		t.Errorf("skill without assets must buffer no note, count=%d", got)
	}
	emit.FlushCoverageNotes()
	if buf.Len() != 0 {
		t.Errorf("expected no output, got: %s", buf.String())
	}
}
