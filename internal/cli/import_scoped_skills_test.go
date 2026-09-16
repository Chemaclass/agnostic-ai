package cli

import (
	"os"
	"path/filepath"
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
