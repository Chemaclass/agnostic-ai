package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSync_SharedSkills_MissingExternalAdapterKeepsLinks(t *testing.T) {
	dir := setupSharedSkillsFixture(t, sharedSkillsCfg)
	testutil.Chdir(t, dir)
	silence(t)
	mustWriteFile(t, filepath.Join(".agnostic-ai", "skills", "wave.md"), "---\nname: wave\ndescription: Wave\n---\nWave at the user.\n")
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	links := []string{filepath.Join(".cursor", "skills", "greet"), filepath.Join(".cursor", "skills", "wave")}
	for _, link := range links {
		if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("first sync did not link %s: %v", link, err)
		}
	}

	t.Setenv("PATH", t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex, cursor, fakex]\nsync:\n  shared-skills: true\n")
	if err := os.Remove(filepath.Join(".agnostic-ai", "skills", "wave.md")); err != nil {
		t.Fatal(err)
	}
	if err := runSync(t); err != nil {
		t.Fatalf("sync without the fakex adapter: %v", err)
	}

	for _, link := range links {
		if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("sync removed %s while fakex did not load: %v", link, err)
		}
		if _, err := os.Stat(filepath.Join(link, "SKILL.md")); err != nil {
			t.Errorf("%s no longer resolves: %v", link, err)
		}
	}
}

func TestPlanSharedSkills_MissingExternalAdapterIsNotFullCoverage(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Setenv("PATH", t.TempDir())
		cfg := &config.Config{Targets: []string{"codex", "cursor", "fakex"}, Sync: config.SyncConfig{SharedSkills: enabled}}
		st, err := planSharedSkills(cfg, newTargetRenders(cfg, spec.Bundle{}), cfg.Targets)
		if err != nil {
			t.Fatal(err)
		}
		if st.coversAll {
			t.Errorf("shared-skills %v: a run without fakex counts as full", enabled)
		}
	}
}
