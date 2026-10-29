package claude

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/gitquery"
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
		{name: "repo settings blocked", settings: map[string]string{local: `{"permissions":{"blockReadsOutsideWorkingDirectories":true}}`}},
		{name: "project settings blocked", settings: map[string]string{project: `{"permissions":{"blockReadsOutsideWorkingDirectories":true}}`}},
		{name: "off in settings environment", settings: map[string]string{local: `{"env":{"CLAUDE_CODE_DISABLE_AUTO_MEMORY":"1"}}`}},
		{name: "settings environment overrides shell", env: "0", settings: map[string]string{local: `{"env":{"CLAUDE_CODE_DISABLE_AUTO_MEMORY":"1"}}`}},
		{name: "local false cannot lift project block", settings: map[string]string{local: `{"permissions":{"blockReadsOutsideWorkingDirectories":false}}`, project: `{"permissions":{"blockReadsOutsideWorkingDirectories":true}}`}},
		{name: "local environment wins over user", settings: map[string]string{local: `{"env":{"CLAUDE_CODE_DISABLE_AUTO_MEMORY":"1"}}`, "user": `{"env":{"CLAUDE_CODE_DISABLE_AUTO_MEMORY":"0"}}`}},
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

func TestAutoMemoryLoadsPersonalIndex_UnmanagedMissingSettingsKeepTheImport(t *testing.T) {
	autoMemoryProject(t)
	cfg := &config.Config{Builtins: []string{"memory"}, Sync: config.SyncConfig{Unmanaged: []string{".claude/settings.local.json"}}}
	if got, err := AutoMemoryLoadsPersonalIndex(cfg); err != nil || got {
		t.Errorf("got %v, %v; want missing unmanaged settings to keep the import", got, err)
	}
}

func TestAutoMemoryLoadsPersonalIndex_PermissionsBlockOnlyRepositorySuppliedDirectories(t *testing.T) {
	for _, c := range []struct {
		name                                 string
		git, tracked, ignored, missing, want bool
	}{
		{name: "outside git"},
		{name: "untracked local settings", git: true, want: true},
		{name: "ignored local settings", git: true, ignored: true, want: true},
		{name: "tracked local settings", git: true, tracked: true},
		{name: "sync will create untracked local settings", git: true, missing: true, want: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			user := autoMemoryProject(t)
			writeClaudeSettings(t, filepath.Join(user, "settings.json"), `{"permissions":{"blockReadsOutsideWorkingDirectories":true}}`)
			if c.git {
				if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
					t.Fatalf("git init: %v %s", err, out)
				}
			}
			path := filepath.Join(".claude", "settings.local.json")
			if !c.missing {
				wd, err := os.Getwd()
				if err != nil {
					t.Fatal(err)
				}
				value, err := json.Marshal(map[string]string{"autoMemoryDirectory": filepath.ToSlash(filepath.Join(wd, ".agnostic-ai", "local", "memory"))})
				if err != nil {
					t.Fatal(err)
				}
				writeClaudeSettings(t, path, string(value))
			}
			if c.ignored {
				writeClaudeSettings(t, ".gitignore", ".claude/settings.local.json\n")
			}
			if c.tracked {
				if out, err := exec.Command("git", "add", "-f", path).CombinedOutput(); err != nil {
					t.Fatalf("git add: %v %s", err, out)
				}
			}
			got, err := AutoMemoryLoadsPersonalIndex(&config.Config{Builtins: []string{"memory"}})
			if err != nil || got != c.want {
				t.Errorf("got %v, %v; want %v", got, err, c.want)
			}
		})
	}
}

