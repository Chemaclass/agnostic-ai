package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestScaffold_Demo_SeedsHookScriptWhereSyncReadsIt(t *testing.T) {
	dir := t.TempDir()
	if err := scaffold(scaffoldOptions{Root: dir, Base: ".", Demo: true, Targets: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "scripts", "no-force-push.sh"))
	if err != nil {
		t.Fatalf("a root-level base must still seed the script under .agnostic-ai/scripts/: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		t.Errorf("script mode = %v, want executable since the hook runs it directly", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(dir, "scripts")); !os.IsNotExist(err) {
		t.Errorf("script must not land under the base dir, stat err = %v", err)
	}
}

// syncDemoProject seeds the demo specs for Claude Code and Codex in a
// fresh working directory and syncs them.
func syncDemoProject(t *testing.T) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	silence(t)
	if err := scaffold(scaffoldOptions{Root: dir, Demo: true, Targets: []string{"claude", "codex"}}); err != nil {
		t.Fatal(err)
	}
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	return dir
}

// Codex runs commandWindows through PowerShell, which ignores a shebang,
// so the Windows form must name the shell that runs the script.
func TestDemoHook_CodexWindowsCommandRunsScriptThroughSh(t *testing.T) {
	dir := syncDemoProject(t)
	data, err := os.ReadFile(filepath.Join(dir, ".codex", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if want := `"commandWindows": "sh .codex/hooks/no-force-push.sh"`; !strings.Contains(string(data), want) {
		t.Errorf(".codex/hooks.json misses %s:\n%s", want, data)
	}
}

// On Windows the hooks need sh and bash on PATH, as Git for Windows
// provides; the guard cannot run without them.
func TestDemoHook_BlocksForcePushOnClaudeAndCodex(t *testing.T) {
	for _, shell := range []string{"sh", "bash"} {
		if _, err := exec.LookPath(shell); err != nil {
			t.Skipf("%s is not on PATH", shell)
		}
	}
	syncDemoProject(t)

	cases := []struct {
		command, expect string
	}{
		{"git push --force origin main", "block"},
		{"git push -f origin main", "block"},
		{"git push -uf origin main", "block"},
		{"git fetch && git push origin main --force", "block"},
		{"git -C '/tmp/project (copy)' push --force origin main", "block"},
		{"git -c push.default=current --git-dir=.git push --force", "block"},
		{"git push \\\n  --force origin main", "block"},
		{"/usr/bin/git push --force", "block"},
		{"GIT_TRACE=1 git push --force", "block"},
		{"git status", "allow"},
		{"git push --force-with-lease origin main", "allow"},
		{"git push -o ci.skip --force-with-lease", "allow"},
		{"git push --repo -f origin", "allow"},
		{`git commit -m "drop -f" && git push`, "allow"},
		{"git commit -m 'git push --force is unsafe'", "allow"},
		{`echo "a; git push --force"`, "allow"},
	}
	for _, c := range cases {
		t.Run(c.command, func(t *testing.T) {
			out, err := runHookRun(t, "no-force-push", "--bash", c.command, "--expect", c.expect)
			if err != nil {
				t.Fatalf("err = %v\n%s", err, out)
			}
			for _, target := range []string{"claude: " + c.expect, "codex: " + c.expect} {
				if !strings.Contains(out, target) {
					t.Errorf("output misses %q:\n%s", target, out)
				}
			}
		})
	}
}
