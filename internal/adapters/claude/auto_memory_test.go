package claude

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// autoMemoryProject starts a project with no Claude Code setting of its
// own and returns the user settings folder.
func autoMemoryProject(t *testing.T) string {
	t.Helper()
	testutil.TempCwd(t)
	user := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", user)
	t.Setenv("CLAUDE_CODE_DISABLE_AUTO_MEMORY", "")
	return user
}

func writeClaudeSettings(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAutoMemoryLoadsPersonalIndex_FollowsWhatClaudeCodeReads(t *testing.T) {
	local := filepath.Join(".claude", "settings.local.json")
	project := filepath.Join(".claude", "settings.json")
	for _, c := range []struct {
		name     string
		settings map[string]string
		env      string
		want     bool
	}{
		{name: "sync writes the folder", want: true},
		{name: "off in local settings", settings: map[string]string{local: `{"autoMemoryEnabled": false}`}},
		{name: "off in project settings", settings: map[string]string{project: `{"autoMemoryEnabled": false}`}},
		{name: "off in user settings", settings: map[string]string{"user": `{"autoMemoryEnabled": false}`}},
		{name: "off in the environment", env: "1"},
		{name: "local settings win over user settings", settings: map[string]string{"user": `{"autoMemoryEnabled": false}`, local: `{"autoMemoryEnabled": true}`}, want: true},
		{name: "the environment forces it on", settings: map[string]string{project: `{"autoMemoryEnabled": false}`}, env: "0", want: true},
		{name: "the user's folder elsewhere", settings: map[string]string{local: `{"autoMemoryDirectory": "~/notes"}`}},
		{name: "an empty user's folder", settings: map[string]string{local: `{"autoMemoryDirectory": null}`}},
	} {
		t.Run(c.name, func(t *testing.T) {
			user := autoMemoryProject(t)
			t.Setenv("CLAUDE_CODE_DISABLE_AUTO_MEMORY", c.env)
			for path, body := range c.settings {
				if path == "user" {
					path = filepath.Join(user, "settings.json")
				}
				writeClaudeSettings(t, path, body)
			}

			got, err := AutoMemoryLoadsPersonalIndex(&config.Config{Builtins: []string{"memory"}})
			if err != nil || got != c.want {
				t.Errorf("got %v, %v; want %v", got, err, c.want)
			}
		})
	}
}

func TestAutoMemoryLoadsPersonalIndex_ReadsTheUserSettingsUnderHome(t *testing.T) {
	autoMemoryProject(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	writeClaudeSettings(t, filepath.Join(home, ".claude", "settings.json"), `{"autoMemoryEnabled": false}`)

	if got, err := AutoMemoryLoadsPersonalIndex(&config.Config{Builtins: []string{"memory"}}); err != nil || got {
		t.Errorf("got %v, %v; want auto memory off", got, err)
	}
}

func TestAutoMemoryLoadsPersonalIndex_AfterSyncWritesTheFolder(t *testing.T) {
	autoMemoryProject(t)
	cfg := &config.Config{Builtins: []string{"memory"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}

	if got, err := AutoMemoryLoadsPersonalIndex(cfg); err != nil || !got {
		t.Errorf("got %v, %v; want the folder sync wrote to count", got, err)
	}
}

func TestAutoMemoryLoadsPersonalIndex_WhenTheUsersFolderIsThePersonalStore(t *testing.T) {
	autoMemoryProject(t)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	value, err := json.Marshal(map[string]string{"autoMemoryDirectory": filepath.ToSlash(filepath.Join(wd, ".agnostic-ai", "local", "memory"))})
	if err != nil {
		t.Fatal(err)
	}
	writeClaudeSettings(t, filepath.Join(".claude", "settings.local.json"), string(value))

	if got, err := AutoMemoryLoadsPersonalIndex(&config.Config{Builtins: []string{"memory"}}); err != nil || !got {
		t.Errorf("got %v, %v; want the personal store to count", got, err)
	}
}

// A folder an earlier sync wrote is sync's to replace, as after a switch
// to memory.personal: repo.
func TestAutoMemoryLoadsPersonalIndex_WhenSyncClaimedAnotherFolder(t *testing.T) {
	autoMemoryProject(t)
	path := filepath.Join(".claude", "settings.local.json")
	writeClaudeSettings(t, path, `{"autoMemoryDirectory": "/old/store"}`)
	emit.PriorMergedKeys = func(string) []emit.MergedKey {
		return []emit.MergedKey{{Path: []string{autoMemoryDirectoryKey}}}
	}
	t.Cleanup(func() { emit.PriorMergedKeys = nil })

	if got, err := AutoMemoryLoadsPersonalIndex(&config.Config{Builtins: []string{"memory"}}); err != nil || !got {
		t.Errorf("got %v, %v; want sync's claim to count", got, err)
	}
}

// gitWorktree makes a repository with one commit and a linked worktree
// beside it, and returns both checkouts.
func gitWorktree(t *testing.T) (main, worktree string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	main, worktree = filepath.Join(parent, "main"), filepath.Join(parent, "worktree")
	for _, args := range [][]string{
		{"init", "-q", main},
		{"-C", main, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init"},
		{"-C", main, "worktree", "add", "-q", worktree},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Skipf("git %v: %v %s", args, err, out)
		}
	}
	return main, worktree
}

// A worktree Claude Code creates copies the main checkout's settings,
// which name the main checkout's store: auto memory loads that index.
func TestAutoMemoryLoadsPersonalIndex_InAWorktreeWithTheMainCheckoutsStore(t *testing.T) {
	autoMemoryProject(t)
	main, worktree := gitWorktree(t)
	value, err := json.Marshal(map[string]string{"autoMemoryDirectory": filepath.ToSlash(filepath.Join(main, ".agnostic-ai", "local", "memory"))})
	if err != nil {
		t.Fatal(err)
	}
	writeClaudeSettings(t, filepath.Join(worktree, ".claude", "settings.local.json"), string(value))
	testutil.Chdir(t, worktree)

	if got, err := AutoMemoryLoadsPersonalIndex(&config.Config{Builtins: []string{"memory"}}); err != nil || !got {
		t.Errorf("got %v, %v; want the main checkout's store to count", got, err)
	}
}

// In a linked worktree Claude Code reads the main checkout's local
// settings first.
func TestAutoMemoryLoadsPersonalIndex_InAWorktreeFollowsTheMainCheckoutsSettings(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Claude Code reads the local settings of the starting folder on Windows")
	}
	for _, c := range []struct {
		name, settings string
		want           bool
	}{
		{name: "auto memory off", settings: `{"autoMemoryEnabled": false}`},
		{name: "a folder elsewhere", settings: `{"autoMemoryDirectory": "/elsewhere"}`},
		{name: "no auto memory key", settings: `{"permissions": {"allow": []}}`, want: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			autoMemoryProject(t)
			main, worktree := gitWorktree(t)
			writeClaudeSettings(t, filepath.Join(main, ".claude", "settings.local.json"), c.settings)
			testutil.Chdir(t, worktree)

			if got, err := AutoMemoryLoadsPersonalIndex(&config.Config{Builtins: []string{"memory"}}); err != nil || got != c.want {
				t.Errorf("got %v, %v; want %v", got, err, c.want)
			}
		})
	}
}

func TestAutoMemoryLoadsPersonalIndex_FalseWithoutTheMemoryBuiltin(t *testing.T) {
	autoMemoryProject(t)

	if got, err := AutoMemoryLoadsPersonalIndex(&config.Config{}); err != nil || got {
		t.Errorf("got %v, %v; want false", got, err)
	}
}
