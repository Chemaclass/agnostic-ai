package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestImportScopedSkillFoldersRestoresScopeAndAssets(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, "services/api/.cursor/skills/review")
	writeFile(t, filepath.Join(skill, "SKILL.md"), "---\nname: review\ndescription: Review API changes.\n---\nBody.\n")
	writeFile(t, filepath.Join(skill, "scripts/check.sh"), "#!/bin/sh\n")
	dst := filepath.Join(root, ".agnostic-ai/skills")
	count, err := importScopedSkillFolders(root, filepath.Join(".cursor", "skills"), dst)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	for _, rel := range []string{"services/api/review/SKILL.md", "services/api/review/scripts/check.sh"} {
		if _, err := os.Stat(filepath.Join(dst, rel)); err != nil {
			t.Errorf("missing scoped import %s: %v", rel, err)
		}
	}
}

func TestImportCodexSkillsRestoresDirectoryScope(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "services/api/.agents/skills/review/SKILL.md"), "---\nname: review\ndescription: Review API changes.\n---\nBody.\n")
	dst := filepath.Join(root, ".agnostic-ai/skills")
	count, err := importCodexSkills(root, dst)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	if _, err := os.Stat(filepath.Join(dst, "services/api/review/SKILL.md")); err != nil {
		t.Fatalf("scoped Codex import missing: %v", err)
	}
}

func TestImportScopedSkillFoldersSkipsCanonicalSources(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".agnostic-ai/examples/.cursor/skills/source/SKILL.md"), "source\n")
	writeFile(t, filepath.Join(root, "services/api/.cursor/skills/native/SKILL.md"), "native\n")
	dst := filepath.Join(root, ".agnostic-ai/skills")
	count, err := importScopedSkillFolders(root, filepath.Join(".cursor", "skills"), dst)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	if _, err := os.Stat(filepath.Join(dst, "services/api/native/SKILL.md")); err != nil {
		t.Fatalf("native scoped skill missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".agnostic-ai/examples/source/SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("canonical source tree was re-imported, err=%v", err)
	}
}

func TestImportScopedSkillFoldersSameNameUnderTwoNativeDirs(t *testing.T) {
	root := t.TempDir()
	skill := func(dir, desc string) {
		writeFile(t, filepath.Join(root, dir, "demo", "SKILL.md"), "---\nname: demo\ndescription: "+desc+"\n---\nBody.\n")
	}
	skill(".claude/skills", "From the root Claude folder.")
	skill(".agents/skills", "From the root agents folder.")
	skill("pkg/.claude/skills", "From the package Claude folder.")
	dst := filepath.Join(root, ".agnostic-ai/skills")
	count, err := importScopedSkillFoldersFrom(root, []string{".agents/skills", ".claude/skills"}, dst)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
	for rel, want := range map[string]string{
		"demo/SKILL.md":     "From the root agents folder.",
		"pkg/demo/SKILL.md": "From the package Claude folder.",
	} {
		data, err := os.ReadFile(filepath.Join(dst, rel))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), want) {
			t.Errorf("%s = %q, want the skill %q", rel, data, want)
		}
	}
}

func TestFindScopedSkillDirsAllMatchesOneWalkPerDir(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{
		".agents/skills/.cursor/skills",
		".cursor/skills",
		"build/.cursor/skills",
		"build/sub/.agents/skills",
		"node_modules/pkg/.agents/skills",
		"nested/.agents/skills",
		"svc/a/.cursor/skills",
		"svc/a/.agents/skills/.cursor/skills",
		"svc/b/.claude/skills",
		".git/x/.agents/skills",
		".agnostic-ai/skills/.cursor/skills",
	} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(root, "nested", ".git"), "gitdir: elsewhere\n")
	importRunTree = &importTree{root: root, ignored: map[string]bool{"build": true, ".cursor": true}}
	t.Cleanup(func() { importRunTree = nil })

	nativeDirs := []string{".cursor/skills", ".agents/skills", ".claude/skills", ".cursor/skills"}
	var want []scopedSkillDir
	for _, d := range nativeDirs {
		found, err := findScopedSkillDirs(root, d)
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, found...)
	}
	got, err := findScopedSkillDirsAll(root, nativeDirs)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("one walk found\n%v\nseparate walks found\n%v", got, want)
	}
	var rels []string
	for _, d := range want {
		rel, err := filepath.Rel(root, d.path)
		if err != nil {
			t.Fatal(err)
		}
		rels = append(rels, filepath.ToSlash(rel)+" scope="+d.scope)
	}
	wantRels := []string{
		".agents/skills/.cursor/skills scope=.agents/skills",
		".cursor/skills scope=",
		"svc/a/.agents/skills/.cursor/skills scope=svc/a/.agents/skills",
		"svc/a/.cursor/skills scope=svc/a",
		".agents/skills scope=",
		"svc/a/.agents/skills scope=svc/a",
		"svc/b/.claude/skills scope=svc/b",
		".agents/skills/.cursor/skills scope=.agents/skills",
		".cursor/skills scope=",
		"svc/a/.agents/skills/.cursor/skills scope=svc/a/.agents/skills",
		"svc/a/.cursor/skills scope=svc/a",
	}
	if !slices.Equal(rels, wantRels) {
		t.Fatalf("separate walks found\n%v\nwant\n%v", rels, wantRels)
	}
}
