package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func runInstallHook(args ...string) (string, error) {
	var out bytes.Buffer
	cmd := NewRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"install-hook"}, args...))
	err := cmd.Execute()
	return out.String(), err
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-q")
}

// globalGitHome makes the global home a git repository and enters it.
// It returns the home and the isolated user git config.
func globalGitHome(t *testing.T) (string, string) {
	t.Helper()
	_, source := globalConfigTestHome(t)
	userConfig := isolateGit(t)
	gitInit(t, source)
	testutil.Chdir(t, source)
	return source, userConfig
}

func globalHookPath(source string) string {
	return filepath.Join(source, ".git", "hooks", "pre-commit")
}

func TestInstallHookGlobal_WritesTheGlobalGate(t *testing.T) {
	source, _ := globalGitHome(t)
	existing := "#!/bin/sh\n# the global gate runs agnostic-ai sync --global --check\necho existing\n"
	if err := os.WriteFile(globalHookPath(source), []byte(existing), 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := runInstallHook("--scope", "user")
	if err != nil {
		t.Fatalf("install-hook --scope user: %v", err)
	}
	if !strings.Contains(out, "appended the checks to") {
		t.Errorf("want the append reported, got %q", out)
	}
	out, err = runInstallHook("--scope", "user")
	if err != nil {
		t.Fatalf("second install-hook --scope user: %v", err)
	}
	if !strings.Contains(out, "already runs the checks") {
		t.Errorf("want the rerun reported as a no-op, got %q", out)
	}

	got := readHook(t, globalHookPath(source))
	if !strings.HasPrefix(got, existing) || !strings.HasSuffix(got, globalHook.text()) {
		t.Errorf("want the checks appended once after the existing content, got:\n%s", got)
	}
	for _, line := range []string{`agnostic-ai lint "${agnostic_ai_scope:---global}" --strict`, `agnostic-ai validate "${agnostic_ai_scope:---global}"`, "\t" + `agnostic-ai sync "${agnostic_ai_scope:---global}" --check`} {
		if n := strings.Count(got, line); n != 1 {
			t.Errorf("want %q once, got %d:\n%s", line, n, got)
		}
	}
	if strings.Contains(got, "agnostic-ai sync --check") {
		t.Errorf("global hook runs the project check:\n%s", got)
	}
}

func TestInstallHook_InTheGlobalHomePointsAtGlobal(t *testing.T) {
	source, _ := globalGitHome(t)

	_, err := runInstallHook()
	if err == nil || !strings.Contains(err.Error(), "agnostic-ai install-hook --scope user") {
		t.Fatalf("want the refusal to point at install-hook --scope user, got %v", err)
	}
	if _, err := os.Stat(globalHookPath(source)); !os.IsNotExist(err) {
		t.Errorf("plain install-hook wrote a hook into the global home: %v", err)
	}
}

func TestInstallHookGlobal_RefusesOutsideTheHome(t *testing.T) {
	source, _ := globalGitHome(t)
	project := t.TempDir()
	gitInit(t, project)
	testutil.Chdir(t, project)

	_, err := runInstallHook("--scope", "user")
	want := "install-hook --scope user runs in the global home " + source
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("want %q, got %v", want, err)
	}
	if _, err := os.Stat(globalHookPath(project)); !os.IsNotExist(err) {
		t.Errorf("install-hook --scope user wrote a hook outside the home: %v", err)
	}
}

func TestInstallHookGlobal_RefusesAHomeOutsideGit(t *testing.T) {
	_, source := globalConfigTestHome(t)
	isolateGit(t)
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, source)

	_, err := runInstallHook("--scope", "user")
	if err == nil || !strings.Contains(err.Error(), "is not the root of a git repository") {
		t.Fatalf("want the git root refusal, got %v", err)
	}
}

func TestInstallHookGlobal_RejectsShared(t *testing.T) {
	globalGitHome(t)

	_, err := runInstallHook("--global", "--shared")
	if err == nil || !strings.Contains(err.Error(), "none of the others can be") {
		t.Fatalf("want --global and --shared rejected together, got %v", err)
	}
}

