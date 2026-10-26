package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestSyncGlobal_RemovesAUserFileItCreatedOnceEmpty(t *testing.T) {
	cases := map[string]struct {
		target, file string
		specs        map[string]string
	}{
		"augment hooks and mcp":     {"augment", ".augment/settings.json", map[string]string{"hooks/start.yaml": "event: SessionStart\ncommand: ~/s.sh\n", "mcps/docs.yaml": globalDocsMCP}},
		"claude hooks and settings": {"claude", ".claude/settings.json", map[string]string{"hooks/start.yaml": "event: SessionStart\ncommand: echo hi\n", "settings/d.yaml": "model: opus\n"}},
		"gemini settings and mcp":   {"gemini", ".gemini/settings.json", map[string]string{"settings/d.yaml": "model:\n  gemini: gemini-3-pro\n", "mcps/docs.yaml": globalDocsMCP}},
		"codex settings only":       {"codex", ".codex/config.toml", map[string]string{"settings/d.yaml": "model:\n  codex: gpt-6-luna\n"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			for rel, body := range tc.specs {
				mustWriteGlobalTest(t, filepath.Join(source, filepath.FromSlash(rel)), body)
			}
			if _, w, err := runGlobalAgentTest("--only", tc.target); err != nil {
				t.Fatalf("sync: %v\n%s", err, w)
			}
			path := filepath.Join(home, filepath.FromSlash(tc.file))
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("sync must create %s: %v", tc.file, err)
			}
			for rel := range tc.specs {
				if err := os.Remove(filepath.Join(source, filepath.FromSlash(rel))); err != nil {
					t.Fatal(err)
				}
			}
			if _, w, err := runGlobalAgentTest("--only", tc.target); err != nil {
				t.Fatalf("sync after removal: %v\n%s", err, w)
			}
			if data, err := os.ReadFile(path); err == nil {
				t.Errorf("%s must be gone, holds %q", tc.file, data)
			}
			if _, _, err := runGlobalAgentTest("--only", tc.target, "--check"); err != nil {
				t.Errorf("check after removal: %v", err)
			}
		})
	}
}

func TestSyncGlobal_KeepsACreatedFileTheUserAddedTo(t *testing.T) {
	home, source := globalAgentTestHome(t)
	spec := filepath.Join(source, "settings", "d.yaml")
	mustWriteGlobalTest(t, spec, "model: opus\n")
	if _, w, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	path := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, path, "{\n  \"model\": \"opus\",\n  \"theme\": \"dark\"\n}\n")
	if err := os.Remove(spec); err != nil {
		t.Fatal(err)
	}
	if _, w, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync after removal: %v\n%s", err, w)
	}
	if got := readGlobalTest(t, path); got != "{\n  \"theme\": \"dark\"\n}\n" {
		t.Errorf("the user's key must stay: %q", got)
	}
}

// A created file under a root variable that later moves is expected,
// not proof of another HOME, and its key moves with the root.
func TestSyncGlobal_CreatedFileUnderMovedRootIsNotForeign(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "d.yaml"), "model:\n  codex: gpt-6-luna\n")
	elsewhere := filepath.Join(t.TempDir(), "codex")
	t.Setenv("CODEX_HOME", elsewhere)
	if _, w, err := runGlobalAgentTest("--only", "codex"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	t.Setenv("CODEX_HOME", "")
	if _, w, err := runGlobalAgentTest("--only", "codex"); err != nil {
		t.Fatalf("sync after the root moved back: %v\n%s", err, w)
	}
	if _, err := os.Stat(filepath.Join(elsewhere, "config.toml")); !os.IsNotExist(err) {
		t.Errorf("the file sync created under the old root must go: %v", err)
	}
	if got := readGlobalTest(t, filepath.Join(home, ".codex", "config.toml")); got != "model = \"gpt-6-luna\"\n" {
		t.Errorf("config.toml = %q", got)
	}
}

// A file the user had before the first sync stays, even when empty.
func TestSyncGlobal_KeepsAPreexistingEmptyFile(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, path, "{}\n")
	hook := filepath.Join(source, "hooks", "start.yaml")
	settings := filepath.Join(source, "settings", "d.yaml")
	mustWriteGlobalTest(t, hook, "event: SessionStart\ncommand: echo hi\n")
	mustWriteGlobalTest(t, settings, "model: opus\n")
	if _, w, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	for _, p := range []string{hook, settings} {
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
	if _, w, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync after removal: %v\n%s", err, w)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("a file the user had must stay: %v", err)
	}
}

func TestApplyGlobalChanges_RemovalStopsWhenFileChangedSincePlan(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	mustWriteGlobalTest(t, path, "{\"a\": 1}\n")
	guards := map[string]*diskSnapshot{path: {data: []byte("{}\n")}}
	if _, err := applyGlobalChanges(nil, []string{path}, false, guards); err == nil {
		t.Fatal("a changed file must not be removed")
	}
	if got := readGlobalTest(t, path); got != "{\"a\": 1}\n" {
		t.Errorf("file = %q", got)
	}
}

