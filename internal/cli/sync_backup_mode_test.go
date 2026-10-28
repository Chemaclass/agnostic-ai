package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSync_BackupKeepsPrivateMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX permission bits")
	}
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, "CLAUDE.md", handWrittenClaude)
	if err := os.Chmod("CLAUDE.md", 0o600); err != nil {
		t.Fatal(err)
	}

	runSyncOK(t, "--backup")

	assertPerm(t, "CLAUDE.md.bak", 0o600)
}

func TestSync_BackupOfARecordedOutputIsGitIgnored(t *testing.T) {
	dir, _ := gitRepo(t)
	testutil.Chdir(t, dir)
	silence(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\ngitignore:\n  enabled: true\n")
	mustWriteFile(t, filepath.Join(dir, "CLAUDE.md"), handWrittenClaude)

	runSyncOK(t, "--backup")
	if !gitIgnored(t, dir, "CLAUDE.md.bak") {
		t.Error("CLAUDE.md.bak is not ignored after sync --backup")
	}
	if strings.Contains(readFile(t, worktreeIncludeFile), "CLAUDE.md.bak") {
		t.Errorf("%s copies the backup into new worktrees", worktreeIncludeFile)
	}

	mustWriteFile(t, filepath.Join(dir, "CLAUDE.md.bak"), handWrittenClaude+"\nEdited.\n")
	runSyncOK(t)
	if !gitIgnored(t, dir, "CLAUDE.md.bak") {
		t.Error("an edited CLAUDE.md.bak is not ignored after a later sync")
	}
}

func TestRevertOne_RestoresTheBackupMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX permission bits")
	}
	path := filepath.Join(t.TempDir(), ".mcp.json")
	mustWriteFile(t, path, "{}\n")
	if err := os.WriteFile(path+".bak", []byte(`{"token":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := revertOne(path, false, false); err != nil {
		t.Fatal(err)
	}
	assertPerm(t, path, 0o600)
}

func TestRevertOne_AnOlderWideBackupDoesNotWidenTheFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX permission bits")
	}
	path := filepath.Join(t.TempDir(), ".mcp.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", []byte(`{"token":"secret"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := revertOne(path, false, false); err != nil {
		t.Fatal(err)
	}
	assertPerm(t, path, 0o600)
}

func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s mode = %o, want %o", path, got, want)
	}
}

func TestRevertOne_RestoresFromAReadOnlyBackup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX permission bits")
	}
	path := filepath.Join(t.TempDir(), "CLAUDE.md")
	mustWriteFile(t, path, "generated\n")
	if err := os.WriteFile(path+".bak", []byte(handWrittenClaude), 0o444); err != nil {
		t.Fatal(err)
	}

	if _, err := revertOne(path, false, false); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != handWrittenClaude {
		t.Errorf("CLAUDE.md = %q, want the backup", got)
	}
}

func TestWithBackups_SkipsLinkedOutputs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	testutil.Chdir(t, t.TempDir())
	if err := os.Mkdir("skill", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("skill", "linked"); err != nil {
		t.Fatal(err)
	}

	got := withBackups([]string{"linked", "CLAUDE.md"}, nil)

	if want := []string{"linked", "CLAUDE.md", "CLAUDE.md.bak"}; !slices.Equal(got, want) {
		t.Errorf("withBackups = %v, want %v", got, want)
	}
}