func TestInstallHookGlobal_RefusesWhenCoreHooksPathSendsGitElsewhere(t *testing.T) {
	for _, tc := range []struct {
		name  string
		scope string
	}{
		{"user level", "--global"},
		{"repository", "--local"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source, userConfig := globalGitHome(t)
			elsewhere := filepath.Join(t.TempDir(), "hooks")
			git(t, source, "config", tc.scope, "core.hooksPath", elsewhere)
			origin := ".git/config"
			if tc.scope == "--global" {
				origin = userConfig
			}

			_, err := runInstallHook("--scope", "user")
			want := "core.hooksPath is " + elsewhere + " (set in " + origin + ")"
			if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), `agnostic-ai lint "${agnostic_ai_scope:---global}" --strict`) {
				t.Fatalf("want %q with the lines to add, got %v", want, err)
			}
			for _, path := range []string{globalHookPath(source), filepath.Join(elsewhere, "pre-commit")} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("refused install wrote %s: %v", path, err)
				}
			}
		})
	}
}

func TestInstallHookGlobal_RefusesAStaleProjectCheck(t *testing.T) {
	source, _ := globalGitHome(t)
	existing := "#!/bin/sh\nagnostic-ai sync --check\n"
	if err := os.WriteFile(globalHookPath(source), []byte(existing), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := runInstallHook("--scope", "user")
	if err == nil || !strings.Contains(err.Error(), "runs `agnostic-ai sync --check`") || !strings.Contains(err.Error(), "remove that line") {
		t.Fatalf("want the stale project line refused, got %v", err)
	}
	if got := readHook(t, globalHookPath(source)); got != existing {
		t.Errorf("refused hook changed:\n%s", got)
	}
}

func TestInstallHookGlobal_RefusesAHookThatExitsFirst(t *testing.T) {
	source, _ := globalGitHome(t)
	existing := "#!/bin/sh\necho hi\nexit 0\n"
	if err := os.WriteFile(globalHookPath(source), []byte(existing), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := runInstallHook("--scope", "user")
	if err == nil || !strings.Contains(err.Error(), `ends at its "exit 0" line`) {
		t.Fatalf("want the exit refused, got %v", err)
	}
	if got := readHook(t, globalHookPath(source)); got != existing {
		t.Errorf("refused hook changed:\n%s", got)
	}
}

// TestInstallHookGlobal_CommitsRunTheChecks commits through git with a
// fake agnostic-ai on PATH that logs each call and fails the one FAIL
// names.
func TestInstallHookGlobal_CommitsRunTheChecks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake agnostic-ai is a sh script")
	}
	source, _ := globalGitHome(t)
	if _, err := runInstallHook("--scope", "user"); err != nil {
		t.Fatal(err)
	}
	git(t, source, "commit", "-q", "--allow-empty", "--no-verify", "-m", "init")
	worktree := filepath.Join(t.TempDir(), "branch")
	git(t, source, "worktree", "add", "-q", "-b", "branch", worktree)

	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "calls.log")
	// OLD makes the fake an agnostic-ai from before --scope: its help
	// probe fails, as an unknown flag does.
	fake := "#!/bin/sh\n" +
		"case \"$*\" in *--help) [ -n \"$OLD\" ] && exit 1; exit 0 ;; esac\n" +
		"echo \"$* home=$AGNOSTIC_AI_HOME\" >> \"$CALLS\"\n" +
		"[ \"$1\" = \"$FAIL\" ] && exit 1\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "agnostic-ai"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}

	all := []string{"lint --scope=user --strict", "validate --scope=user", "sync --scope=user --check"}
	older := []string{"lint --global --strict", "validate --global", "sync --global --check"}
	for _, tc := range []struct {
		name    string
		dir     string
		fail    string
		old     string
		wantErr bool
		want    []string
	}{
		{"all pass", source, "", "", false, all},
		{"lint fails", source, "lint", "", true, all[:1]},
		{"validate fails", source, "validate", "", true, all[:2]},
		{"sync fails", source, "sync", "", true, all},
		{"linked worktree skips sync", worktree, "sync", "", false, all[:2]},
		{"linked worktree still lints", worktree, "lint", "", true, all[:1]},
		// A hook this release wrote must not block commits when an
		// older agnostic-ai is first on PATH.
		{"older agnostic-ai gets --global", source, "", "1", false, older},
		{"older agnostic-ai still fails a check", source, "validate", "1", true, older[:2]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = os.Remove(log)
			commit := exec.Command("git", "commit", "-q", "--allow-empty", "-m", tc.name)
			commit.Dir = tc.dir
			commit.Env = append(os.Environ(),
				"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
				"AGNOSTIC_AI_HOME="+t.TempDir(),
				"CALLS="+log,
				"FAIL="+tc.fail,
				"OLD="+tc.old,
			)
			out, err := commit.CombinedOutput()
			if (err != nil) != tc.wantErr {
				t.Errorf("want commit error %v, got %v\n%s", tc.wantErr, err, out)
			}
			home, err := filepath.EvalSymlinks(tc.dir)
			if err != nil {
				t.Fatal(err)
			}
			var want []string
			for _, c := range tc.want {
				want = append(want, c+" home="+home)
			}
			calls, _ := os.ReadFile(log)
			if got := strings.TrimSpace(string(calls)); got != strings.Join(want, "\n") {
				t.Errorf("calls\nwant %s\ngot  %s", strings.Join(want, "\n"), got)
			}
		})
	}
}

