package emit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
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

func TestPropagateSkillAssets_LinkedSourceRootCopiesRegularAssets(t *testing.T) {
	root := t.TempDir()
	testutil.Chdir(t, root)
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("Skill body.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const script = "#!/bin/sh\necho shared\n"
	if err := os.WriteFile(filepath.Join(source, "scripts", "run.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "outside.txt"), []byte("Do not copy.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.DirectoryAlias(t, outside, filepath.Join(source, "nested-link"))
	alias := filepath.Join(root, "skills-alias")
	testutil.DirectoryAlias(t, source, alias)
	sk := spec.Entry{Kind: spec.KindSkill, Name: "shared", Path: filepath.Join(alias, "SKILL.md")}
	if !SkillHasBundledAssets(sk, SkipSKILLMd) {
		t.Error("linked skill root hides bundled assets")
	}
	dst := filepath.Join(root, "out")
	if err := NewSession().PropagateSkillAssets(sk, dst, SkipSKILLMd, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dst, "scripts", "run.sh")
	if got, err := os.ReadFile(path); err != nil || string(got) != script {
		t.Errorf("copied asset = %q, %v; want %q", got, err, script)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o755 {
			t.Errorf("copied executable mode: %v, %v", info, err)
		}
	}
	for _, rel := range []string{"SKILL.md", filepath.Join("nested-link", "outside.txt")} {
		if _, err := os.Stat(filepath.Join(dst, rel)); !os.IsNotExist(err) {
			t.Errorf("unexpected copied file %s: %v", rel, err)
		}
	}
}

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

func TestPropagateSkillAssets_CopiesTheListedFilesWithoutWalkingTheFolder(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	srcSkill := filepath.Join(dir, "skills", "alpha")
	for rel, body := range map[string]string{"SKILL.md": "---\nname: alpha\n---\nbody\n", "listed.txt": "listed\n", "added-later.txt": "late\n"} {
		if err := os.MkdirAll(srcSkill, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(srcSkill, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := spec.Entry{
		Kind: spec.KindSkill, Name: "alpha", Path: filepath.Join(srcSkill, "SKILL.md"),
		Assets: &[]spec.AssetFile{{Rel: "SKILL.md", Mode: 0o644}, {Rel: "listed.txt", Mode: 0o644}},
	}
	dst := filepath.Join(dir, "out", "alpha")

	if err := sess.PropagateSkillAssets(s, dst, SkipSKILLMd, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dst, "listed.txt")); err != nil {
		t.Errorf("listed asset not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "added-later.txt")); err == nil {
		t.Error("copied a file the load did not list: the folder was walked again")
	}
}
