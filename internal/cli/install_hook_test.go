package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Initialize a real git repo so git commands work.
	cmd := exec.Command("git", "init", dir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	return dir
}

func TestInstallHook_CreatesPreCommit(t *testing.T) {
	dir := setupGitRepo(t)

	var buf strings.Builder
	if err := installPreCommitHook(dir, false, &buf); err != nil {
		t.Fatalf("installPreCommitHook: %v", err)
	}

	hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
	data, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("hook not written: %v", err)
	}
	if !strings.Contains(string(data), "agnostic-ai sync --check") {
		t.Errorf("hook missing sync --check, got:\n%s", data)
	}
	if !strings.Contains(string(data), "#!/bin/sh") {
		t.Errorf("hook missing shebang, got:\n%s", data)
	}
}

func TestInstallHook_AppendsToExisting(t *testing.T) {
	dir := setupGitRepo(t)
	hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
	existing := "#!/bin/sh\necho 'existing hook'\n"
	if err := os.WriteFile(hookPath, []byte(existing), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := installPreCommitHook(dir, false, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(hookPath)
	got := string(data)
	if !strings.Contains(got, "existing hook") {
		t.Error("existing content overwritten")
	}
	if !strings.Contains(got, "agnostic-ai sync --check") {
		t.Error("agnostic-ai line not appended")
	}
}

func TestInstallHook_Idempotent(t *testing.T) {
	dir := setupGitRepo(t)

	for i := 0; i < 3; i++ {
		if err := installPreCommitHook(dir, false, &strings.Builder{}); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}

	hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
	data, _ := os.ReadFile(hookPath)
	count := strings.Count(string(data), "agnostic-ai sync --check")
	if count != 1 {
		t.Errorf("expected 1 occurrence of sync --check, got %d:\n%s", count, data)
	}
}

func TestInstallHook_Shared_CreatesGithooksDir(t *testing.T) {
	dir := setupGitRepo(t)

	if err := installPreCommitHook(dir, true, &strings.Builder{}); err != nil {
		t.Fatalf("shared install: %v", err)
	}

	hookPath := filepath.Join(dir, ".githooks", "pre-commit")
	data, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf(".githooks/pre-commit not created: %v", err)
	}
	if !strings.Contains(string(data), "agnostic-ai sync --check") {
		t.Errorf("hook missing sync --check, got:\n%s", data)
	}
}
