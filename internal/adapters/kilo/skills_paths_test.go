package kilo

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

// Kilo Code scans `.kilo/skills`, `.agents/skills`, and `.claude/skills`
// without configuration. A skills dir outside all three needs a
// `skills.paths` entry in kilo.jsonc, or sync writes the folders where
// nothing reads them (target-audit 2026-09-18, #861).
func TestEmit_SkillsDirOutsideScannedTrees_ListsSkillsPaths(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{Outputs: map[string]config.Output{"kilo": {SkillsDir: "docs/team-skills"}}}
	entries := []spec.Entry{{Kind: spec.KindSkill, Name: "demo", Body: "skill body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "docs/team-skills/demo/SKILL.md")); err != nil {
		t.Fatalf("expected docs/team-skills/demo/SKILL.md: %v", err)
	}
	got := readFile(t, filepath.Join(dir, "kilo.jsonc"))
	for _, want := range []string{`"skills"`, `"paths"`, `"docs/team-skills"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// A skills dir Kilo Code already scans needs no config entry, so
// kilo.jsonc keeps no `skills` key for those trees.
func TestEmit_SkillsDirInsideScannedTrees_NoSkillsKey(t *testing.T) {
	for _, skillsDir := range []string{"", ".kilo/skills", ".claude/skills"} {
		t.Run(skillsDir, func(t *testing.T) {
			dir := testutil.TempCwd(t)

			cfg := &config.Config{Outputs: map[string]config.Output{"kilo": {SkillsDir: skillsDir}}}
			entries := []spec.Entry{{Kind: spec.KindSkill, Name: "demo", Body: "skill body"}}
			if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "kilo.jsonc"))
			if os.IsNotExist(err) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), `"skills"`) {
				t.Errorf("unexpected skills key for a scanned tree:\n%s", data)
			}
		})
	}
}

// The merge replaces a whole array, so a user's own `skills.paths`
// entries have to survive the managed one being added, the way every
// other user-authored kilo.jsonc key survives (#725).
func TestEmit_SkillsPaths_PreservesUserEntries(t *testing.T) {
	dir := testutil.TempCwd(t)

	existing := `{
  // team config
  "skills": {"paths": ["vendor/skills"], "urls": ["https://example.com/.well-known/skills/"]}
}`
	if err := os.WriteFile(filepath.Join(dir, "kilo.jsonc"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Outputs: map[string]config.Output{"kilo": {SkillsDir: "docs/team-skills"}}}
	entries := []spec.Entry{{Kind: spec.KindSkill, Name: "demo", Body: "skill body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "kilo.jsonc"))
	for _, want := range []string{`"vendor/skills"`, `"docs/team-skills"`, "https://example.com/.well-known/skills/"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// A second sync must not stack duplicate copies of the managed path.
func TestEmit_SkillsPaths_IdempotentAcrossSyncs(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{Outputs: map[string]config.Output{"kilo": {SkillsDir: "docs/team-skills"}}}
	entries := []spec.Entry{{Kind: spec.KindSkill, Name: "demo", Body: "skill body"}}
	for range 2 {
		if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
			t.Fatal(err)
		}
	}
	got := readFile(t, filepath.Join(dir, "kilo.jsonc"))
	if n := strings.Count(got, `"docs/team-skills"`); n != 1 {
		t.Errorf("managed path listed %d times, want 1:\n%s", n, got)
	}
}

// No skill spec means no folders to point at, so a custom skills dir
// alone never writes a surprise key.
func TestEmit_SkillsDirOverrideWithoutSkills_NoSkillsKey(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{Outputs: map[string]config.Output{"kilo": {SkillsDir: "docs/team-skills"}}}
	entries := []spec.Entry{{Kind: spec.KindRule, Name: "r1", Body: "rule body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readFile(t, filepath.Join(dir, "kilo.jsonc")), `"skills"`) {
		t.Error("skills key written without any skill spec")
	}
}
