package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSyncGlobal_BackupKeepsHandEditedRemovedSkill(t *testing.T) {
	home, source := globalAgentTestHome(t)
	src := filepath.Join(source, "skills", "tidy", "SKILL.md")
	mustWriteGlobalTest(t, src, "---\nname: tidy\ndescription: Tidy\n---\nTidy.\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(home, ".claude", "skills", "tidy", "SKILL.md")
	edited := editGlobalFile(t, out, "Tidy.\n", "Tidy, my way.\n")
	if err := os.RemoveAll(filepath.Dir(src)); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runGlobalAgentTest("--only", "claude", "--backup"); err != nil {
		t.Fatalf("--backup must remove the hand-edited file: %v", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("the removed skill must be gone: %v", err)
	}
	if got := readGlobalTest(t, out+".bak"); got != edited {
		t.Errorf("backup must hold the hand edit:\n%s", got)
	}
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Errorf("the next sync must not report the removed file: %v", err)
	}
}

func TestApplyGlobalChanges_BackupKeepsRemovedFileBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	mustWriteGlobalTest(t, path, "{\"a\": 1}\n")
	guards := map[string]*diskSnapshot{path: {data: []byte("{\"a\": 1}\n")}}
	if _, err := applyGlobalChanges(nil, []string{path}, true, guards); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the file must be removed: %v", err)
	}
	if got := readGlobalTest(t, path+".bak"); got != "{\"a\": 1}\n" {
		t.Errorf("backup = %q", got)
	}
}

func TestApplyGlobalChanges_NoBackupFlagLeavesNoBackupOnRemoval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	mustWriteGlobalTest(t, path, "{}\n")
	if _, err := applyGlobalChanges(nil, []string{path}, false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Errorf("no backup without --backup: %v", err)
	}
}

func TestApplyGlobalChanges_FailedRemovalBackupKeepsTheFile(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.json")
	path := filepath.Join(dir, "settings.json")
	mustWriteGlobalTest(t, first, "{}\n")
	mustWriteGlobalTest(t, path, "{\"a\": 1}\n")
	if err := os.MkdirAll(filepath.Join(path+".bak", "busy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := applyGlobalChanges(nil, []string{first, path}, true, nil); err == nil || !strings.Contains(err.Error(), "backup "+path) {
		t.Fatalf("a failed backup must stop the removal: %v", err)
	}
	if got := readGlobalTest(t, path); got != "{\"a\": 1}\n" {
		t.Errorf("file = %q", got)
	}
	if got := readGlobalTest(t, first); got != "{}\n" {
		t.Errorf("an earlier removal must be rolled back: %q", got)
	}
}

func TestApplyGlobalChanges_RemovalBackupOfPrivateFileStaysPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX permission bits")
	}
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(path, []byte("{\"key\": \"secret\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, path+".bak", "old\n")
	if _, err := applyGlobalChanges(nil, []string{path}, true, nil); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("backup mode = %v, want 0600", info.Mode().Perm())
	}
	if got := readGlobalTest(t, path+".bak"); got != "{\"key\": \"secret\"}\n" {
		t.Errorf("backup = %q", got)
	}
}

func TestApplyGlobalChanges_SymlinkRemovalBackupKeepsTargetBytesBesideTheLink(t *testing.T) {
	link, target := symlinkedGlobalTestFile(t, "{\"theme\":\"dark\"}\n")
	guards := map[string]*diskSnapshot{link: {data: []byte("{\"theme\":\"dark\"}\n")}}
	if _, err := applyGlobalChanges(nil, []string{link}, true, guards); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{target, link} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Errorf("%s must be gone: %v", path, err)
		}
	}
	if got := readGlobalTest(t, link+".bak"); got != "{\"theme\":\"dark\"}\n" {
		t.Errorf("backup = %q", got)
	}
	if _, err := os.Lstat(target + ".bak"); !os.IsNotExist(err) {
		t.Errorf("the backup goes beside the link, as for writes: %v", err)
	}
}

func TestSyncGlobal_ReaddedSkillSyncsBesideItsBackup(t *testing.T) {
	home, source := globalAgentTestHome(t)
	src := filepath.Join(source, "skills", "tidy", "SKILL.md")
	mustWriteGlobalTest(t, src, "---\nname: tidy\ndescription: Tidy\n---\nTidy.\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(home, ".claude", "skills", "tidy", "SKILL.md")
	edited := editGlobalFile(t, out, "Tidy.\n", "Tidy, my way.\n")
	if err := os.RemoveAll(filepath.Dir(src)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest("--only", "claude", "--backup"); err != nil {
		t.Fatal(err)
	}

	mustWriteGlobalTest(t, src, "---\nname: tidy\ndescription: Tidy\n---\nTidy, my way.\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("a skill put back in the source must sync beside its backup: %v", err)
	}
	if got := readGlobalTest(t, out); !strings.Contains(got, "Tidy, my way.") {
		t.Errorf("skill = %q", got)
	}
	if got := readGlobalTest(t, out+".bak"); got != edited {
		t.Errorf("the backup must stay: %q", got)
	}
}

func TestApplyGlobalChanges_RollbackRestoresAnEarlierBackup(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.json")
	changed := filepath.Join(dir, "settings.json")
	mustWriteGlobalTest(t, first, "{}\n")
	mustWriteGlobalTest(t, first+".bak", "kept by the user\n")
	mustWriteGlobalTest(t, changed, "{\"a\": 1}\n")
	guards := map[string]*diskSnapshot{changed: {data: []byte("{}\n")}}
	if _, err := applyGlobalChanges(nil, []string{first, changed}, true, guards); err == nil {
		t.Fatal("a changed file must stop the removal")
	}
	if got := readGlobalTest(t, first); got != "{}\n" {
		t.Errorf("first = %q", got)
	}
	if got := readGlobalTest(t, first+".bak"); got != "kept by the user\n" {
		t.Errorf("an earlier backup must come back: %q", got)
	}
	if _, err := os.Stat(changed + ".bak"); !os.IsNotExist(err) {
		t.Errorf("no backup may stay for a file that was kept: %v", err)
	}
}

func TestApplyGlobalChanges_RefusesABackupPathThatIsASymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	mustWriteGlobalTest(t, path, "{}\n")
	elsewhere := filepath.Join(dir, "elsewhere.json")
	if err := os.Symlink(elsewhere, path+".bak"); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if _, err := applyGlobalChanges(nil, []string{path}, true, nil); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("a symlinked backup path must stop the run: %v", err)
	}
	if got := readGlobalTest(t, path); got != "{}\n" {
		t.Errorf("file = %q", got)
	}
	if _, err := os.Lstat(elsewhere); !os.IsNotExist(err) {
		t.Errorf("nothing may be written through the link: %v", err)
	}
	if got, err := os.Readlink(path + ".bak"); err != nil || got != elsewhere {
		t.Errorf("the link must stay: %q, %v", got, err)
	}
}

func TestGlobalFolderMatches_SymlinkedBackupIsACollision(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tidy")
	skill := filepath.Join(dir, "SKILL.md")
	mustWriteGlobalTest(t, skill, "Tidy.\n")
	if err := os.Symlink(filepath.Join(t.TempDir(), "private"), skill+".bak"); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	same, err := globalFolderMatches(dir, map[string][]byte{skill: []byte("Tidy.\n")})
	if err != nil {
		t.Fatal(err)
	}
	if same {
		t.Error("a symlinked .bak in the folder must not be adopted")
	}
}
