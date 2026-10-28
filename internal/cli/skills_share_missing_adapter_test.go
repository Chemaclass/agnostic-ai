package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSharedSkills_MissingExternalAdapterKeepsLinks(t *testing.T) {
	dir := setupSharedSkillsFixture(t, sharedSkillsCfg)
	testutil.Chdir(t, dir)
	silence(t)
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(".cursor", "skills", "greet")
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("first sync did not link %s: %v", link, err)
	}

	t.Setenv("PATH", t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex, cursor, fakex]\nsync:\n  shared-skills: true\n")
	if err := os.Remove(filepath.Join(".agnostic-ai", "skills", "greet.md")); err != nil {
		t.Fatal(err)
	}
	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	st, err := planSharedSkills(cfg, b, cfg.Targets)
	if err != nil {
		t.Fatal(err)
	}
	if st.coversAll || len(st.links) > 0 {
		t.Errorf("plan treats a run without fakex as full: coversAll=%v links=%v", st.coversAll, st.links)
	}
	sess, err := st.reconcile(readStateFile(".").Outputs, false)
	if err != nil {
		t.Fatal(err)
	}
	sess.Commit()
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("reconcile removed %s while fakex did not load: %v", link, err)
	}
}
