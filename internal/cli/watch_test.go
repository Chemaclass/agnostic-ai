package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
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

	buf := captureWatchOutput(t)

	done := make(chan error, 1)
	go func() {
		done <- watchSync(ctx, 10*time.Millisecond, ".", []string{"claude"}, false, false, "off", false, 1)
	}()

	// Wait for the watcher to arm, not merely for the initial sync: the
	// baseline snapshot is taken before the banner prints, so editing a
	// watched file earlier is invisible to the poller (#585).
	waitForOutput(t, buf, "watching", 10*time.Second)

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

func TestWatchSync_PreservesOrdinarySyncBytes(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	if err := runSyncOnce(".", []string{"codex"}, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "AGENTS.md")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, stop := startWatch(t, []string{"codex"}, false)
	defer stop()
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("watch initial sync changed ordinary sync bytes:\nbefore:\n%s\nafter:\n%s", before, after)
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

	buf := captureWatchOutput(t)

	done := make(chan error, 1)
	go func() {
		// forcePoll = true exercises the polling backend explicitly.
		done <- watchSync(ctx, 20*time.Millisecond, ".", []string{"claude"}, false, false, "off", true, 1)
	}()

	waitForOutput(t, buf, "watching", 10*time.Second)

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

// Every input is listed before it exists, so poll mode sees it appear and
// fsnotify mode knows which new entry of a watched parent counts. A
// missing path is skipped when the watcher registers it.
func TestWatchDirs_ListsInputsBeforeTheyExist(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)

	cfg, _, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	got := watchDirs(".", cfg)
	for _, want := range []string{
		agnosticOverlayDir,
		"agnostic-ai.local.yaml",
		"agnostic.config.yaml",
		filepath.Join(".agnostic-ai", "commands"),
	} {
		if _, err := os.Stat(want); err == nil {
			t.Fatalf("%s must not exist in the fixture", want)
		}
		if !slices.Contains(got, filepath.Join(".", want)) {
			t.Errorf("watchDirs missing %s\ngot %v", want, got)
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

	buf := captureWatchOutput(t)

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

	waitForOutput(t, buf, "watching", 10*time.Second)

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
	if !isIgnoredEvent(fsnotify.Event{Name: "x", Op: fsnotify.Chmod}, ".") {
		t.Error("chmod-only events must be ignored")
	}
	if isIgnoredEvent(fsnotify.Event{Name: "x", Op: fsnotify.Write}, ".") {
		t.Error("write events must not be ignored")
	}
}

func TestIsIgnoredEvent_SyncStateFile(t *testing.T) {
	ev := fsnotify.Event{Name: filepath.Join("anywhere", ".sync-state"), Op: fsnotify.Write}
	if !isIgnoredEvent(ev, ".") {
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

func TestPlanWatchResync_AbsoluteSourceChangesStayIncremental(t *testing.T) {
	dir := testutil.TempCwd(t)
	external := t.TempDir()
	rules := filepath.Join(external, "rules")
	skills := filepath.Join(external, "skills")
	rule := filepath.Join(rules, "demo.md")
	skill := filepath.Join(skills, "demo", "SKILL.md")
	asset := filepath.Join(skills, "demo", "references", "guide.txt")
	writeTestFile(t, rule, "---\nname: demo\ntarget: claude\n---\nExternal rule.\n")
	writeTestFile(t, skill, "---\nname: demo\ndescription: demo\ntarget: codex\n---\nExternal skill.\n")
	writeTestFile(t, asset, "External asset.\n")
	writeTestFile(t, config.ConfigFileName, fmt.Sprintf(
		"version: 1\nsources:\n  rules: %q\n  skills: %q\ntargets: [claude, codex]\n",
		filepath.ToSlash(rules), filepath.ToSlash(skills)))
	cfg, b, err := loadProject(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		path, kind, target string
	}{
		{rule, "rule", "claude"},
		{skill, "skill", "codex"},
		{asset, "skill", "codex"},
	} {
		t.Run(tc.kind+"/"+filepath.Base(tc.path), func(t *testing.T) {
			plan := planWatchResync(dir, cfg, b, []string{tc.path}, cfg.Targets)
			if plan.full {
				t.Errorf("absolute source change forced a full re-sync: %+v", plan)
			}
			if plan.reason != tc.kind || !slices.Equal(plan.targets, []string{tc.target}) {
				t.Errorf("plan = %+v, want %s change for [%s]", plan, tc.kind, tc.target)
			}
		})
	}
}

func TestPlanWatchResync_OverlappingAbsoluteSourcesUseLoadedKind(t *testing.T) {
	dir := testutil.TempCwd(t)
	external := t.TempDir()
	hooks := filepath.Join(external, "hooks")
	hook := filepath.Join(hooks, "gate.yaml")
	writeTestFile(t, hook, "name: gate\nevent: PreToolUse\ncommand: echo guard\n")
	writeTestFile(t, config.ConfigFileName, fmt.Sprintf(
		"version: 1\nsources:\n  agents: %q\n  hooks: %q\ntargets: [crush]\n",
		filepath.ToSlash(external), filepath.ToSlash(hooks)))
	cfg, b, err := loadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	plan := planWatchResync(dir, cfg, b, []string{hook}, cfg.Targets)
	if plan.full || plan.reason != "hook" || !slices.Equal(plan.targets, []string{"crush"}) {
		t.Errorf("plan = %+v, want incremental hook change for [crush]", plan)
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

	// Then wait for the watcher to arm before touching anything.
	//
	// The emitted file appearing only proves the initial sync ran.
	// watchSyncPoll takes its baseline mtime snapshot after that sync and
	// prints the banner immediately afterwards, so the banner is the first
	// observable point at which the snapshot exists. Editing the spec
	// before it means the baseline already contains the edit, no change is
	// ever detected, and the test fails when its own deadline expires
	// rather than for any real reason. That is what flaked on the slower
	// windows-latest runner (#585).
	waitForOutput(t, buf, "watching", 10*time.Second)

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

// captureWatchOutput pins the summary sink to a buffer and restores it
// afterwards, so a test can wait on what the watch loop printed.
//
// Every watch test needs this, not only the ones that assert on output:
// waiting for the watcher's banner is the only reliable way to know the
// baseline mtime snapshot exists before editing a watched file (#585).
func captureWatchOutput(t *testing.T) *safeBuffer {
	t.Helper()
	buf := &safeBuffer{}
	prevOut, prevVerbosity := logOut, verbosity
	logOut = buf
	verbosity = levelDefault
	t.Cleanup(func() { logOut, verbosity = prevOut, prevVerbosity })
	return buf
}

// waitForOutput blocks until buf contains want, or fails after timeout.
func waitForOutput(t *testing.T, buf *safeBuffer, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q in watch output; got:\n%s", want, buf.String())
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

// assertWatchPicksUpNewLocalInstructions starts a watch in a project with
// no project-user dir, creates it with an AGNOSTIC_AI.md, and waits
// for the text to reach CLAUDE.md. The directory is the usual way a user
// starts personal instructions, so its creation must not need a restart.
func assertWatchPicksUpNewLocalInstructions(t *testing.T, forcePoll bool) {
	t.Helper()
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	buf := captureWatchOutput(t)
	done := make(chan error, 1)
	go func() {
		done <- watchSync(ctx, 20*time.Millisecond, ".", []string{"claude"}, false, false, "off", forcePoll, 1)
	}()
	waitForOutput(t, buf, "watching", 10*time.Second)
	if _, err := os.Stat(defaultProjectUser); err == nil {
		t.Fatalf("%s must not exist before the watch starts", defaultProjectUser)
	}
	claudeMD := filepath.Join(dir, "CLAUDE.md")
	if _, err := os.Stat(claudeMD); err != nil {
		t.Fatalf("initial sync did not write CLAUDE.md: %v", err)
	}

	if err := os.MkdirAll(defaultProjectUser, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(defaultProjectUser, "AGNOSTIC_AI.md"), []byte("Watched local line.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	var got []byte
	for time.Now().Before(deadline) {
		got, _ = os.ReadFile(claudeMD)
		if strings.Contains(string(got), "Watched local line.") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(string(got), "Watched local line.") {
		t.Errorf("watch did not pick up a new %s/AGNOSTIC_AI.md:\n%s", defaultProjectUser, got)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestWatchSync_PicksUpLocalDirCreatedMidSession(t *testing.T) {
	assertWatchPicksUpNewLocalInstructions(t, false)
}

func TestWatchSync_PollPicksUpLocalDirCreatedMidSession(t *testing.T) {
	assertWatchPicksUpNewLocalInstructions(t, true)
}

// The parents of the inputs are watched only to see an input appear.
// Anything else sync writes there must not count, or every sync would
// trigger the next one.
func TestIsWatchNoise_KeepsOnlyWatchedInputs(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	cfg, _, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	watched := watchDirs(".", cfg)
	if err := os.WriteFile("CLAUDE.md", []byte("generated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(".claude", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("specs", 0o755); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		op   fsnotify.Op
		want bool
	}{
		{"CLAUDE.md", fsnotify.Write, true},
		{".claude", fsnotify.Create, true},
		{filepath.Join(".agnostic-ai", ".generated"), fsnotify.Create, true},
		{filepath.Join(".agnostic-ai", "nested", "file.md"), fsnotify.Write, true},
		{config.LocalOverrideFileName, fsnotify.Create, false},
		{defaultProjectUser, fsnotify.Create, false},
		{filepath.Join(defaultProjectUser, "AGNOSTIC_AI.md"), fsnotify.Write, false},
		{filepath.Join(agnosticOverlayDir, codexOverlayFile), fsnotify.Write, false},
		{".agnostic-ai", fsnotify.Write, true},
	} {
		if got := isWatchNoise(fsnotify.Event{Name: tc.name, Op: tc.op}, watched); got != tc.want {
			t.Errorf("isWatchNoise(%s %q) = %v, want %v", tc.op, tc.name, got, tc.want)
		}
	}
	for _, p := range watched {
		if isWatchNoise(fsnotify.Event{Name: p, Op: fsnotify.Write}, watched) {
			t.Errorf("watched path %q must never be noise", p)
		}
	}
}

// A directory created on the way to a missing input counts only while it
// leads to one, so the watch can arm it and see the input appear.
func TestIsWatchNoise_KeepsDirectoryLeadingToMissingInput(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	writeTestFile(t, config.ConfigFileName, "version: 1\nsources:\n  rules: specs/team/rules\n")
	cfg, _, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	watched := watchDirs(".", cfg)
	if err := os.Mkdir("specs", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("other", 0o755); err != nil {
		t.Fatal(err)
	}

	if isWatchNoise(fsnotify.Event{Name: "specs", Op: fsnotify.Create}, watched) {
		t.Error("a created dir leading to a missing input must not be noise")
	}
	if !isWatchNoise(fsnotify.Event{Name: "other", Op: fsnotify.Create}, watched) {
		t.Error("a created dir leading nowhere must be noise")
	}
	if !isWatchNoise(fsnotify.Event{Name: "specs", Op: fsnotify.Write}, watched) {
		t.Error("only the creation of a leading dir counts")
	}
}

func TestWatchAnchor_StopsAtNearestExistingDirInsideRoot(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	absDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join("a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, want string
		ok         bool
	}{
		{filepath.Join("a", "b", "c", "d"), filepath.Join("a", "b"), true},
		{filepath.Join(absDir, "a", "b", "c", "d"), filepath.Join(absDir, "a", "b"), true},
		{filepath.Join("x", "y", "z"), ".", true},
		{"agnostic-ai.yaml", ".", true},
		{filepath.Join("..", "missing-sibling", "rules"), "", false},
	} {
		got, ok := watchAnchor(".", tc.path)
		if got != tc.want || ok != tc.ok {
			t.Errorf("watchAnchor(%q) = %q, %v; want %q, %v", tc.path, got, ok, tc.want, tc.ok)
		}
	}
}

// An input created between the two registration phases gets its
// parent's watch too late to report it, so it must be watched itself.
func TestArmWatches_WatchesAnInputCreatedWhileArming(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	if err := os.MkdirAll(filepath.Join("specs", "team"), 0o755); err != nil {
		t.Fatal(err)
	}
	rules := filepath.Join("specs", "team", "rules")
	prev := armPause
	armPause = func() {
		if err := os.MkdirAll(rules, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { armPause = prev })
	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()

	if err := armWatches(w, ".", []string{rules}); err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(w.WatchList(), func(p string) bool { return filepath.Clean(p) == rules }) {
		t.Errorf("want %s watched, got %v", rules, w.WatchList())
	}
}

// A source beside the project must not put the folder holding the
// project, such as the home directory, under watch.
func TestWatchAnchor_NeverAnchorsOnAnAncestorOfRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "shared"), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, filepath.Join(dir, "proj"))

	if got, ok := watchAnchor(".", filepath.Join("..", "rules")); ok {
		t.Errorf("watchAnchor(../rules) = %q; want no anchor", got)
	}
	want := filepath.Join("..", "shared")
	if got, ok := watchAnchor(".", filepath.Join("..", "shared", "rules")); !ok || got != want {
		t.Errorf("watchAnchor(../shared/rules) = %q, %v; want %q, true", got, ok, want)
	}
}

// startWatch runs watchSync in the background, returns once the watcher
// is armed, and hands back its output. The returned stop cancels the
// watch and fails the test if it returned an error.
func startWatch(t *testing.T, targets []string, forcePoll bool) (*safeBuffer, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	buf := captureWatchOutput(t)
	done := make(chan error, 1)
	go func() {
		done <- watchSync(ctx, 20*time.Millisecond, ".", targets, false, false, "off", forcePoll, 1)
	}()
	waitForOutput(t, buf, "watching", 10*time.Second)
	return buf, func() {
		t.Helper()
		cancel()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

// waitForFileContaining blocks until path holds want, or fails.
func waitForFileContaining(t *testing.T, path, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var got []byte
	for time.Now().Before(deadline) {
		got, _ = os.ReadFile(path)
		if strings.Contains(string(got), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q in %s; got:\n%s", want, path, got)
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// assertWatchPicksUpNewLocalOverride creates agnostic-ai.local.yaml after
// the watch starts and waits for the full re-sync it must trigger.
func assertWatchPicksUpNewLocalOverride(t *testing.T, forcePoll bool) {
	t.Helper()
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	buf, stop := startWatch(t, []string{"claude"}, forcePoll)
	defer stop()

	writeTestFile(t, config.LocalOverrideFileName, "version: 1\n")
	waitForOutput(t, buf, "change · "+config.LocalOverrideFileName, 5*time.Second)
	waitForOutput(t, buf, "full re-sync", 5*time.Second)
}

func TestWatchSync_PicksUpLocalOverrideCreatedMidSession(t *testing.T) {
	assertWatchPicksUpNewLocalOverride(t, false)
}

func TestWatchSync_PollPicksUpLocalOverrideCreatedMidSession(t *testing.T) {
	assertWatchPicksUpNewLocalOverride(t, true)
}

// assertWatchPicksUpNewOverlayDir creates the overlay dir and a codex
// overlay after the watch starts and waits for its key in the output.
func assertWatchPicksUpNewOverlayDir(t *testing.T, forcePoll bool) {
	t.Helper()
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	_, stop := startWatch(t, []string{"codex"}, forcePoll)
	defer stop()

	writeTestFile(t, filepath.Join(agnosticOverlayDir, codexOverlayFile), "model = \"o4-late\"\n")
	waitForFileContaining(t, filepath.Join(dir, ".codex", "config.toml"), "o4-late", 5*time.Second)
}

func TestWatchSync_PicksUpOverlayDirCreatedMidSession(t *testing.T) {
	assertWatchPicksUpNewOverlayDir(t, false)
}

func TestWatchSync_PollPicksUpOverlayDirCreatedMidSession(t *testing.T) {
	assertWatchPicksUpNewOverlayDir(t, true)
}

// assertWatchPicksUpNewCommandsDir creates the commands source dir after
// the watch starts. Commands are a source kind like any other.
func assertWatchPicksUpNewCommandsDir(t *testing.T, forcePoll bool) {
	t.Helper()
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	_, stop := startWatch(t, []string{"claude"}, forcePoll)
	defer stop()

	writeTestFile(t, filepath.Join(".agnostic-ai", "commands", "deploy.md"),
		"---\nname: deploy\ndescription: deploy the app\n---\nrun deploy\n")
	waitForFileContaining(t, filepath.Join(dir, ".claude", "commands", "deploy.md"), "run deploy", 5*time.Second)
}

func TestWatchSync_PicksUpCommandsDirCreatedMidSession(t *testing.T) {
	assertWatchPicksUpNewCommandsDir(t, false)
}

func TestWatchSync_PollPicksUpCommandsDirCreatedMidSession(t *testing.T) {
	assertWatchPicksUpNewCommandsDir(t, true)
}

// assertWatchPicksUpSharedInstructionsEdit edits AGNOSTIC_AI.md after the
// watch starts and waits for the entry point to carry the new body.
func assertWatchPicksUpSharedInstructionsEdit(t *testing.T, forcePoll bool) {
	t.Helper()
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	_, stop := startWatch(t, []string{"claude"}, forcePoll)
	defer stop()

	writeTestFile(t, adapters.AgnosticEntryPointPath, "# Project\n\nedited shared instructions\n")
	waitForFileContaining(t, filepath.Join(dir, "CLAUDE.md"), "edited shared instructions", 5*time.Second)
}

func TestWatchSync_PicksUpSharedInstructionsEdit(t *testing.T) {
	assertWatchPicksUpSharedInstructionsEdit(t, false)
}

func TestWatchSync_PollPicksUpSharedInstructionsEdit(t *testing.T) {
	assertWatchPicksUpSharedInstructionsEdit(t, true)
}

func TestWatchError_OverflowEndsTheSessionAsALostWatch(t *testing.T) {
	silence(t)
	if err := watchError(fsnotify.ErrEventOverflow); !errors.Is(err, errWatchLost) {
		t.Errorf("overflow: got %v, want errWatchLost", err)
	}
	if err := watchError(errors.New("Windows system assumed buffer larger than it is, events have likely been missed")); !errors.Is(err, errWatchLost) {
		t.Errorf("windows missed events: got %v, want errWatchLost", err)
	}
	if err := watchError(errors.New("read failed")); err != nil {
		t.Errorf("other error: got %v, want the session kept", err)
	}
}

// assertWatchPicksUpNestedSourceDir points a source at a path whose
// parent is missing too, then creates the whole chain mid-session.
func assertWatchPicksUpNestedSourceDir(t *testing.T, forcePoll bool) {
	t.Helper()
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	writeTestFile(t, config.ConfigFileName, "version: 1\nsources:\n  rules: specs/team/rules\n")
	_, stop := startWatch(t, []string{"claude"}, forcePoll)
	defer stop()

	writeTestFile(t, filepath.Join("specs", "team", "rules", "r2.md"), "---\nname: r2\n---\nlate rule body\n")
	waitForFileContaining(t, filepath.Join(dir, ".claude", "rules", "r2.md"), "late rule body", 5*time.Second)
}

func TestWatchSync_PicksUpNestedSourceDirCreatedMidSession(t *testing.T) {
	assertWatchPicksUpNestedSourceDir(t, false)
}

func TestWatchSync_PollPicksUpNestedSourceDirCreatedMidSession(t *testing.T) {
	assertWatchPicksUpNestedSourceDir(t, true)
}

func assertWatchReEmitsAbsoluteSources(t *testing.T, forcePoll bool) {
	t.Helper()
	dir := testutil.TempCwd(t)
	silence(t)
	external := t.TempDir()
	rules := filepath.Join(external, "rules")
	skills := filepath.Join(external, "skills")
	rule := filepath.Join(rules, "demo.md")
	asset := filepath.Join(skills, "demo", "references", "guide.txt")
	writeTestFile(t, rule, "---\nname: demo\n---\nInitial external rule.\n")
	writeTestFile(t, filepath.Join(skills, "demo", "SKILL.md"),
		"---\nname: demo\ndescription: demo\n---\nExternal skill.\n")
	writeTestFile(t, asset, "Initial external asset.\n")
	writeTestFile(t, config.ConfigFileName, fmt.Sprintf(
		"version: 1\nsources:\n  rules: %q\n  skills: %q\ntargets: [claude]\n",
		filepath.ToSlash(rules), filepath.ToSlash(skills)))
	t.Run("rule", func(t *testing.T) {
		buf, stop := startWatch(t, nil, forcePoll)
		defer stop()
		if !forcePoll && !strings.Contains(buf.String(), "fsnotify") {
			t.Fatalf("fsnotify test fell back to polling:\n%s", buf.String())
		}
		writeAndBumpMtime(t, rule, []byte("---\nname: demo\n---\nUpdated external rule.\n"))
		waitForFileContaining(t, filepath.Join(dir, ".claude", "rules", "demo.md"), "Updated external rule.", 5*time.Second)
		waitForOutput(t, buf, "rule change · re-syncing 1 target: claude", 5*time.Second)
	})
	t.Run("skill asset", func(t *testing.T) {
		buf, stop := startWatch(t, nil, forcePoll)
		defer stop()
		if !forcePoll && !strings.Contains(buf.String(), "fsnotify") {
			t.Fatalf("fsnotify test fell back to polling:\n%s", buf.String())
		}
		writeAndBumpMtime(t, asset, []byte("Updated external asset.\n"))
		waitForFileContaining(t, filepath.Join(dir, ".claude", "skills", "demo", "references", "guide.txt"), "Updated external asset.", 5*time.Second)
		waitForOutput(t, buf, "skill change · re-syncing 1 target: claude", 5*time.Second)
	})
}

func TestWatchSync_ReEmitsAbsoluteSources(t *testing.T) {
	assertWatchReEmitsAbsoluteSources(t, false)
}

func TestWatchSync_PollReEmitsAbsoluteSources(t *testing.T) {
	assertWatchReEmitsAbsoluteSources(t, true)
}

func TestWatchSync_WatchesMissingAbsoluteSourceWithinProject(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	absDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	rules := filepath.Join(absDir, "specs", "team", "rules")
	writeTestFile(t, config.ConfigFileName, fmt.Sprintf("version: 1\nsources:\n  rules: %q\ntargets: [claude]\n", filepath.ToSlash(rules)))
	buf, stop := startWatch(t, nil, false)
	defer stop()
	if !strings.Contains(buf.String(), "fsnotify") {
		t.Fatalf("fsnotify test fell back to polling:\n%s", buf.String())
	}
	writeTestFile(t, filepath.Join(rules, "late.md"), "---\nname: late\n---\nLate absolute rule.\n")
	waitForFileContaining(t, filepath.Join(dir, ".claude", "rules", "late.md"), "Late absolute rule.", 5*time.Second)
}

func TestWatchSync_PollsMissingAbsoluteSourceWithoutSafeAnchor(t *testing.T) {
	workspace := t.TempDir()
	dir := filepath.Join(workspace, "project")
	rules := filepath.Join(workspace, "shared")
	writeTestFile(t, filepath.Join(dir, config.ConfigFileName), fmt.Sprintf("version: 1\nsources:\n  rules: %q\ntargets: [claude]\n", filepath.ToSlash(rules)))
	testutil.Chdir(t, dir)
	silence(t)
	buf, stop := startWatch(t, nil, false)
	defer stop()
	if !strings.Contains(buf.String(), "(poll)") {
		t.Fatalf("missing source without a safe anchor must use polling:\n%s", buf.String())
	}
	writeTestFile(t, filepath.Join(rules, "late.md"), "---\nname: late\n---\nLate external rule.\n")
	waitForFileContaining(t, filepath.Join(dir, ".claude", "rules", "late.md"), "Late external rule.", 5*time.Second)
}

func TestWatchSync_PollsMissingAbsoluteSourceUnderProjectAlias(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	alias := filepath.Join(t.TempDir(), "alias")
	if err := createImportSourceAlias(dir, alias); err != nil {
		t.Fatal(err)
	}
	rules := filepath.Join(alias, "rules")
	writeTestFile(t, config.ConfigFileName, fmt.Sprintf("version: 1\nsources:\n  rules: %q\ntargets: [claude]\n", filepath.ToSlash(rules)))
	buf, stop := startWatch(t, nil, false)
	defer stop()
	if !strings.Contains(buf.String(), "(poll)") {
		t.Fatalf("missing aliased source must use polling:\n%s", buf.String())
	}
	writeTestFile(t, filepath.Join(rules, "late.md"), "---\nname: late\n---\nLate aliased rule.\n")
	waitForFileContaining(t, filepath.Join(dir, ".claude", "rules", "late.md"), "Late aliased rule.", 5*time.Second)
}

// Watching the project root to see new inputs appear must not turn the
// files sync writes there into triggers, or every sync would start the
// next one.
func TestWatchSync_OwnWritesDoNotRetrigger(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	writeTestFile(t, config.ConfigFileName, "version: 1\ntargets: [claude]\n")
	buf, stop := startWatch(t, nil, false)
	defer stop()

	// The override adds targets, so the re-sync writes new files at the
	// root, right next to the inputs the root watch exists for.
	writeTestFile(t, config.LocalOverrideFileName, "targets: [claude, codex, gemini]\n")
	waitForFile(t, filepath.Join(dir, "AGENTS.md"), 5*time.Second)
	waitForFile(t, filepath.Join(dir, "GEMINI.md"), 5*time.Second)
	time.Sleep(600 * time.Millisecond)
	if n := strings.Count(buf.String(), "] change · "); n != 1 {
		t.Errorf("want exactly one re-sync, got %d:\n%s", n, buf.String())
	}
}

// A source tree moved away and recreated must not keep the watches of
// the moved directories, or edits in the new tree raise no event. A move
// and a recreate inside one kqueue diff raise no event for the moved
// name, so the watch must also notice the stale directories on the next
// directory it sees created.
func TestWatchSync_WatchesSourceTreeMovedAndRecreatedInOneStep(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows cannot rename a directory while it is watched")
	}
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	_, stop := startWatch(t, []string{"claude"}, false)
	defer stop()

	if err := os.Rename(".agnostic-ai", "moved"); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(".agnostic-ai", "rules", "r2.md"), "---\nname: r2\n---\nback rule body\n")
	waitForFileContaining(t, filepath.Join(dir, ".claude", "rules", "r2.md"), "back rule body", 5*time.Second)

	writeTestFile(t, filepath.Join(".agnostic-ai", "rules", "r3.md"), "---\nname: r3\n---\nlater rule body\n")
	waitForFileContaining(t, filepath.Join(dir, ".claude", "rules", "r3.md"), "later rule body", 5*time.Second)
}

func TestDropStaleWatches_FindsADirRecreatedUnderItsName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows cannot rename a directory while it is watched")
	}
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	rules := filepath.Join(".agnostic-ai", "rules")
	if err := os.MkdirAll(rules, 0o755); err != nil {
		t.Fatal(err)
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()
	if err := armWatches(w, ".", []string{rules}); err != nil {
		t.Fatal(err)
	}

	if err := os.Rename(".agnostic-ai", "moved"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rules, 0o755); err != nil {
		t.Fatal(err)
	}
	if stale := dropStaleWatches(w); !slices.Contains(stale, rules) {
		t.Errorf("stale = %v; want it to hold %s", stale, rules)
	}
	if err := armWatches(w, ".", []string{rules}); err != nil {
		t.Fatal(err)
	}
	if stale := dropStaleWatches(w); len(stale) != 0 {
		t.Errorf("after re-arming, stale = %v; want none", stale)
	}
}

// A directory removed before its watch is added is no lost watch: the
// session keeps its OS events instead of dropping to polling.
func TestWatchSync_StaysOnFsnotifyWhenNewDirIsGoneBeforeItsWatch(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	fail := failWatchAddWith(t, ".agnostic-ai/commands", fs.ErrNotExist)
	buf, stop := startWatch(t, []string{"claude"}, false)
	defer stop()
	fail.Store(true)

	writeTestFile(t, filepath.Join(".agnostic-ai", "commands", "deploy.md"),
		"---\nname: deploy\ndescription: deploy the app\n---\nrun deploy\n")
	waitForFileContaining(t, filepath.Join(dir, ".claude", "commands", "deploy.md"), "run deploy", 5*time.Second)
	if strings.Contains(buf.String(), "(poll)") {
		t.Errorf("want the fsnotify watch kept, got:\n%s", buf.String())
	}
}

// failWatchAdd makes watcher registration fail for any path containing
// marker once the returned switch is on, the way an exhausted inotify
// limit fails partway through a session.
func failWatchAdd(t *testing.T, marker string) *atomic.Bool {
	return failWatchAddWith(t, marker, errors.New("no space left on device"))
}

// failWatchAddWith is failWatchAdd with the error registration returns.
func failWatchAddWith(t *testing.T, marker string, failure error) *atomic.Bool {
	t.Helper()
	var on atomic.Bool
	prev := watchAdd
	watchAdd = func(w *fsnotify.Watcher, p string) error {
		if on.Load() && strings.Contains(filepath.ToSlash(p), marker) {
			return failure
		}
		return prev(w, p)
	}
	t.Cleanup(func() { watchAdd = prev })
	return &on
}

// A directory the watch cannot register when it appears must not go
// dark: its first file syncs, and so does every later one.
func TestWatchSync_KeepsWatchingWhenNewDirCannotBeRegistered(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	fail := failWatchAdd(t, ".agnostic-ai/commands")
	_, stop := startWatch(t, []string{"claude"}, false)
	defer stop()
	fail.Store(true)

	writeTestFile(t, filepath.Join(".agnostic-ai", "commands", "deploy.md"),
		"---\nname: deploy\ndescription: deploy the app\n---\nrun deploy\n")
	waitForFileContaining(t, filepath.Join(dir, ".claude", "commands", "deploy.md"), "run deploy", 5*time.Second)

	writeTestFile(t, filepath.Join(".agnostic-ai", "commands", "ship.md"),
		"---\nname: ship\ndescription: ship the app\n---\nrun ship\n")
	waitForFileContaining(t, filepath.Join(dir, ".claude", "commands", "ship.md"), "run ship", 5*time.Second)
}

// A source dir the watch cannot register after a config reload must not
// go dark either: later edits in it still sync.
func TestWatchSync_KeepsWatchingWhenReloadedSourceCannotBeRegistered(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	writeTestFile(t, filepath.Join("extra", "rules", "r2.md"), "---\nname: r2\n---\nextra rule body\n")
	fail := failWatchAdd(t, "extra")
	_, stop := startWatch(t, []string{"claude"}, false)
	defer stop()
	fail.Store(true)

	writeTestFile(t, config.ConfigFileName, "version: 1\nsources:\n  rules: extra/rules\n")
	waitForFileContaining(t, filepath.Join(dir, ".claude", "rules", "r2.md"), "extra rule body", 5*time.Second)

	writeTestFile(t, filepath.Join("extra", "rules", "r3.md"), "---\nname: r3\n---\nlater rule body\n")
	waitForFileContaining(t, filepath.Join(dir, ".claude", "rules", "r3.md"), "later rule body", 5*time.Second)
}

func TestResyncForChanges_IgnoresHandoffBeforeLoadingConfig(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	writeTestFile(t, config.ConfigFileName, "invalid: [")
	for _, name := range []string{"HANDOFF.md", "HANDOFF.auto.md", "HANDOFF.auto.md.a1B2c3"} {
		path := filepath.Join(dir, defaultProjectUser, name)
		if err := resyncForChanges(dir, []string{"claude"}, []string{path}, false, false, "off", 1); err != nil {
			t.Errorf("ignored %s loaded broken config: %v", name, err)
		}
	}
}

func TestResyncForChanges_MixedHandoffAndRuleSyncsRule(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	buf := captureWatchOutput(t)
	rule := filepath.Join(dir, ".agnostic-ai", "rules", "r1.md")
	writeTestFile(t, rule, "---\nname: r1\n---\nUpdated rule.\n")
	if err := resyncForChanges(dir, []string{"claude"}, []string{filepath.Join(dir, defaultProjectUser, "HANDOFF.md"), filepath.Join(dir, defaultProjectUser, "HANDOFF.auto.md.a1B2c3"), rule}, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "rules", "r1.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Updated rule.") {
		t.Errorf("rule did not sync: %s", data)
	}
	if strings.Contains(buf.String(), "full re-sync") {
		t.Errorf("ignored handoff forced full sync: %s", buf.String())
	}
}

func TestResyncForChanges_HandoffNamedRuleSyncs(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	path := filepath.Join(dir, ".agnostic-ai", "rules", "HANDOFF.md")
	writeTestFile(t, path, "---\nname: handoff-rule\n---\nKeep this ordinary rule.\n")
	if err := resyncForChanges(dir, []string{"claude"}, []string{path}, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "rules", "handoff-rule.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Keep this ordinary rule.") {
		t.Errorf("handoff-named rule did not sync: %s", data)
	}
}

func TestIsIgnoredEvent_HandoffPaths(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	for _, name := range []string{"HANDOFF.md", "HANDOFF.auto.md", "HANDOFF.auto.md.a1B2c3"} {
		for _, op := range []fsnotify.Op{fsnotify.Create, fsnotify.Write, fsnotify.Rename, fsnotify.Remove, fsnotify.Create | fsnotify.Write} {
			for _, path := range []string{filepath.Join(defaultProjectUser, name), filepath.Join(dir, defaultProjectUser, name)} {
				root := "."
				if filepath.IsAbs(path) {
					root = dir
				}
				if !isIgnoredEvent(fsnotify.Event{Name: path, Op: op}, root) {
					t.Errorf("handoff %s event not ignored: %s", op, path)
				}
			}
			for _, path := range []string{filepath.Join(".agnostic-ai", "rules", name), filepath.Join(defaultProjectUser, "rules", name), filepath.Join("child", defaultProjectUser, name)} {
				if isIgnoredEvent(fsnotify.Event{Name: path, Op: op}, ".") {
					t.Errorf("legitimate %s event ignored: %s", op, path)
				}
			}
		}
	}
}

func TestWatchSync_HandoffChangesDoNotResync(t *testing.T) {
	for _, forcePoll := range []bool{false, true} {
		t.Run(map[bool]string{false: "fsnotify", true: "poll"}[forcePoll], func(t *testing.T) {
			dir := setupFixture(t)
			testutil.Chdir(t, dir)
			silence(t)
			for _, name := range []string{"HANDOFF.md", "HANDOFF.auto.md"} {
				writeTestFile(t, filepath.Join(dir, defaultProjectUser, name), "Initial handoff.\n")
			}
			buf, stop := startWatch(t, []string{"claude"}, forcePoll)
			defer stop()
			if !forcePoll && !strings.Contains(buf.String(), "fsnotify") {
				t.Fatalf("fsnotify test fell back to polling: %s", buf.String())
			}
			for _, name := range []string{"HANDOFF.md", "HANDOFF.auto.md"} {
				before := buf.String()
				writeAndBumpMtime(t, filepath.Join(dir, defaultProjectUser, name), []byte("Updated handoff.\n"))
				time.Sleep(200 * time.Millisecond)
				if got := strings.TrimPrefix(buf.String(), before); strings.Contains(got, "re-sync") {
					t.Errorf("%s triggered sync: %s", name, got)
				}
			}
			auto := filepath.Join(dir, defaultProjectUser, "HANDOFF.auto.md")
			temp := filepath.Join(dir, defaultProjectUser, "HANDOFF.auto.md.a1B2c3")
			for _, stage := range []struct {
				name string
				run  func()
			}{
				{"create", func() { writeTestFile(t, temp, "Incomplete snapshot.\n") }},
				{"write", func() { writeAndBumpMtime(t, temp, []byte("Complete snapshot.\n")) }},
				{"replace", func() {
					if err := os.Remove(auto); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(temp, auto); err != nil {
						t.Fatal(err)
					}
				}},
				{"create-for-cleanup", func() { writeTestFile(t, temp, "Failed snapshot.\n") }},
				{"remove", func() {
					if err := os.Remove(temp); err != nil {
						t.Fatal(err)
					}
				}},
			} {
				before := buf.String()
				stage.run()
				time.Sleep(200 * time.Millisecond)
				if got := strings.TrimPrefix(buf.String(), before); strings.Contains(got, "re-sync") {
					t.Errorf("snapshot temporary-file %s triggered sync: %s", stage.name, got)
				}
			}
			outside := filepath.Join(dir, ".agnostic-ai", "rules", "HANDOFF.auto.md.a1B2c3")
			writeTestFile(t, outside, "Ordinary watched file.\n")
			waitForOutput(t, buf, "full re-sync", 3*time.Second)
			rule := filepath.Join(dir, ".agnostic-ai", "rules", "r1.md")
			writeAndBumpMtime(t, rule, []byte("---\nname: r1\n---\nOrdinary change.\n"))
			waitForFileContaining(t, filepath.Join(dir, ".claude", "rules", "r1.md"), "Ordinary change.", 3*time.Second)
		})
	}
}

func TestWatchSync_LocalDirectoryWritesTrackFiles(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	writeTestFile(t, config.ConfigFileName, "version: 1\ntargets: [claude]\n")
	local := defaultProjectUser
	rules := filepath.Join(local, "rules")
	rule := filepath.Join(rules, "directory-rule.md")
	writeTestFile(t, rule, "---\nname: directory-rule\n---\nInitial local rule.\n")
	writeTestFile(t, filepath.Join(local, "HANDOFF.auto.md"), "Initial snapshot.\n")
	watcherReady := make(chan *fsnotify.Watcher, 1)
	var directoryOnly atomic.Bool
	prev := watchAdd
	watchAdd = func(w *fsnotify.Watcher, p string) error {
		select {
		case watcherReady <- w:
		default:
		}
		if directoryOnly.Load() && pathWithin(rules, p) {
			return nil
		}
		return prev(w, p)
	}
	t.Cleanup(func() { watchAdd = prev })
	buf, stop := startWatch(t, []string{"claude"}, false)
	defer stop()
	if !strings.Contains(buf.String(), "fsnotify") {
		t.Fatalf("fsnotify test fell back to polling: %s", buf.String())
	}
	watcher := <-watcherReady
	localWatch := ""
	for _, path := range watcher.WatchList() {
		if pathWithin(local, path) && pathWithin(path, local) {
			localWatch = path
			break
		}
	}
	if localWatch == "" {
		t.Fatalf("local directory is not registered: %v", watcher.WatchList())
	}
	directoryWrite := func() {
		t.Helper()
		select {
		case watcher.Events <- fsnotify.Event{Name: localWatch, Op: fsnotify.Write}:
		case <-time.After(time.Second):
			t.Fatal("watcher did not receive directory event")
		}
	}
	temp := filepath.Join(local, "HANDOFF.auto.md.a1B2c3")
	for _, stage := range []struct {
		name string
		run  func()
	}{
		{"unchanged", func() {}},
		{"create", func() { writeTestFile(t, temp, "Incomplete snapshot.\n") }},
		{"write", func() { writeAndBumpMtime(t, temp, []byte("Complete snapshot.\n")) }},
		{"remove", func() {
			if err := os.Remove(temp); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		before := buf.String()
		stage.run()
		directoryWrite()
		time.Sleep(200 * time.Millisecond)
		if got := strings.TrimPrefix(buf.String(), before); strings.Contains(got, "re-sync") {
			t.Errorf("snapshot-only directory %s triggered sync: %s", stage.name, got)
		}
	}
	directoryOnly.Store(true)
	dropWatchesUnder(watcher, rules)
	writeAndBumpMtime(t, rule, []byte("---\nname: directory-rule\n---\nUpdated local rule.\n"))
	directoryWrite()
	waitForFileContaining(t, filepath.Join(dir, ".claude", "rules", "directory-rule.md"), "Updated local rule.", 3*time.Second)
	added := filepath.Join(rules, "added-rule.md")
	writeTestFile(t, added, "---\nname: added-rule\n---\nAdded local rule.\n")
	directoryWrite()
	waitForFileContaining(t, filepath.Join(dir, ".claude", "rules", "added-rule.md"), "Added local rule.", 3*time.Second)
	renamed := filepath.Join(rules, "renamed-rule.md")
	if err := os.Rename(added, renamed); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, renamed, "---\nname: renamed-rule\n---\nRenamed local rule.\n")
	directoryWrite()
	waitForFileContaining(t, filepath.Join(dir, ".claude", "rules", "renamed-rule.md"), "Renamed local rule.", 3*time.Second)
	waitForRemoval := func(path string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Errorf("orphan rule output remains: %s", path)
	}
	waitForRemoval(filepath.Join(dir, ".claude", "rules", "added-rule.md"))
	if err := os.Remove(renamed); err != nil {
		t.Fatal(err)
	}
	directoryWrite()
	waitForRemoval(filepath.Join(dir, ".claude", "rules", "renamed-rule.md"))
}

func TestWatchSync_LocalRuleEditedWhileArmingIsReconciled(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	writeTestFile(t, config.ConfigFileName, "version: 1\ntargets: [claude]\n")
	local := defaultProjectUser
	rules := filepath.Join(local, "rules")
	rule := filepath.Join(rules, "registration-rule.md")
	writeTestFile(t, rule, "---\nname: registration-rule\n---\nBefore registration.\n")
	prevAdd := watchAdd
	watchAdd = func(w *fsnotify.Watcher, p string) error {
		if pathWithin(rules, p) {
			return nil
		}
		return prevAdd(w, p)
	}
	t.Cleanup(func() { watchAdd = prevAdd })
	var edited atomic.Bool
	prevPause := armPause
	armPause = func() {
		if edited.CompareAndSwap(false, true) {
			writeAndBumpMtime(t, rule, []byte("---\nname: registration-rule\n---\nEdited during registration.\n"))
		}
	}
	t.Cleanup(func() { armPause = prevPause })
	buf, stop := startWatch(t, []string{"claude"}, false)
	defer stop()
	if !strings.Contains(buf.String(), "fsnotify") {
		t.Fatalf("fsnotify test fell back to polling: %s", buf.String())
	}
	output, err := os.ReadFile(filepath.Join(dir, ".claude", "rules", "registration-rule.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "Edited during registration.") {
		t.Errorf("ready watcher retained stale rule: %s", output)
	}
}
