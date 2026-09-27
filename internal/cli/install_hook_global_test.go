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
	if out, err := exec.Command("git", "init", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
}

func globalHookPath(source string) string {
	return filepath.Join(source, ".git", "hooks", "pre-commit")
}

func TestInstallHookGlobal_WritesTheGlobalGate(t *testing.T) {
	_, source := globalConfigTestHome(t)
	gitInit(t, source)
	testutil.Chdir(t, source)
	existing := "#!/bin/sh\necho existing\n"
	if err := os.WriteFile(globalHookPath(source), []byte(existing), 0o755); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if _, err := runInstallHook("--global"); err != nil {
			t.Fatalf("install-hook --global: %v", err)
		}
	}

	data, err := os.ReadFile(globalHookPath(source))
	if err != nil {
		t.Fatalf("hook not written: %v", err)
	}
	got := string(data)
	if !strings.HasPrefix(got, existing) {
		t.Errorf("existing hook content lost, got:\n%s", got)
	}
	for _, line := range []string{"agnostic-ai lint --global --strict", "agnostic-ai validate --global", "agnostic-ai sync --global --check"} {
		if n := strings.Count(got, line); n != 1 {
			t.Errorf("want %q once, got %d:\n%s", line, n, got)
		}
	}
	if strings.Contains(got, "agnostic-ai sync --check") {
		t.Errorf("global hook runs the project check:\n%s", got)
	}
}

func TestInstallHook_InTheGlobalHomePointsAtGlobal(t *testing.T) {
	_, source := globalConfigTestHome(t)
	gitInit(t, source)
	testutil.Chdir(t, source)

	_, err := runInstallHook()
	if err == nil || !strings.Contains(err.Error(), "agnostic-ai install-hook --global") {
		t.Fatalf("want the refusal to point at install-hook --global, got %v", err)
	}
	if _, err := os.Stat(globalHookPath(source)); !os.IsNotExist(err) {
		t.Errorf("plain install-hook wrote a hook into the global home: %v", err)
	}
}

func TestInstallHookGlobal_RefusesOutsideTheHome(t *testing.T) {
	_, source := globalConfigTestHome(t)
	gitInit(t, source)
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
	_, source := globalConfigTestHome(t)
	gitInit(t, source)
	testutil.Chdir(t, source)

	_, err := runInstallHook("--global", "--shared")
	if err == nil || !strings.Contains(err.Error(), "none of the others can be") {
		t.Fatalf("want --global and --shared rejected together, got %v", err)
	}
}

func TestInstallHookGlobal_ScriptChecksTheCommittedHomeAndStopsOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("runs the hook with sh")
	}
	_, source := globalConfigTestHome(t)
	gitInit(t, source)
	testutil.Chdir(t, source)
	if _, err := runInstallHook("--global"); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}

	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "calls.log")
	fake := "#!/bin/sh\n" +
		"echo \"$* home=$AGNOSTIC_AI_HOME\" >> \"$CALLS\"\n" +
		"[ \"$1\" = \"$FAIL\" ] && exit 1\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "agnostic-ai"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		fail    string
		wantErr bool
		want    []string
	}{
		{"", false, []string{"lint --global --strict", "validate --global", "sync --global --check"}},
		{"lint", true, []string{"lint --global --strict"}},
		{"validate", true, []string{"lint --global --strict", "validate --global"}},
		{"sync", true, []string{"lint --global --strict", "validate --global", "sync --global --check"}},
	} {
		_ = os.Remove(log)
		hook := exec.Command("sh", globalHookPath(source))
		hook.Dir = source
		hook.Env = append(os.Environ(),
			"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
			"AGNOSTIC_AI_HOME="+t.TempDir(),
			"CALLS="+log,
			"FAIL="+tc.fail,
		)
		out, err := hook.CombinedOutput()
		if (err != nil) != tc.wantErr {
			t.Errorf("fail=%q: want error %v, got %v\n%s", tc.fail, tc.wantErr, err, out)
		}
		calls, _ := os.ReadFile(log)
		var want []string
		for _, c := range tc.want {
			want = append(want, c+" home="+resolved)
		}
		if got := strings.TrimSpace(string(calls)); got != strings.Join(want, "\n") {
			t.Errorf("fail=%q: calls\nwant %s\ngot  %s", tc.fail, strings.Join(want, "\n"), got)
		}
	}
}
