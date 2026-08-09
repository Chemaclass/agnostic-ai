package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/chemaclass/agnostic-ai/internal/spec"
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
	writeAndBumpMtime(t, specPath, append(content, '\n'))

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
	writeAndBumpMtime(t, specPath, append(content, '\n'))

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
	writeAndBumpMtime(t, overlayFile, []byte("model = \"o4-2025\"\n"))

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

// setupIncrementalFixture writes a project with three targets and one
// spec per relevant kind: a plain rule (every target emits rules), an
// agent scoped to claude only, and a review (only cursor emits reviews).
// The mix lets the incremental-watch tests assert that a change re-syncs
// exactly the affected target subset.
func setupIncrementalFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte("version: 1\ntargets: [claude, cursor, gemini]\n"), 0o644))
	must(os.MkdirAll(filepath.Join(dir, ".agnostic-ai", "rules"), 0o755))
	must(os.WriteFile(filepath.Join(dir, ".agnostic-ai", "rules", "r1.md"),
		[]byte("---\nname: r1\n---\nrule body"), 0o644))
	must(os.MkdirAll(filepath.Join(dir, ".agnostic-ai", "agents"), 0o755))
	must(os.WriteFile(filepath.Join(dir, ".agnostic-ai", "agents", "a1.md"),
		[]byte("---\nname: a1\ntarget: claude\ndescription: claude-only agent\n---\nagent body"), 0o644))
	must(os.MkdirAll(filepath.Join(dir, ".agnostic-ai", "reviews"), 0o755))
	must(os.WriteFile(filepath.Join(dir, ".agnostic-ai", "reviews", "rev1.md"),
		[]byte("---\nname: rev1\n---\nreview body"), 0o644))
	return dir
}

func TestPlanWatchResync_RuleHitsEveryRuleEmitter(t *testing.T) {
	dir := setupIncrementalFixture(t)
	testutil.Chdir(t, dir)

	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	configured := []string{"claude", "cursor", "gemini"}
	plan := planWatchResync(".", cfg, b, []string{filepath.Join(".agnostic-ai", "rules", "r1.md")}, configured)

	if plan.full {
		t.Fatalf("a rule edit must stay incremental, got full re-sync: %+v", plan)
	}
	if plan.reason != "rule" {
		t.Errorf("reason = %q, want %q", plan.reason, "rule")
	}
	if !slices.Equal(plan.targets, configured) {
		t.Errorf("rule edit targets = %v, want %v (every rule-emitting target)", plan.targets, configured)
	}
}

func TestPlanWatchResync_ClaudeScopedAgentHitsOnlyClaude(t *testing.T) {
	dir := setupIncrementalFixture(t)
	testutil.Chdir(t, dir)

	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	plan := planWatchResync(".", cfg, b,
		[]string{filepath.Join(".agnostic-ai", "agents", "a1.md")},
		[]string{"claude", "cursor", "gemini"})

	if plan.full {
		t.Fatalf("a claude-scoped agent edit must stay incremental, got full: %+v", plan)
	}
	if !slices.Equal(plan.targets, []string{"claude"}) {
		t.Errorf("claude-scoped agent targets = %v, want [claude]", plan.targets)
	}
}

// affectedTargetsForKind reads the same targetsSupportingKind map the
// orphan-kind validator uses (see native_capabilities.go). factory,
// qoder, and openhands gained native MCP support (target-audit
// 2026-08-01); an MCP spec edit must re-sync a project configured with
// only one of them, not silently affect zero targets.
func TestAffectedTargetsForKind_MCPHitsNewlySupportedTargets(t *testing.T) {
	entry := spec.Entry{Kind: spec.KindMCP, Name: "fs"}
	for _, target := range []string{"qoder", "factory", "openhands"} {
		t.Run(target, func(t *testing.T) {
			got := affectedTargetsForKind(spec.KindMCP, []string{target}, entry)
			if !slices.Equal(got, []string{target}) {
				t.Errorf("affectedTargetsForKind(mcp, [%s], entry) = %v, want [%s]", target, got, target)
			}
		})
	}
}

func TestPlanWatchResync_ConfigChangeForcesFull(t *testing.T) {
	dir := setupIncrementalFixture(t)
	testutil.Chdir(t, dir)

	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	plan := planWatchResync(".", cfg, b, []string{"agnostic-ai.yaml"}, []string{"claude", "cursor"})
	if !plan.full {
		t.Errorf("editing agnostic-ai.yaml must force a full re-sync, got %+v", plan)
	}
}

func TestPlanWatchResync_OverlayChangeForcesFull(t *testing.T) {
	dir := setupIncrementalFixture(t)
	testutil.Chdir(t, dir)

	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	overlay := filepath.Join(agnosticOverlayDir, "codex.config.toml")
	plan := planWatchResync(".", cfg, b, []string{overlay}, []string{"claude", "codex"})
	if !plan.full {
		t.Errorf("an overlay edit must force a full re-sync, got %+v", plan)
	}
}

func TestPlanWatchResync_DeletedSpecForcesFull(t *testing.T) {
	dir := setupIncrementalFixture(t)
	testutil.Chdir(t, dir)

	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	// A path under rules/ with no matching bundle entry models a delete or
	// rename: the kind is known but the spec is gone, so fall back to full.
	gone := filepath.Join(".agnostic-ai", "rules", "gone.md")
	plan := planWatchResync(".", cfg, b, []string{gone}, []string{"claude", "cursor"})
	if !plan.full {
		t.Errorf("a removed/renamed spec must force a full re-sync, got %+v", plan)
	}
}

