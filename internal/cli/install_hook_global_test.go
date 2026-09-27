package cli

import (
	"bytes"
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

	out, err := runInstallHook("--global")
	if err != nil {
		t.Fatalf("install-hook --global: %v", err)
	}
	if !strings.Contains(out, "appended the checks to") {
		t.Errorf("want the append reported, got %q", out)
	}
	out, err = runInstallHook("--global")
	if err != nil {
		t.Fatalf("second install-hook --global: %v", err)
	}
	if !strings.Contains(out, "already runs the checks") {
		t.Errorf("want the rerun reported as a no-op, got %q", out)
	}

	got := readHook(t, globalHookPath(source))
	if !strings.HasPrefix(got, existing) || !strings.HasSuffix(got, globalHook.text()) {
		t.Errorf("want the checks appended once after the existing content, got:\n%s", got)
	}
	for _, line := range []string{"agnostic-ai lint --global --strict", "agnostic-ai validate --global", "\tagnostic-ai sync --global --check"} {
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
	if err == nil || !strings.Contains(err.Error(), "agnostic-ai install-hook --global") {
		t.Fatalf("want the refusal to point at install-hook --global, got %v", err)
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

	_, err := runInstallHook("--global")
	want := "install-hook --global runs in the global home " + source
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("want %q, got %v", want, err)
	}
	if _, err := os.Stat(globalHookPath(project)); !os.IsNotExist(err) {
		t.Errorf("install-hook --global wrote a hook outside the home: %v", err)
	}
}

func TestInstallHookGlobal_RefusesAHomeOutsideGit(t *testing.T) {
	_, source := globalConfigTestHome(t)
	isolateGit(t)
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, source)

	_, err := runInstallHook("--global")
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

			_, err := runInstallHook("--global")
			want := "core.hooksPath is " + elsewhere + " (set in " + origin + ")"
			if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "agnostic-ai lint --global --strict") {
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

	_, err := runInstallHook("--global")
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

	_, err := runInstallHook("--global")
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
	if _, err := runInstallHook("--global"); err != nil {
		t.Fatal(err)
	}
	git(t, source, "commit", "-q", "--allow-empty", "--no-verify", "-m", "init")
	worktree := filepath.Join(t.TempDir(), "branch")
	git(t, source, "worktree", "add", "-q", "-b", "branch", worktree)

	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "calls.log")
	fake := "#!/bin/sh\n" +
		"echo \"$* home=$AGNOSTIC_AI_HOME\" >> \"$CALLS\"\n" +
		"[ \"$1\" = \"$FAIL\" ] && exit 1\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "agnostic-ai"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}

	all := []string{"lint --global --strict", "validate --global", "sync --global --check"}
	for _, tc := range []struct {
		name    string
		dir     string
		fail    string
		wantErr bool
		want    []string
	}{
		{"all pass", source, "", false, all},
		{"lint fails", source, "lint", true, all[:1]},
		{"validate fails", source, "validate", true, all[:2]},
		{"sync fails", source, "sync", true, all},
		{"linked worktree skips sync", worktree, "sync", false, all[:2]},
		{"linked worktree still lints", worktree, "lint", true, all[:1]},
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
