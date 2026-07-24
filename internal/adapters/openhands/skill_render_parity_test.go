package openhands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestSkillMarkdown_MatchesSharedTreeTargets asserts that the shared
// SKILL.md renderer (emit.SkillMarkdown), which every adapter writing
// into the `.agents/skills/` tree calls, produces byte-identical output
// for openhands, crush, codex, amp, and zed given the same entry. The
// `target` argument only changes output when the entry carries a
// `x-<target>` override or a per-target `model` map; a plain entry
// (no such keys) must render the same regardless of which of those
// targets asks, or the shared-tree dedupe in `sync.shared-skills`
// would silently stop working the moment two targets' bytes drift.
//
// This test deliberately calls emit.SkillMarkdown directly rather than
// importing the crush or codex adapter packages: no-cross-adapter-
// imports forbids one adapter package importing another, even in
// tests.
func TestSkillMarkdown_MatchesSharedTreeTargets(t *testing.T) {
	entry := spec.Entry{
		Kind: spec.KindSkill,
		Name: "uno",
		Meta: map[string]any{"description": "Uno skill description."},
		Body: "uno skill body",
	}

	want := emit.SkillMarkdown(entry, "openhands")
	for _, target := range []string{"crush", "codex", "amp", "zed"} {
		if got := emit.SkillMarkdown(entry, target); got != want {
			t.Errorf("SkillMarkdown(entry, %q) diverged from openhands:\n--- openhands ---\n%s\n--- %s ---\n%s",
				target, want, target, got)
		}
	}
}

// TestKitSink_SkillOutputMatchesCrushGolden confirms the openhands
// SKILL.md render is byte-identical to the checked-in crush golden
// fixture for the same kit-sink skill entries, so the on-disk
// shared-tree dedupe (sync.shared-skills) actually finds the two
// trees identical rather than relying solely on the in-memory renderer
// check above. Reads crush's testdata directly off disk (a fixture
// read, not a Go import) so this stays within no-cross-adapter-imports.
func TestKitSink_SkillOutputMatchesCrushGolden(t *testing.T) {
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	crushSkillsDir := filepath.Join(origCwd, "..", "crush", "testdata", "kitsink", ".agents", "skills")
	if _, err := os.Stat(crushSkillsDir); err != nil {
		t.Skipf("crush golden fixtures not found at %s: %v", crushSkillsDir, err)
	}

	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	for _, name := range []string{"uno", "dos", "tres"} {
		gotPath := filepath.Join(dir, ".agents", "skills", name, "SKILL.md")
		got, err := os.ReadFile(gotPath)
		if err != nil {
			t.Fatalf("read openhands output %s: %v", gotPath, err)
		}
		wantPath := filepath.Join(crushSkillsDir, name, "SKILL.md")
		want, err := os.ReadFile(wantPath)
		if err != nil {
			t.Fatalf("read crush golden %s: %v", wantPath, err)
		}
		if string(got) != string(want) {
			t.Errorf("openhands SKILL.md for %q diverged from crush golden:\n--- crush (%s) ---\n%s\n--- openhands ---\n%s",
				name, wantPath, want, got)
		}
	}
}