// Git names the work tree by its resolved path, while the root can be
// another spelling of it: a link such as macOS /var, or a Windows short
// name such as RUNNER~1.
func TestAutoMemoryLoadsPersonalIndex_PermissionsFollowGitUnderAnotherRootSpelling(t *testing.T) {
	for _, c := range []struct {
		name    string
		tracked bool
	}{
		{name: "untracked local settings"},
		{name: "tracked local settings", tracked: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			user := autoMemoryProject(t)
			real, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(t.TempDir(), "link")
			if err := os.Symlink(real, root); err != nil {
				t.Skipf("symlink: %v", err)
			}
			writeClaudeSettings(t, filepath.Join(user, "settings.json"), `{"permissions":{"blockReadsOutsideWorkingDirectories":true}}`)
			if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
				t.Fatalf("git init: %v %s", err, out)
			}
			if c.tracked {
				path := filepath.Join(".claude", "settings.local.json")
				value, err := json.Marshal(map[string]string{"autoMemoryDirectory": filepath.ToSlash(filepath.Join(real, ".agnostic-ai", "local", "memory"))})
				if err != nil {
					t.Fatal(err)
				}
				writeClaudeSettings(t, path, string(value))
				if out, err := exec.Command("git", "add", "-f", path).CombinedOutput(); err != nil {
					t.Fatalf("git add: %v %s", err, out)
				}
			}
			dir, err := New().NativePersonalMemoryDir(&config.Config{Builtins: []string{"memory"}}, root)
			if err != nil || (dir != "") == c.tracked {
				t.Errorf("got %q, %v; want the personal store only for untracked settings", dir, err)
			}
		})
	}
}

func TestAutoMemoryLoadsPersonalIndex_PermissionsBlockLocalSettingsTrackedUnderAnotherSpelling(t *testing.T) {
	for _, c := range []struct {
		name       string
		tracked    string
		ignoreCase string
		alias      func(string) string
	}{
		{name: "look-alike letter", tracked: ".claude/\U0000017fettings.local.json", ignoreCase: "true", alias: func(name string) string { return strings.Replace(name, "s", "\U0000017f", 1) }},
		{name: "file name", tracked: ".claude/Settings.Local.json", ignoreCase: "true"},
		{name: "folder name", tracked: ".CLAUDE/settings.local.json", ignoreCase: "true"},
		{name: "Git setting off", tracked: ".claude/Settings.Local.json", ignoreCase: "false"},
	} {
		t.Run(c.name, func(t *testing.T) {
			user := autoMemoryProject(t)
			probe := filepath.Join(t.TempDir(), "settings")
			if err := os.WriteFile(probe, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			spelled := strings.ToUpper(probe)
			if c.alias != nil {
				spelled = filepath.Join(filepath.Dir(probe), c.alias("settings"))
			}
			if _, err := os.Stat(spelled); err != nil {
				t.Skip("the disk does not open this spelling as the name Claude Code reads, so it never reads it as settings.local.json")
			}
			wd, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			writeClaudeSettings(t, filepath.Join(user, "settings.json"), `{"permissions":{"blockReadsOutsideWorkingDirectories":true}}`)
			for _, args := range [][]string{{"init", "-q"}, {"config", "core.ignorecase", c.ignoreCase}} {
				if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
					t.Fatalf("git %v: %v %s", args, err, out)
				}
			}
			if err := os.MkdirAll(".claude", 0o755); err != nil {
				t.Fatal(err)
			}
			value, err := json.Marshal(map[string]string{"autoMemoryDirectory": filepath.ToSlash(filepath.Join(wd, ".agnostic-ai", "local", "memory"))})
			if err != nil {
				t.Fatal(err)
			}
			writeClaudeSettings(t, filepath.FromSlash(c.tracked), string(value))
			if out, err := exec.Command("git", "add", "-f", c.tracked).CombinedOutput(); err != nil {
				t.Fatalf("git add: %v %s", err, out)
			}
			if out, err := exec.Command("git", "ls-files", "-z").Output(); err != nil || !strings.Contains(string(out), c.tracked) {
				t.Fatalf("the index holds %q, want %s (%v)", out, c.tracked, err)
			}
			dir, err := New().NativePersonalMemoryDir(&config.Config{Builtins: []string{"memory"}}, wd)
			if err != nil || dir != "" {
				t.Errorf("got %q, %v; want the repository's directory blocked", dir, err)
			}
		})
	}
}