func TestParseConfigOrigin_UnquotesWindowsPaths(t *testing.T) {
	value, origin := parseConfigOrigin("file:\"C:\\\\Users\\\\me\\\\.gitconfig\"\tC:\\hooks\n")

	if value != `C:\hooks` || origin != `C:\Users\me\.gitconfig` {
		t.Errorf("got value %q origin %q", value, origin)
	}
}

func TestParseConfigOrigin_KeepsPlainPaths(t *testing.T) {
	value, origin := parseConfigOrigin("file:.git/config\t.husky\n")

	if value != ".husky" || origin != ".git/config" {
		t.Errorf("got value %q origin %q", value, origin)
	}
}

// A hook an older release wrote runs the checks with --global, which now
// warns on every commit. Installing again moves each line to --scope user
// and keeps what the user added around them.
func TestInstallHookGlobal_UpgradesTheDeprecatedFlagInAnInstalledHook(t *testing.T) {
	source, _ := globalGitHome(t)
	old := "#!/bin/sh\necho mine\n" + globalHook.sentinel + "\n" + globalHookChecksBeforeScope + "\n"
	if !strings.Contains(old, "agnostic-ai lint --global --strict || exit 1") {
		t.Fatalf("fixture does not hold the old lines:\n%s", old)
	}
	path := globalHookPath(source)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(old), 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := runInstallHook("--scope", "user")
	if err != nil {
		t.Fatalf("install-hook --scope user: %v\n%s", err, out)
	}
	if !strings.Contains(out, "updated the checks") {
		t.Errorf("want the update reported, got %q", out)
	}
	got := readHook(t, path)
	if want := "#!/bin/sh\necho mine\n" + globalHook.text(); got != want {
		t.Errorf("hook after upgrade:\n%s\nwant:\n%s", got, want)
	}
}

func TestInstallHookGlobal_ScopeUserRejectsProjectHookFlags(t *testing.T) {
	globalGitHome(t)
	for _, flag := range []string{"--shared", "--post-checkout"} {
		out, err := runInstallHook("--scope", "user", flag)
		if err == nil || !strings.Contains(err.Error(), "--scope user cannot be combined") {
			t.Errorf("install-hook --scope user %s: err=%v out=%q, want a flag conflict", flag, err, out)
		}
	}
}

