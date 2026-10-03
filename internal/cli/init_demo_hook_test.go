package cli

import (
	"encoding/json"
	"errors"
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
// fresh Git repository, the working directory, and syncs them.
func syncDemoProject(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	dir := testutil.TempCwd(t)
	gitInit(t, dir)
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

// preToolUseHandler returns a field of the first PreToolUse handler in a
// Claude Code or Codex hooks file.
func preToolUseHandler(t *testing.T, path, field string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Hooks map[string][]struct {
			Hooks []map[string]any `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	groups := doc.Hooks["PreToolUse"]
	if len(groups) == 0 || len(groups[0].Hooks) == 0 {
		t.Fatalf("%s has no PreToolUse handler:\n%s", path, data)
	}
	value, _ := groups[0].Hooks[0][field].(string)
	return value
}

// Codex runs commandWindows through powershell.exe -Command, which ignores
// a shebang and turns a failed native command's exit code into 1. The
// Windows form names sh, finds the script from the Git root, and passes
// sh's exit code on, or 1 when sh never ran.
func TestDemoHook_CodexWindowsCommandKeepsExitCode(t *testing.T) {
	dir := syncDemoProject(t)
	got := preToolUseHandler(t, filepath.Join(dir, ".codex", "hooks.json"), "commandWindows")
	want := `$LASTEXITCODE = 1; sh "$(git rev-parse --show-toplevel)/.codex/hooks/no-force-push.sh"; exit $LASTEXITCODE`
	if got != want {
		t.Errorf("commandWindows = %q\nwant %q", got, want)
	}
}

// Claude Code and Codex start hooks in the session directory, which can
// be below the project root.
func TestDemoHook_BlocksFromNestedDirectory(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := syncDemoProject(t)
	nested := filepath.Join(dir, "src", "app")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"git push --force origin main"}}`
	runs := []struct {
		target, shell, file string
	}{
		{"claude", "bash", filepath.Join(dir, ".claude", "settings.json")},
		{"codex", "sh", filepath.Join(dir, ".codex", "hooks.json")},
	}
	for _, r := range runs {
		t.Run(r.target, func(t *testing.T) {
			cmd := exec.Command(r.shell, "-c", preToolUseHandler(t, r.file, "command"))
			cmd.Dir = nested
			cmd.Env = append(os.Environ(), "CLAUDE_PROJECT_DIR="+dir)
			cmd.Stdin = strings.NewReader(payload)
			out, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 2 {
				t.Errorf("exit = %v, want 2\n%s", err, out)
			}
		})
	}
}

// On Windows the hooks need sh and bash on PATH, as Git for Windows
// provides; the guard cannot run without them. There Codex runs
// commandWindows through PowerShell, so a block shows the exit code
// PowerShell passed on.
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
		{"# Don't overwrite history\ngit push --force origin main", "block"},
		{"git status", "allow"},
		{"git push --force-with-lease origin main", "allow"},
		{"git push --force-with-lease origin main # avoid --force", "allow"},
		{"git push -o ci.skip --force-with-lease", "allow"},
		{"git push --repo -f origin", "allow"},
		{`git commit -m "drop -f" && git push`, "allow"},
		{"git commit -m 'git push --force is unsafe'", "allow"},
		{`echo "a; git push --force"`, "allow"},
		{"echo a#b; git push origin main", "allow"},
	}
	for _, c := range cases {
		t.Run(c.command, func(t *testing.T) {
			out, err := runHookRun(t, "no-force-push", "--bash", c.command, "--expect", c.expect)
			if err != nil {
				t.Fatalf("err = %v\n%s", err, out)
			}
			code := "(exit 0"
			if c.expect == "block" {
				code = "(exit 2"
			}
			for _, target := range []string{"claude: " + c.expect + " " + code, "codex: " + c.expect + " " + code} {
				if !strings.Contains(out, target) {
					t.Errorf("output misses %q:\n%s", target, out)
				}
			}
		})
	}
}
