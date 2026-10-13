package claude

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func readLocalSettings(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(".claude", "settings.local.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestEmit_PointsAutoMemoryAtThePersonalStore(t *testing.T) {
	testutil.TempCwd(t)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{Builtins: []string{"memory"}}, false); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(wd, ".agnostic-ai", "local", "memory")
	if got := readLocalSettings(t)["autoMemoryDirectory"]; got != filepath.ToSlash(want) {
		t.Errorf("autoMemoryDirectory = %v, want %s", got, filepath.ToSlash(want))
	}
}

func TestEmit_LeavesAutoMemoryAloneWithoutTheMemoryBuiltin(t *testing.T) {
	testutil.TempCwd(t)

	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(".claude", "settings.local.json")); !os.IsNotExist(err) {
		t.Errorf("settings.local.json written without the memory built-in: %v", err)
	}
}

func TestEmit_KeepsTheUsersOwnAutoMemoryDirectory(t *testing.T) {
	testutil.TempCwd(t)
	if err := os.MkdirAll(".claude", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".claude", "settings.local.json"), []byte(`{"autoMemoryDirectory": "~/notes", "theme": "dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{Builtins: []string{"memory"}}, false); err != nil {
		t.Fatal(err)
	}

	doc := readLocalSettings(t)
	if doc["autoMemoryDirectory"] != "~/notes" || doc["theme"] != "dark" {
		t.Errorf("user settings changed: %v", doc)
	}
}

func claimAutoMemoryDirectory(t *testing.T) {
	t.Helper()
	emit.PriorMergedKeys = func(string) []emit.MergedKey {
		return []emit.MergedKey{{Path: []string{autoMemoryDirectoryKey}}}
	}
	t.Cleanup(func() { emit.PriorMergedKeys = nil })
}

func TestEmit_QueuesASeedFromClaudesOwnStoreTheFirstTimeItPointsClaudeAway(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("CLAUDE_CODE_PROJECT_DIR_NAME", "")
	testutil.TempCwd(t)
	emit.TakeMemorySeeds()

	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{Builtins: []string{"memory"}}, false); err != nil {
		t.Fatal(err)
	}

	native, err := Adapter{}.NativeMemoryDir(".")
	if err != nil {
		t.Fatal(err)
	}
	store := readLocalSettings(t)["autoMemoryDirectory"]
	seeds := emit.TakeMemorySeeds()
	if len(seeds) != 1 || seeds[0].From != native || filepath.ToSlash(seeds[0].To) != store || seeds[0].Target != "claude" {
		t.Errorf("seeds = %+v, want one from %s to %v", seeds, native, store)
	}
}

func TestEmit_QueuesNoSeedInADryRun(t *testing.T) {
	testutil.TempCwd(t)
	emit.TakeMemorySeeds()

	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{Builtins: []string{"memory"}}, true); err != nil {
		t.Fatal(err)
	}

	if seeds := emit.TakeMemorySeeds(); len(seeds) != 0 {
		t.Errorf("dry run queued %+v", seeds)
	}
}

func TestEmit_QueuesNoSeedOnceSyncOwnsTheKey(t *testing.T) {
	testutil.TempCwd(t)
	claimAutoMemoryDirectory(t)
	emit.TakeMemorySeeds()

	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{Builtins: []string{"memory"}}, false); err != nil {
		t.Fatal(err)
	}

	if seeds := emit.TakeMemorySeeds(); len(seeds) != 0 {
		t.Errorf("a later sync queued %+v", seeds)
	}
}

func TestEmit_NamesTheFactsClaudeStopsReadingWhenTheBuiltinGoesAway(t *testing.T) {
	dir := testutil.TempCwd(t)
	store := filepath.Join(dir, ".agnostic-ai", "local", "memory")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"MEMORY.md", "tabs.md", "ci.md"} {
		if err := os.WriteFile(filepath.Join(store, name), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(".claude", 0o755); err != nil {
		t.Fatal(err)
	}
	settings := `{"autoMemoryDirectory": "` + filepath.ToSlash(store) + `"}`
	if err := os.WriteFile(filepath.Join(".claude", "settings.local.json"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
	claimAutoMemoryDirectory(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	var log strings.Builder
	prev := emit.Warner
	emit.Warner = &log
	t.Cleanup(func() { emit.Warner = prev })
	emit.FlushCoverageNotes()
	if !strings.Contains(log.String(), "no longer reads the 2 fact(s) in "+filepath.ToSlash(store)) {
		t.Errorf("notes:\n%s", log.String())
	}
}

// The file holds an absolute checkout path, so sync keeps it out of Git
// even before Claude Code ignores it.
func TestEmit_ExcludesLocalSettingsFromGit(t *testing.T) {
	testutil.TempCwd(t)
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git init: %v %s", err, out)
	}

	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{Builtins: []string{"memory"}}, false); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("git", "check-ignore", "-q", filepath.Join(".claude", "settings.local.json"))
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Errorf("settings.local.json is not ignored: %v", err)
	}
}

func TestEmit_QueuesNoSeedWhenTheMainWorktreeRedirectsAutoMemory(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	repo := canonicalPath(t.TempDir())
	gitIn(t, repo, "init", "-q")
	gitIn(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	settings, err := json.Marshal(map[string]string{autoMemoryDirectoryKey: filepath.Join(repo, ".agnostic-ai", "local", "memory")})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".claude", "settings.local.json"), append([]byte("// Claude settings\n"), settings...), 0o644); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(t.TempDir(), "linked")
	gitIn(t, repo, "worktree", "add", "-q", linked)
	testutil.Chdir(t, linked)
	emit.TakeMemorySeeds()

	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{Builtins: []string{"memory"}}, false); err != nil {
		t.Fatal(err)
	}

	if seeds := emit.TakeMemorySeeds(); len(seeds) != 0 {
		t.Errorf("a linked worktree queued %+v", seeds)
	}
}

func TestEmit_QueuesASeedWhenTheMainWorktreeStillUsesNativeMemory(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	repo := canonicalPath(t.TempDir())
	gitIn(t, repo, "init", "-q")
	gitIn(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	linked := filepath.Join(t.TempDir(), "linked")
	gitIn(t, repo, "worktree", "add", "-q", linked)
	testutil.Chdir(t, linked)
	emit.TakeMemorySeeds()
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{Builtins: []string{"memory"}}, false); err != nil {
		t.Fatal(err)
	}
	native, err := Adapter{}.NativeMemoryDir(".")
	if err != nil {
		t.Fatal(err)
	}
	seeds := emit.TakeMemorySeeds()
	if len(seeds) != 1 || seeds[0].From != native {
		t.Errorf("seeds = %+v, want native store %s", seeds, native)
	}
}

func TestEmit_QueuesNoSeedWhenTheMainWorktreeRedirectsToAHomeRelativeStore(t *testing.T) {
	cases := []struct {
		name, scope string
		comments    bool
	}{
		{"local JSONC", "settings.local.json", true},
		{"project JSONC", "settings.json", true},
		{"local JSON", "settings.local.json", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
			repo := canonicalPath(t.TempDir())
			gitIn(t, repo, "init", "-q")
			gitIn(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
			if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o755); err != nil {
				t.Fatal(err)
			}
			settings := []byte("{\n\"autoMemoryDirectory\": \"~/notes\"\n}\n")
			if tc.comments {
				settings = append([]byte("// Claude settings\n"), settings...)
			}
			if err := os.WriteFile(filepath.Join(repo, ".claude", tc.scope), settings, 0o644); err != nil {
				t.Fatal(err)
			}
			linked := filepath.Join(t.TempDir(), "linked")
			gitIn(t, repo, "worktree", "add", "-q", linked)
			testutil.Chdir(t, linked)
			emit.TakeMemorySeeds()
			if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{Builtins: []string{"memory"}}, false); err != nil {
				t.Fatal(err)
			}
			if seeds := emit.TakeMemorySeeds(); len(seeds) != 0 {
				t.Errorf("main home-relative store queued %+v", seeds)
			}
		})
	}
}
