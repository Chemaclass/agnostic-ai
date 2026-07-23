package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
		done <- watchSync(ctx, 10*time.Millisecond, ".", []string{"claude"}, false, false, "off", false, 1)
	}()

	// Wait for initial sync.
	time.Sleep(80 * time.Millisecond)

	claudeMD := filepath.Join(dir, ".claude/rules/r1.md")
	if _, err := os.Stat(claudeMD); err != nil {
		t.Fatal("initial sync did not produce claude rule")
	}

	// Remove the file so we can verify re-emit.
	if err := os.Remove(claudeMD); err != nil {
		t.Fatal(err)
	}

	// Touch the spec to trigger re-emit.
	specPath := filepath.Join(dir, ".agnostic-ai", "rules", "r1.md")
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
		t.Error("watch did not re-emit claude rule after spec change")
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
		done <- watchSync(ctx, 20*time.Millisecond, ".", []string{"claude"}, false, false, "off", true, 1)
	}()

	time.Sleep(80 * time.Millisecond)

	claudeMD := filepath.Join(dir, ".claude/rules/r1.md")
	if _, err := os.Stat(claudeMD); err != nil {
		t.Fatal("initial sync did not produce claude rule")
	}
	if err := os.Remove(claudeMD); err != nil {
		t.Fatal(err)
	}

	specPath := filepath.Join(dir, ".agnostic-ai", "rules", "r1.md")
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
		t.Error("polling watch did not re-emit claude rule after spec change")
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// TestWatchDirs_IncludesOverlayDir asserts that watchDirs lists the
// captured overlay directory so hand-edits to claude.settings.json /
// codex.config.toml trigger a re-emit in `sync --watch`.
func TestWatchDirs_IncludesOverlayDir(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)

	if err := os.MkdirAll(filepath.Join(dir, agnosticOverlayDir), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(".", agnosticOverlayDir)
	found := false
	for _, p := range watchDirs(".", cfg) {
		if p == want {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("watchDirs missing %s\ngot %v", want, watchDirs(".", cfg))
	}
}

// TestWatchDirs_OmitsOverlayDirWhenAbsent makes sure we do not register
// a non-existent overlay dir (would surface as a setup error on some
// platforms). Only watch when the directory has actually been created.
func TestWatchDirs_OmitsOverlayDirWhenAbsent(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)

	cfg, _, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(".", agnosticOverlayDir)
	for _, p := range watchDirs(".", cfg) {
		if p == want {
			t.Errorf("watchDirs should not list %s when it does not exist", want)
		}
	}
}

// TestWatchSync_ReEmitsOnCodexOverlayChange exercises the end-to-end
// watch loop: edit the codex config overlay and confirm the codex
// adapter re-emits .codex/config.toml within the debounce window. Pinned
// to the polling backend so the test is deterministic on every platform
// (fsnotify event delivery on Linux CI is occasionally flaky).
func TestWatchSync_ReEmitsOnCodexOverlayChange(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	overlay := filepath.Join(dir, agnosticOverlayDir)
	if err := os.MkdirAll(overlay, 0o755); err != nil {
		t.Fatal(err)
	}
	overlayFile := filepath.Join(overlay, codexOverlayFile)
	if err := os.WriteFile(overlayFile, []byte("model = \"o4-mini\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		// forcePoll=true: deterministic across platforms.
		done <- watchSync(ctx, 20*time.Millisecond, ".", []string{"codex"}, false, false, "off", true, 1)
	}()

	codexConfig := filepath.Join(dir, ".codex", "config.toml")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(codexConfig); err == nil && len(data) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	first, err := os.ReadFile(codexConfig)
	if err != nil {
		t.Fatalf("initial sync did not produce codex config: %v", err)
	}
	if !strings.Contains(string(first), "o4-mini") {
		t.Fatalf("initial codex config missing overlay content:\n%s", first)
	}

	// Remove the emitted file so we can verify re-emit fires.
	if err := os.Remove(codexConfig); err != nil {
		t.Fatal(err)
	}

	// Edit the overlay → polling backend should detect the mtime change.
	time.Sleep(30 * time.Millisecond)
	if err := os.WriteFile(overlayFile, []byte("model = \"o4-2025\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(codexConfig); err == nil && strings.Contains(string(data), "o4-2025") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	data, err := os.ReadFile(codexConfig)
	if err != nil {
		t.Fatalf("watch did not re-emit codex config after overlay edit: %v", err)
	}
	if !strings.Contains(string(data), "o4-2025") {
		t.Errorf("re-emitted codex config still has old overlay content:\n%s", data)
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