func TestAutoMemoryLoadsPersonalIndex_PermissionsBlockLocalSettingsUnderTrackedLookAlikeFolder(t *testing.T) {
	user := autoMemoryProject(t)
	probe := filepath.Join(t.TempDir(), "sub")
	if err := os.Mkdir(probe, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(probe), "\U0000017fub")); err != nil {
		t.Skip("the disk does not open \U0000017fub as sub, so a tracked \U0000017fub/ never supplies the settings of sub/")
	}
	top, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(top, "sub")
	writeClaudeSettings(t, filepath.Join(user, "settings.json"), `{"permissions":{"blockReadsOutsideWorkingDirectories":true}}`)
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	value, err := json.Marshal(map[string]string{"autoMemoryDirectory": filepath.ToSlash(filepath.Join(project, ".agnostic-ai", "local", "memory"))})
	if err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(project, ".claude", "settings.local.json")
	writeClaudeSettings(t, local, string(value))
	cfg := &config.Config{Builtins: []string{"memory"}}
	if dir, err := New().NativePersonalMemoryDir(cfg, project); err != nil || dir == "" {
		t.Fatalf("untracked local settings: got %q, %v; want the repository's directory trusted", dir, err)
	}
	blob, err := exec.Command("git", "hash-object", "-w", local).Output()
	if err != nil {
		t.Fatalf("git hash-object: %v", err)
	}
	gitquery.Reset()
	entry := "100644," + strings.TrimSpace(string(blob)) + ",\U0000017fub/.claude/settings.local.json"
	if out, err := exec.Command("git", "update-index", "--add", "--cacheinfo", entry).CombinedOutput(); err != nil {
		t.Fatalf("git update-index: %v %s", err, out)
	}
	gitquery.Reset()
	if dir, err := New().NativePersonalMemoryDir(cfg, project); err != nil || dir != "" {
		t.Errorf("got %q, %v; want the repository's directory blocked", dir, err)
	}
}

func TestAutoMemoryLoadsPersonalIndex_WorktreeBlockFollowsTheDirectorySource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Claude Code reads the local settings of the starting folder on Windows")
	}
	for _, tracked := range []bool{false, true} {
		t.Run(map[bool]string{false: "untracked main settings", true: "tracked main settings"}[tracked], func(t *testing.T) {
			user := autoMemoryProject(t)
			writeClaudeSettings(t, filepath.Join(user, "settings.json"), `{"permissions":{"blockReadsOutsideWorkingDirectories":true}}`)
			main, worktree := gitWorktree(t)
			value, err := json.Marshal(map[string]string{"autoMemoryDirectory": filepath.ToSlash(filepath.Join(main, ".agnostic-ai", "local", "memory"))})
			if err != nil {
				t.Fatal(err)
			}
			writeClaudeSettings(t, filepath.Join(main, ".claude", "settings.local.json"), string(value))
			if tracked {
				if out, err := exec.Command("git", "-C", main, "add", "-f", ".claude/settings.local.json").CombinedOutput(); err != nil {
					t.Fatalf("git add: %v %s", err, out)
				}
			}
			testutil.Chdir(t, worktree)
			got, err := AutoMemoryLoadsPersonalIndex(&config.Config{Builtins: []string{"memory"}})
			if err != nil || got == tracked {
				t.Errorf("got %v, %v; want %v", got, err, !tracked)
			}
		})
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

func TestAutoMemoryLoadsPersonalIndex_HomeRelativePersonalStoreUsesNativeMemory(t *testing.T) {
	autoMemoryProject(t)
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	writeClaudeSettings(t, filepath.Join(".claude", "settings.local.json"), `{"autoMemoryDirectory":"~/.agnostic-ai/local/memory"}`)
	if got, err := AutoMemoryLoadsPersonalIndex(&config.Config{Builtins: []string{"memory"}}); err != nil || !got {
		t.Errorf("got %v, %v; want home-relative personal store to use native memory", got, err)
	}
}
