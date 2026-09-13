package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const cursorGreet = ".cursor/skills/greet"

func assertRealDir(t *testing.T, p string) {
	t.Helper()
	fi, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.IsDir() {
		t.Errorf("%s should be a real directory, got mode %v", p, fi.Mode())
	}
}

// A folder that could hold a user-owned file never becomes a link: the
// user's file would otherwise live in, and be written through to, the
// canonical copy another target owns.
func TestSync_SharedSkills_UnmanagedFolderIsNeverLinked(t *testing.T) {
	dir := setupSharedSkillsFixture(t, sharedSkillsCfg+"  unmanaged:\n    - "+cursorGreet+"/notes.md\n")
	testutil.Chdir(t, dir)
	silence(t)

	if err := runSync(t); err != nil {
		t.Fatal(err)
	}

	assertRealDir(t, cursorGreet)
	if _, err := os.Stat(filepath.Join(cursorGreet, "SKILL.md")); err != nil {
		t.Errorf("generated SKILL.md should still be written: %v", err)
	}
}

// Owning a file inside an already linked folder replaces the link with a
// real copy before emission, so the edit made through the link survives
// while the canonical copy is regenerated for its own target.
func TestSync_SharedSkills_OwningLinkedFileKeepsEdit(t *testing.T) {
	dir := setupSharedSkillsFixture(t, sharedSkillsCfg)
	testutil.Chdir(t, dir)
	silence(t)
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(cursorGreet); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("fixture should link %s first: mode %v, err %v", cursorGreet, fi, err)
	}
	owned := filepath.Join(cursorGreet, "SKILL.md")
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), sharedSkillsCfg+"  unmanaged:\n    - "+filepath.ToSlash(owned)+"\n")
	mustWriteFile(t, owned, "mine\n") // written through the link

	if err := runSync(t); err != nil {
		t.Fatal(err)
	}

	assertRealDir(t, cursorGreet)
	if got := readFile(t, owned); got != "mine\n" {
		t.Errorf("user-owned skill file lost: %q", got)
	}
	if got := readFile(t, ".agents/skills/greet/SKILL.md"); got == "mine\n" {
		t.Error("canonical copy should be regenerated for codex")
	}
}