// globalHookChecksBeforeScope is the body releases up to v0.96.0 wrote.
const globalHookChecksBeforeScope = `AGNOSTIC_AI_HOME="$(git rev-parse --show-toplevel)" || exit 1
export AGNOSTIC_AI_HOME
agnostic-ai lint --global --strict || exit 1
agnostic-ai validate --global || exit 1
# A linked worktree holds a branch, not the specs sync --global deployed,
# so only the main checkout compares them with the live files.
git_dir="$(cd "$(git rev-parse --git-dir)" && pwd -P)" || exit 1
common_dir="$(cd "$(git rev-parse --git-common-dir)" && pwd -P)" || exit 1
if [ "$git_dir" = "$common_dir" ]; then
	agnostic-ai sync --global --check || exit 1
fi`

// commitWithFakeAgnosticAI commits in dir with a fake agnostic-ai first
// on PATH and returns the calls it logged, apart from help probes.
func commitWithFakeAgnosticAI(t *testing.T, dir string) (string, error) {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "calls.log")
	fake := "#!/bin/sh\ncase \"$*\" in *--help) exit 0 ;; esac\necho \"$*\" >> \"$CALLS\"\n"
	if err := os.WriteFile(filepath.Join(bin, "agnostic-ai"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	commit := exec.Command("git", "commit", "-q", "--allow-empty", "-m", "x")
	commit.Dir = dir
	commit.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "CALLS="+log)
	out, err := commit.CombinedOutput()
	calls, _ := os.ReadFile(log)
	if err != nil {
		return string(calls), fmt.Errorf("%w: %s", err, out)
	}
	return strings.TrimSpace(string(calls)), nil
}

// A hook edited by hand may lack the line the scope probe replaces.
// The other checks must still run at user scope, not fall to the
// project.
func TestInstallHookGlobal_UpgradeOfAHookWithoutTheLintLineStaysAtUserScope(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake agnostic-ai is a sh script")
	}
	source, _ := globalGitHome(t)
	body := strings.Replace(globalHookChecksBeforeScope, "agnostic-ai lint --global --strict || exit 1\n", "", 1)
	path := globalHookPath(source)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+globalHook.sentinel+"\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := runInstallHook("--scope", "user"); err != nil {
		t.Fatalf("install-hook: %v\n%s", err, out)
	}

	calls, err := commitWithFakeAgnosticAI(t, source)
	if err != nil {
		t.Fatal(err)
	}
	if want := "validate --global\nsync --global --check"; calls != want {
		t.Errorf("calls\nwant %s\ngot  %s", want, calls)
	}
}

// The flag must reach agnostic-ai as one argument in a hook the user
// runs under zsh or after changing IFS, where a two-word value would
// not split.
func TestInstallHookGlobal_ScopeSurvivesZshAndAChangedIFS(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake agnostic-ai is a sh script")
	}
	for _, tc := range []struct{ name, shell, prelude string }{
		{"changed IFS", "/bin/sh", "IFS='\n'\n"},
		{"zsh", "/bin/zsh", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := os.Stat(tc.shell); err != nil {
				t.Skipf("%s is not installed", tc.shell)
			}
			source, _ := globalGitHome(t)
			path := globalHookPath(source)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("#!"+tc.shell+"\n"+tc.prelude+globalHook.text()), 0o755); err != nil {
				t.Fatal(err)
			}
			calls, err := commitWithFakeAgnosticAI(t, source)
			if err != nil {
				t.Fatal(err)
			}
			if want := "lint --scope=user --strict\nvalidate --scope=user\nsync --scope=user --check"; calls != want {
				t.Errorf("calls\nwant %s\ngot  %s", want, calls)
			}
		})
	}
}

func TestInstallHookGlobal_UpgradeKeepsTheIndentOfEveryInsertedLine(t *testing.T) {
	source, _ := globalGitHome(t)
	path := globalHookPath(source)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	old := "#!/bin/sh\nif true; then\n    " + globalHook.sentinel + "\n    agnostic-ai lint --global --strict || exit 1\nfi\n"
	if err := os.WriteFile(path, []byte(old), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := runInstallHook("--scope", "user"); err != nil {
		t.Fatalf("install-hook: %v\n%s", err, out)
	}
	for _, line := range strings.Split(readHook(t, path), "\n")[2:6] {
		if !strings.HasPrefix(line, "    ") {
			t.Errorf("line lost its indent: %q", line)
		}
	}
}
