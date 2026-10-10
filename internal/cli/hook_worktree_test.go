package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runWorktreeRemoval(t *testing.T, repo, allowed, path string) (string, error) {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"hook_event_name": "WorktreeRemove", "worktree_path": path})
	if err != nil {
		t.Fatal(err)
	}
	return executeWorktreeRemoval(string(raw), "--repo", repo, "--allowed-root", allowed)
}

func executeWorktreeRemoval(payload string, args ...string) (string, error) {
	var out bytes.Buffer
	cmd := NewRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(payload))
	cmd.SetArgs(append([]string{"hook", "worktree-remove"}, args...))
	err := cmd.Execute()
	return out.String(), err
}

func disposableWorktree(t *testing.T) (main, allowed, linked string) {
	t.Helper()
	main = setupGitRepo(t)
	git(t, main, "commit", "-q", "--allow-empty", "-m", "base")
	allowed = t.TempDir()
	linked = filepath.Join(allowed, "worktree with spaces")
	git(t, main, "worktree", "add", "-q", "--detach", linked)
	return
}

func TestHookWorktreeRemove_RemovesCleanRegisteredWorktreeAndRepeatsSafely(t *testing.T) {
	main, allowed, linked := disposableWorktree(t)
	out, err := runWorktreeRemoval(t, main, allowed, linked)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "removed") {
		t.Errorf("output = %q", out)
	}
	if _, err := os.Stat(linked); !os.IsNotExist(err) {
		t.Fatalf("worktree remains: %v", err)
	}
	out, err = runWorktreeRemoval(t, main, allowed, linked)
	if err != nil || !strings.Contains(out, "already absent") {
		t.Errorf("repeat = %q, %v", out, err)
	}
}

func TestHookWorktreeRemove_PreservesUnsafePaths(t *testing.T) {
	for _, scenario := range []string{"main", "dirty", "tracked", "locked", "unregistered", "foreign", "outside", "traversal", "symlink", "symlink-escape", "root"} {
		t.Run(scenario, func(t *testing.T) {
			main, allowed, linked := disposableWorktree(t)
			candidate := linked
			switch scenario {
			case "main":
				candidate = main
				allowed = filepath.Dir(main)
			case "dirty":
				if err := os.WriteFile(filepath.Join(linked, "untracked.txt"), []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "locked":
				git(t, main, "worktree", "lock", linked)
			case "tracked":
				file := filepath.Join(linked, "tracked.txt")
				if err := os.WriteFile(file, []byte("base"), 0o600); err != nil {
					t.Fatal(err)
				}
				git(t, linked, "add", ".")
				git(t, linked, "commit", "-q", "-m", "base file")
				if err := os.WriteFile(file, []byte("keep change"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "unregistered":
				candidate = filepath.Join(allowed, "ordinary")
				if err := os.Mkdir(candidate, 0o700); err != nil {
					t.Fatal(err)
				}
			case "foreign":
				foreign, _, _ := disposableWorktree(t)
				candidate = filepath.Join(allowed, "foreign")
				git(t, foreign, "worktree", "add", "-q", "--detach", candidate)
			case "outside":
				allowed = t.TempDir()
			case "traversal":
				candidate = filepath.Join(allowed, "child") + string(filepath.Separator) + ".." + string(filepath.Separator) + filepath.Base(linked)
			case "symlink":
				candidate = filepath.Join(allowed, "alias")
				if err := os.Symlink(linked, candidate); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			case "symlink-escape":
				outside := t.TempDir()
				git(t, main, "worktree", "add", "-q", "--detach", filepath.Join(outside, "child"))
				alias := filepath.Join(allowed, "alias")
				if err := os.Symlink(outside, alias); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
				candidate = filepath.Join(alias, "child")
			case "root":
				allowed = linked
			}
			_, err := runWorktreeRemoval(t, main, allowed, candidate)
			if err == nil {
				t.Fatal("unsafe removal accepted")
			}
			if _, err := os.Stat(candidate); err != nil && scenario != "traversal" {
				t.Errorf("candidate was not preserved: %v", err)
			}
			if _, err := os.Stat(linked); err != nil {
				t.Errorf("linked worktree was not preserved: %v", err)
			}
		})
	}
}

func TestHookWorktreeRemove_RejectsMalformedPayloadAndMissingPolicy(t *testing.T) {
	main, allowed, linked := disposableWorktree(t)
	for _, payload := range []string{"{", "null", `{"hook_event_name":"SessionEnd","worktree_path":"/tmp/example"}`, `{"hook_event_name":"WorktreeRemove"}`, `{"hook_event_name":"WorktreeRemove","worktree_path":"relative"}`, `{"hook_event_name":"WorktreeRemove","worktree_path":12}`} {
		if _, err := executeWorktreeRemoval(payload, "--repo", main, "--allowed-root", allowed); err == nil {
			t.Errorf("accepted %s", payload)
		}
	}
	for _, args := range [][]string{nil, {"--repo", main}, {"--allowed-root", allowed}} {
		if _, err := executeWorktreeRemoval(`{}`, args...); err == nil {
			t.Errorf("accepted flags %v", args)
		}
	}
	if _, err := os.Stat(linked); err != nil {
		t.Fatal(err)
	}
}

func TestHookWorktreeRemove_PreservesIgnoredFiles(t *testing.T) {
	main, allowed, linked := disposableWorktree(t)
	if err := os.WriteFile(filepath.Join(main, ".git", "info", "exclude"), []byte("private.txt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	private := filepath.Join(linked, "private.txt")
	if err := os.WriteFile(private, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runWorktreeRemoval(t, main, allowed, linked); err == nil {
		t.Fatal("ignored file accepted")
	}
	if got, err := os.ReadFile(private); err != nil || string(got) != "private" {
		t.Errorf("private file changed: %q, %v", got, err)
	}
}

func TestHookWorktreeRemove_RejectsReplacedRepositoryMetadata(t *testing.T) {
	main, allowed, linked := disposableWorktree(t)
	foreign, _, _ := disposableWorktree(t)
	if err := os.WriteFile(filepath.Join(linked, ".git"), []byte("gitdir: "+filepath.Join(foreign, ".git")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runWorktreeRemoval(t, main, allowed, linked); err == nil {
		t.Fatal("foreign metadata accepted")
	}
	if _, err := os.Stat(linked); err != nil {
		t.Fatal(err)
	}
}

func TestHookWorktreeRemove_PreservesAbsentRegistration(t *testing.T) {
	main, allowed, linked := disposableWorktree(t)
	// Rename simulates a host that already moved the checkout, leaving Git metadata.
	moved := linked + " moved"
	if err := os.Rename(linked, moved); err != nil {
		t.Fatal(err)
	}
	before := git(t, main, "worktree", "list", "--porcelain")
	out, err := runWorktreeRemoval(t, main, allowed, linked)
	if err != nil || !strings.Contains(out, "already absent") {
		t.Fatalf("absent = %q, %v", out, err)
	}
	if after := git(t, main, "worktree", "list", "--porcelain"); before != after {
		t.Error("absent registration changed")
	}
	if _, err := os.Stat(moved); err != nil {
		t.Fatal(err)
	}
}

func TestHookWorktreeRemove_IgnoresInheritedRepositoryRedirects(t *testing.T) {
	main, allowed, linked := disposableWorktree(t)
	foreign, _, _ := disposableWorktree(t)
	t.Setenv("GIT_DIR", filepath.Join(foreign, ".git"))
	t.Setenv("GIT_WORK_TREE", foreign)
	if _, err := runWorktreeRemoval(t, main, allowed, linked); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatal(err)
	}
}
