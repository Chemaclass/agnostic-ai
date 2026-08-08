package warp

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
// for warp, codex, amp, zed, crush, and openhands given the same entry
// (target-audit 2026-08-08: verifying this was a precondition for
// pointing warp's default skills-dir at the shared tree instead of
// leaving skills unreached, #557). The `target` argument only changes
// output when the entry carries an `x-<target>` override; a plain entry
// (no such keys) must render the same regardless of which of those
// targets asks, or the shared-tree dedupe in `sync.shared-skills` would
// silently stop working the moment two targets' bytes drift.
//
// This test deliberately calls emit.SkillMarkdown directly rather than
// importing another adapter package: no-cross-adapter-imports forbids
// one adapter package importing another, even in tests.
func TestSkillMarkdown_MatchesSharedTreeTargets(t *testing.T) {
	entry := spec.Entry{
		Kind: spec.KindSkill,
		Name: "uno",
		Meta: map[string]any{"description": "Uno skill description."},
		Body: "uno skill body",
	}

	want := emit.SkillMarkdown(entry, "warp")
	for _, other := range []string{"codex", "amp", "zed", "crush", "openhands"} {
		if got := emit.SkillMarkdown(entry, other); got != want {
			t.Errorf("SkillMarkdown(entry, %q) diverged from warp:\n--- warp ---\n%s\n--- %s ---\n%s",
				other, want, other, got)
		}
	}
}

// TestKitSink_SkillOutputMatchesZedGolden confirms the warp SKILL.md
// render is byte-identical to the checked-in zed golden fixture for the
// same kit-sink skill entries, so the on-disk shared-tree dedupe
// (sync.shared-skills) actually finds the two trees identical rather
// than relying solely on the in-memory renderer check above. Reads
// zed's testdata directly off disk (a fixture read, not a Go import) so
// this stays within no-cross-adapter-imports.
func TestKitSink_SkillOutputMatchesZedGolden(t *testing.T) {
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	zedSkillsDir := filepath.Join(origCwd, "..", "zed", "testdata", "kitsink", ".agents", "skills")
	if _, err := os.Stat(zedSkillsDir); err != nil {
		t.Skipf("zed golden fixtures not found at %s: %v", zedSkillsDir, err)
	}

	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	for _, name := range []string{"uno", "dos", "tres"} {
		gotPath := filepath.Join(dir, ".agents", "skills", name, "SKILL.md")
		got, err := os.ReadFile(gotPath)
		if err != nil {
			t.Fatalf("read warp output %s: %v", gotPath, err)
		}
		wantPath := filepath.Join(zedSkillsDir, name, "SKILL.md")
		want, err := os.ReadFile(wantPath)
		if err != nil {
			t.Fatalf("read zed golden %s: %v", wantPath, err)
		}
		if string(got) != string(want) {
			t.Errorf("warp SKILL.md for %q diverged from zed golden:\n--- zed (%s) ---\n%s\n--- warp ---\n%s",
				name, wantPath, want, got)
		}
	}
}
