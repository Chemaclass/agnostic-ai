package emit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestFolderBasedSkill(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"folder skill", filepath.Join("skills", "alpha", "SKILL.md"), true},
		{"flat skill", filepath.Join("skills", "alpha.md"), false},
		{"in-memory spec", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FolderBasedSkill(spec.Entry{Name: "alpha", Path: tc.path}); got != tc.want {
				t.Errorf("FolderBasedSkill(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// A flat-file skill shares the skills/ directory with its siblings, so
// propagation must copy nothing — otherwise every sibling skill body leaks
// into this skill's folder (#387).
func TestPropagateSkillAssets_FlatFileSkipsSiblings(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha.md", "beta.md", "gamma.md"} {
		if err := os.WriteFile(filepath.Join(skillsDir, name), []byte("---\nname: x\n---\nbody\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	s := spec.Entry{Kind: spec.KindSkill, Name: "alpha", Path: filepath.Join(skillsDir, "alpha.md")}
	dst := filepath.Join(dir, "out", "alpha")
	if err := sess.PropagateSkillAssets(s, dst, skipNothing, false); err != nil {
		t.Fatal(err)
	}

	if entries, err := os.ReadDir(dst); err == nil && len(entries) > 0 {
		t.Errorf("flat-file skill leaked %d sibling files into %s", len(entries), dst)
	}
}

// A folder-based skill owns its directory, so every sibling asset (minus the
// re-rendered SKILL.md) propagates into the emitted folder.
func TestPropagateSkillAssets_FolderCopiesSiblings(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	srcSkill := filepath.Join(dir, "skills", "alpha")
	if err := os.MkdirAll(filepath.Join(srcSkill, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcSkill, "SKILL.md"), []byte("---\nname: alpha\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcSkill, "scripts", "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	s := spec.Entry{Kind: spec.KindSkill, Name: "alpha", Path: filepath.Join(srcSkill, "SKILL.md")}
	dst := filepath.Join(dir, "out", "alpha")
	skip := func(rel string) bool { return rel == "SKILL.md" }
	if err := sess.PropagateSkillAssets(s, dst, skip, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dst, "scripts", "run.sh")); err != nil {
		t.Errorf("folder skill did not propagate sibling asset: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "SKILL.md")); err == nil {
		t.Errorf("skip predicate ignored: SKILL.md was copied")
	}
}

func skipNothing(string) bool { return false }

func TestSkillHasBundledAssets(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	withAssets := filepath.Join(dir, "skills", "alpha")
	if err := os.MkdirAll(filepath.Join(withAssets, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(withAssets, "SKILL.md"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(withAssets, "scripts", "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	lone := filepath.Join(dir, "skills", "beta")
	if err := os.MkdirAll(lone, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lone, "SKILL.md"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		path string
		want bool
	}{
		{"folder skill with sibling asset", filepath.Join(withAssets, "SKILL.md"), true},
		{"folder skill with only SKILL.md", filepath.Join(lone, "SKILL.md"), false},
		{"flat-file skill", filepath.Join(dir, "skills", "beta.md"), false},
		{"in-memory spec", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := spec.Entry{Kind: spec.KindSkill, Name: "x", Path: tc.path}
			if got := SkillHasBundledAssets(s, SkipSKILLMd); got != tc.want {
				t.Errorf("SkillHasBundledAssets = %v, want %v", got, tc.want)
			}
		})
	}
}
