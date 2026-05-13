package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestMtimesChanged_DetectsAdd(t *testing.T) {
	prev := map[string]time.Time{"a": time.Unix(1, 0)}
	curr := map[string]time.Time{"a": time.Unix(1, 0), "b": time.Unix(2, 0)}
	if !mtimesChanged(prev, curr) {
		t.Error("expected changed when file added")
	}
}

func TestMtimesChanged_DetectsRemove(t *testing.T) {
	prev := map[string]time.Time{"a": time.Unix(1, 0), "b": time.Unix(2, 0)}
	curr := map[string]time.Time{"a": time.Unix(1, 0)}
	if !mtimesChanged(prev, curr) {
		t.Error("expected changed when file removed")
	}
}

func TestMtimesChanged_DetectsModify(t *testing.T) {
	prev := map[string]time.Time{"a": time.Unix(1, 0)}
	curr := map[string]time.Time{"a": time.Unix(2, 0)}
	if !mtimesChanged(prev, curr) {
		t.Error("expected changed when mtime updated")
	}
}

func TestMtimesChanged_NoChange(t *testing.T) {
	prev := map[string]time.Time{"a": time.Unix(1, 0)}
	curr := map[string]time.Time{"a": time.Unix(1, 0)}
	if mtimesChanged(prev, curr) {
		t.Error("expected no change when mtimes identical")
	}
}

func TestCollectMtimes_WalksDir(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "x.md")
	if err := os.WriteFile(f, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := collectMtimes([]string{dir})
	if _, ok := m[f]; !ok {
		t.Errorf("expected %s in mtimes map", f)
	}
}

func TestCollectMtimes_SkipsMissingPath(t *testing.T) {
	m := collectMtimes([]string{"/no/such/path/xyz"})
	if len(m) != 0 {
		t.Errorf("expected empty map for missing path, got %d entries", len(m))
	}
}

func TestWatchSync_ReEmitsOnChange(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- watchSync(ctx, 10*time.Millisecond, ".", []string{"claude"}, false, false, "off", false)
	}()

	// Wait for initial sync.
	time.Sleep(80 * time.Millisecond)

	claudeMD := filepath.Join(dir, "CLAUDE.md")
	if _, err := os.Stat(claudeMD); err != nil {
		t.Fatal("initial sync did not produce CLAUDE.md")
	}

	// Remove the file so we can verify re-emit.
	if err := os.Remove(claudeMD); err != nil {
		t.Fatal(err)
	}

	// Touch the spec to trigger re-emit.
	specPath := filepath.Join(dir, "rules", "r1.md")
	content, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(15 * time.Millisecond) // ensure mtime advances
	if err := os.WriteFile(specPath, append(content, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for watch loop to pick up the change.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(claudeMD); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if _, err := os.Stat(claudeMD); err != nil {
		t.Error("watch did not re-emit CLAUDE.md after spec change")
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestWatchSync_CheckAndWatchIncompatible(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--watch", "--check"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for --watch --check combination")
	}
}

func TestWatchSync_WatchPollWithoutWatch(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--watch-poll"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when --watch-poll is used without --watch")
	}
}

func TestWatchSync_PollFallback(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		// forcePoll = true exercises the polling backend explicitly.
		done <- watchSync(ctx, 20*time.Millisecond, ".", []string{"claude"}, false, false, "off", true)
	}()

	time.Sleep(80 * time.Millisecond)

	claudeMD := filepath.Join(dir, "CLAUDE.md")
	if _, err := os.Stat(claudeMD); err != nil {
		t.Fatal("initial sync did not produce CLAUDE.md")
	}
	if err := os.Remove(claudeMD); err != nil {
		t.Fatal(err)
	}

	specPath := filepath.Join(dir, "rules", "r1.md")
	content, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(15 * time.Millisecond)
	if err := os.WriteFile(specPath, append(content, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(claudeMD); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(claudeMD); err != nil {
		t.Error("polling watch did not re-emit CLAUDE.md after spec change")
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestIsIgnoredEvent_Chmod(t *testing.T) {
	if !isIgnoredEvent(fsnotify.Event{Name: "x", Op: fsnotify.Chmod}) {
		t.Error("chmod-only events must be ignored")
	}
	if isIgnoredEvent(fsnotify.Event{Name: "x", Op: fsnotify.Write}) {
		t.Error("write events must not be ignored")
	}
}

func TestIsIgnoredEvent_SyncStateFile(t *testing.T) {
	ev := fsnotify.Event{Name: filepath.Join("anywhere", ".sync-state"), Op: fsnotify.Write}
	if !isIgnoredEvent(ev) {
		t.Error(".sync-state writes must be ignored to avoid feedback loops")
	}
}
