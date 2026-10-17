package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfigFiles(t *testing.T, base, local string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	if local != "" {
		if err := os.WriteFile(filepath.Join(dir, LocalOverrideFileName), []byte(local), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoad_ReadsPersonalMemoryFromTheLocalFile(t *testing.T) {
	cfg, err := Load(writeConfigFiles(t, "version: 1\n", "memory:\n  personal: repo\n"))
	if err != nil || !cfg.RepoPersonalMemory() {
		t.Errorf("cfg = %+v, err %v", cfg, err)
	}
}

func TestLoad_RejectsPersonalMemoryInTheSharedConfig(t *testing.T) {
	_, err := Load(writeConfigFiles(t, "version: 1\nmemory:\n  personal: checkout\n", ""))
	if err == nil || !strings.Contains(err.Error(), LocalOverrideFileName) {
		t.Errorf("err = %v, want a pointer to %s", err, LocalOverrideFileName)
	}
}

func TestLoad_RejectsAnUnknownPersonalMemoryMode(t *testing.T) {
	_, err := Load(writeConfigFiles(t, "version: 1\n", "memory:\n  personal: home\n"))
	if err == nil || !strings.Contains(err.Error(), "memory.personal") || !strings.Contains(err.Error(), LocalOverrideFileName) {
		t.Errorf("err = %v, want it to name memory.personal and %s", err, LocalOverrideFileName)
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)
	if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func TestProjectGit_GivesACopyTheModeAWorktreeInherits(t *testing.T) {
	main := writeConfigFiles(t, "version: 1\n", "memory:\n  personal: repo\n")
	gitRun(t, main, "init", "-q")
	gitRun(t, main, "add", ConfigFileName)
	gitRun(t, main, "commit", "-q", "-m", "init")
	worktree := filepath.Join(t.TempDir(), "wt")
	gitRun(t, main, "worktree", "add", "-q", worktree)

	git, ok, err := ReadProjectGit(worktree)
	if err != nil || !ok || git.Personal != PersonalMemoryRepo || git.CommonDir == "" {
		t.Fatalf("ReadProjectGit = %+v, %v, %v", git, ok, err)
	}
	copyDir := writeConfigFiles(t, "version: 1\n", "")
	if cfg, err := Load(copyDir); err != nil || cfg.RepoPersonalMemory() {
		t.Fatalf("a copy without Git inherited: %+v, %v", cfg, err)
	}
	clear := UseProjectGit(copyDir, git)
	cfg, err := Load(copyDir)
	clear()
	if err != nil || !cfg.RepoPersonalMemory() {
		t.Errorf("cfg = %+v, err %v, want the inherited repo mode", cfg, err)
	}
	if _, ok := ProjectGitFor(copyDir); ok {
		t.Error("the override outlived its clear")
	}
}
