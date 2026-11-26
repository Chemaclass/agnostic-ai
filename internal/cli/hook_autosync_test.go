package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// stubAutosyncSync replaces the sync child with one that records the
// root it ran in and returns stderr and err.
func stubAutosyncSync(t *testing.T, stderr string, err error) *[]string {
	t.Helper()
	var roots []string
	previous := autosyncSync
	autosyncSync = func(_ context.Context, root string) (string, error) {
		roots = append(roots, root)
		return stderr, err
	}
	t.Cleanup(func() { autosyncSync = previous })
	return &roots
}

func runHookAutosync(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"hook", "autosync"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("hook autosync: %v\n%s", err, out.String())
	}
	return out.String()
}

func autosyncProject(t *testing.T) string {
	t.Helper()
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	t.Setenv("CLAUDE_CODE_REMOTE", "")
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	dir := testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nbuiltins: [autosync]\n")
	return canonicalDir(dir)
}

func TestHookAutosync_SyncsTheProjectFromASubfolderAndNamesOnlyKeptFiles(t *testing.T) {
	root := autosyncProject(t)
	roots := stubAutosyncSync(t, "warning: something noisy\n  ~ kept AGENTS.md (edited since the last sync)\n", nil)
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, filepath.Join(root, "sub"))

	got := runHookAutosync(t, "--target", "codex")

	if len(*roots) != 1 || (*roots)[0] != root {
		t.Errorf("synced in %v, want [%s]", *roots, root)
	}
	if got != "agnostic-ai: ~ kept AGENTS.md (edited since the last sync)\n" {
		t.Errorf("output = %q, want only the kept line", got)
	}
}

func TestHookAutosync_SilentOnSuccessWithNothingKept(t *testing.T) {
	autosyncProject(t)
	stubAutosyncSync(t, "", nil)

	if got := runHookAutosync(t, "--target", "claude"); got != "" {
		t.Errorf("output = %q, want none", got)
	}
}

func TestHookAutosync_DoesNothingOutsideAProject(t *testing.T) {
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	testutil.TempCwd(t)
	roots := stubAutosyncSync(t, "", nil)

	if got := runHookAutosync(t, "--target", "claude"); got != "" {
		t.Errorf("output = %q, want none", got)
	}
	if len(*roots) != 0 {
		t.Errorf("synced in %v outside a project", *roots)
	}
}

func TestHookAutosync_SkipsTheUserSource(t *testing.T) {
	home := testutil.TempCwd(t)
	t.Setenv("AGNOSTIC_AI_HOME", home)
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	roots := stubAutosyncSync(t, "", nil)

	runHookAutosync(t, "--target", "claude")
	if len(*roots) != 0 {
		t.Errorf("synced the user source: %v", *roots)
	}
}

func TestHookAutosync_SilentWhenAnotherSyncHoldsTheLock(t *testing.T) {
	autosyncProject(t)
	stubAutosyncSync(t, "Error: .agnostic-ai/.command-lock: project is locked by agnostic-ai sync (pid 7); retry when it finishes\n", errors.New("exit status 1"))

	if got := runHookAutosync(t, "--target", "codex"); got != "" {
		t.Errorf("output = %q, want none on lock contention", got)
	}
}

func TestHookAutosync_FailedSyncPrintsANoteAndSucceeds(t *testing.T) {
	autosyncProject(t)
	stubAutosyncSync(t, "one\ntwo\nthree\nError: agnostic-ai.yaml: bad targets\n", errors.New("exit status 1"))

	got := runHookAutosync(t, "--target", "codex")
	if !strings.Contains(got, "autosync could not sync; run `agnostic-ai sync`") || !strings.Contains(got, "bad targets") {
		t.Errorf("output = %q, want a note with the error", got)
	}
	if strings.Contains(got, "one") {
		t.Errorf("output = %q, want only the last three lines", got)
	}
}

func TestHookAutosync_ReadsTheTargetProjectDirectory(t *testing.T) {
	root := autosyncProject(t)
	roots := stubAutosyncSync(t, "", nil)
	testutil.Chdir(t, t.TempDir())
	t.Setenv("GEMINI_PROJECT_DIR", root)

	runHookAutosync(t, "--target", "gemini")
	if len(*roots) != 1 || (*roots)[0] != root {
		t.Errorf("synced in %v, want [%s]", *roots, root)
	}
}

func TestHookAutosync_ClaudeCloudWaitsForTheInstallLock(t *testing.T) {
	root := autosyncProject(t)
	isolateGit(t)
	git(t, root, "init", "-q")
	lockPath := filepath.Join(root, ".git", "agnostic-ai-cloud-install.lock")
	holder, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := tryLockProjectFile(holder); err != nil {
		t.Skipf("file locks unsupported here: %v", err)
	}
	released := make(chan time.Time, 1)
	go func() {
		time.Sleep(400 * time.Millisecond)
		released <- time.Now()
		_ = holder.Close()
	}()
	var syncedAt time.Time
	previous := autosyncSync
	autosyncSync = func(context.Context, string) (string, error) {
		syncedAt = time.Now()
		return "", nil
	}
	t.Cleanup(func() { autosyncSync = previous })
	t.Setenv("CLAUDE_CODE_REMOTE", "true")

	runHookAutosync(t, "--target", "claude")
	if releasedAt := <-released; syncedAt.Before(releasedAt) {
		t.Errorf("synced at %v, before the install released its lock at %v", syncedAt, releasedAt)
	}
}

func TestUserLayers_SkipAutosync(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)
	t.Setenv("HOME", cacheDir)
	t.Setenv("LocalAppData", cacheDir)
	b, err := loadUserBundle(t.TempDir(), []string{config.AutosyncBuiltin})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range b.Hooks {
		if h.Name == "autosync-session-start" {
			t.Errorf("user scope loaded the autosync hook %s", h.Path)
		}
	}
}