func symlinkedGlobalTestFile(t *testing.T, body string) (link, target string) {
	t.Helper()
	dir := t.TempDir()
	target = filepath.Join(dir, "dotfiles", "settings.json")
	mustWriteGlobalTest(t, target, body)
	link = filepath.Join(dir, "settings.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	return link, target
}

func TestApplyGlobalChanges_SymlinkRemovalStopsWhenTargetChangedSincePlan(t *testing.T) {
	link, target := symlinkedGlobalTestFile(t, "{\"theme\":\"dark\"}\n")
	guards := map[string]*diskSnapshot{link: {data: []byte("{}\n")}}
	if _, err := applyGlobalChanges(nil, []string{link}, false, guards); err == nil || !strings.Contains(err.Error(), "changed while sync ran") {
		t.Fatalf("a changed target must not be removed: %v", err)
	}
	if got := readGlobalTest(t, target); got != "{\"theme\":\"dark\"}\n" {
		t.Errorf("target = %q", got)
	}
	if got, err := os.Readlink(link); err != nil || got != target {
		t.Errorf("the symlink must stay: %q, %v", got, err)
	}
}

func TestApplyGlobalChanges_SymlinkRemovalKeepsUnchangedTargetBehavior(t *testing.T) {
	link, target := symlinkedGlobalTestFile(t, "{}\n")
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	guards := map[string]*diskSnapshot{link: {data: []byte("{}\n")}}
	linked, err := applyGlobalChanges(nil, []string{link}, false, guards)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{target, link} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Errorf("%s must be gone: %v", path, err)
		}
	}
	if len(linked) != 1 || linked[0] != "removed "+resolved+" and its symlink "+link {
		t.Errorf("linked = %q", linked)
	}
}

func TestApplyGlobalChanges_BrokenSymlinkRemovalTouchesOnlyTheLink(t *testing.T) {
	link, target := symlinkedGlobalTestFile(t, "{}\n")
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(filepath.Dir(target), "other.json")
	mustWriteGlobalTest(t, other, "{}\n")
	guards := map[string]*diskSnapshot{link: {data: []byte("{}\n")}}
	if _, err := applyGlobalChanges(nil, []string{link}, false, guards); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("the broken link must go: %v", err)
	}
	if got := readGlobalTest(t, other); got != "{}\n" {
		t.Errorf("an unrelated file must stay: %q", got)
	}
}

func TestApplyGlobalChanges_LaterRefusalRestoresRemovedSymlinkAndTarget(t *testing.T) {
	link, target := symlinkedGlobalTestFile(t, "{}\n")
	changed := filepath.Join(t.TempDir(), "settings.json")
	mustWriteGlobalTest(t, changed, "{\"a\": 1}\n")
	guards := map[string]*diskSnapshot{link: {data: []byte("{}\n")}, changed: {data: []byte("{}\n")}}
	if _, err := applyGlobalChanges(nil, []string{link, changed}, false, guards); err == nil {
		t.Fatal("a changed file must stop the removal")
	}
	if got := readGlobalTest(t, target); got != "{}\n" {
		t.Errorf("target = %q", got)
	}
	if got, err := os.Readlink(link); err != nil || got != target {
		t.Errorf("the symlink must come back: %q, %v", got, err)
	}
	if got := readGlobalTest(t, changed); got != "{\"a\": 1}\n" {
		t.Errorf("changed file = %q", got)
	}
}

func TestSyncGlobal_PlannedGuardStopsRemovingAChangedSymlinkedFile(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "d.yaml"), "model: opus\n")
	if _, w, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	link := filepath.Join(home, ".claude", "settings.json")
	target := filepath.Join(home, "dotfiles", "settings.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(link, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	old, err := loadGlobalState(filepath.Join(source, "state", "global.json"))
	if err != nil {
		t.Fatal(err)
	}
	writes, next, err := buildGlobalWrites(home, source, []string{"claude"}, nil, spec.NewBundle(nil), old, nil, io.Discard, "warn")
	if err != nil {
		t.Fatal(err)
	}
	removals := removedGlobalFiles(old.Files, next.Files)
	if !strings.Contains(strings.Join(removals, "\n"), link) || next.removalGuards[link] == nil {
		t.Fatalf("the plan must remove %s under a guard: removals %q, guards %v", link, removals, next.removalGuards)
	}
	edited := "{\n  \"theme\": \"dark\"\n}\n"
	mustWriteGlobalTest(t, target, edited)
	if _, err := applyGlobalChanges(writes, removals, false, next.removalGuards); err == nil || !strings.Contains(err.Error(), "changed while sync ran") {
		t.Fatalf("stale plan accepted: %v", err)
	}
	if got := readGlobalTest(t, target); got != edited {
		t.Errorf("the edit must survive: %q", got)
	}
	if got, err := os.Readlink(link); err != nil || got != target {
		t.Errorf("the symlink must stay: %q, %v", got, err)
	}
}