func TestPlanWatchResync_UnknownPathForcesFull(t *testing.T) {
	dir := setupIncrementalFixture(t)
	testutil.Chdir(t, dir)

	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	plan := planWatchResync(".", cfg, b, []string{"README.md"}, []string{"claude", "cursor"})
	if !plan.full {
		t.Errorf("an unrecognized path must force a full re-sync, got %+v", plan)
	}
}

func TestPlanWatchResync_KindWithNoConfiguredEmitterSkips(t *testing.T) {
	dir := setupIncrementalFixture(t)
	testutil.Chdir(t, dir)

	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	// Reviews are emitted only by cursor. With cursor absent from the
	// configured set the change maps to no target: it is attributed (not a
	// full re-sync), just with an empty subset the caller skips.
	rev := filepath.Join(".agnostic-ai", "reviews", "rev1.md")
	plan := planWatchResync(".", cfg, b, []string{rev}, []string{"claude", "gemini"})
	if plan.full {
		t.Fatalf("a review edit is attributable, not a full re-sync: %+v", plan)
	}
	if len(plan.targets) != 0 {
		t.Errorf("review edit with no cursor configured should map to no targets, got %v", plan.targets)
	}
}

func TestPlanWatchResync_MultiKindBurstUnionsTargets(t *testing.T) {
	dir := setupIncrementalFixture(t)
	testutil.Chdir(t, dir)

	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	// A save-all burst that touches both the claude-scoped agent and the
	// shared rule re-syncs the union: claude (agent + rule) plus every
	// other rule-emitting target.
	changed := []string{
		filepath.Join(".agnostic-ai", "agents", "a1.md"),
		filepath.Join(".agnostic-ai", "rules", "r1.md"),
	}
	plan := planWatchResync(".", cfg, b, changed, []string{"claude", "cursor", "gemini"})
	if plan.full {
		t.Fatalf("a multi-spec burst must stay incremental, got full: %+v", plan)
	}
	if !slices.Equal(plan.targets, []string{"claude", "cursor", "gemini"}) {
		t.Errorf("multi-kind burst targets = %v, want the union", plan.targets)
	}
	if plan.reason != "agent, rule" {
		t.Errorf("reason = %q, want %q", plan.reason, "agent, rule")
	}
}

// safeBuffer is a mutex-guarded buffer so the watch goroutine can write
// the summary while the test goroutine reads it under -race.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestWatchSync_IncrementalReSyncsOnlyAffectedTarget drives the real
// watch loop end to end: editing a claude-scoped agent spec must re-sync
// only claude, and the summary must name that single target rather than
// reporting a full re-sync of every configured target.
func TestWatchSync_IncrementalReSyncsOnlyAffectedTarget(t *testing.T) {
	dir := setupIncrementalFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	// Pin the summary sink and verbosity so the assertion never depends on
	// global state a prior test left behind (e.g. a `--quiet` run).
	buf := &safeBuffer{}
	prevOut, prevVerbosity := logOut, verbosity
	logOut = buf
	verbosity = levelDefault
	t.Cleanup(func() { logOut, verbosity = prevOut, prevVerbosity })

	// Generous ceilings so the assertion never flakes on a loaded CI
	// runner: under `-race` the poll -> debounce -> re-sync chain can take
	// seconds. The waits below break as soon as the expected output lands,
	// so the common path still finishes in well under a second.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		// forcePoll=true keeps the test deterministic across platforms.
		done <- watchSync(ctx, 20*time.Millisecond, ".", []string{"claude", "cursor"}, false, false, "off", true, 1)
	}()

	// Wait for the initial full sync to land claude's agent file.
	claudeAgent := filepath.Join(dir, ".claude", "agents", "a1.md")
	waitForFile(t, claudeAgent, 10*time.Second)

	// Touch the claude-scoped agent spec; ensure the mtime advances first.
	specPath := filepath.Join(dir, ".agnostic-ai", "agents", "a1.md")
	content, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	writeAndBumpMtime(t, specPath, append(content, '\n'))

	want := "re-syncing 1 target: claude"
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), want) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	got := buf.String()
	if !strings.Contains(got, want) {
		t.Errorf("incremental summary missing %q; got:\n%s", want, got)
	}
	if strings.Contains(got, "full re-sync") {
		t.Errorf("a claude-scoped agent edit must not trigger a full re-sync; got:\n%s", got)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// waitForFile blocks until path exists or the timeout elapses.
// writeAndBumpMtime writes data to path and forces the modification time
// strictly forward, so an mtime-polling watcher is guaranteed to see a
// change.
//
// Sleeping first and relying on the write to advance the clock is not
// enough. Windows updates a file's last-write-time lazily, so a rewrite
// microseconds later can land on the identical timestamp; the poller then
// sees no change, the re-sync never fires, and the test fails only once
// its own wait deadline expires. That is what made
// TestWatchSync_IncrementalReSyncsOnlyAffectedTarget flake on
// windows-latest at almost exactly its 15s ceiling (#585).
//
// Setting the timestamp explicitly removes the race instead of widening
// the window, and drops the sleep, so the test is faster on every
// platform.
func writeAndBumpMtime(t *testing.T, path string, data []byte) {
	t.Helper()
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	next := before.ModTime().Add(time.Second)
	if err := os.Chtimes(path, next, next); err != nil {
		t.Fatal(err)
	}
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
