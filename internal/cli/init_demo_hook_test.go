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
	return syncDemoProjectBelowGitRoot(t, "")
}

// syncDemoProjectBelowGitRoot seeds and syncs the demo project in rel
// below a fresh Git repository's root, the working directory.
func syncDemoProjectBelowGitRoot(t *testing.T, rel string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	repo := t.TempDir()
	gitInit(t, repo)
	dir := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
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

// demoWindowsCommand is the commandWindows sync writes for the demo hook
// in a project at rel below the Git root.
func demoWindowsCommand(rel string) string {
	return `$s = "$(git rev-parse --show-toplevel)/` + rel + `.codex/hooks/no-force-push.sh"; ` +
		`if (-not (Get-Command sh -ErrorAction SilentlyContinue)) { exit 1 }; sh $s; exit $LASTEXITCODE`
}

// Codex runs commandWindows through powershell.exe -Command, which ignores
// a shebang and turns a failed native command's exit code into 1. The
// Windows form finds the script from the Git root, names sh, and passes
// sh's exit code on, or 1 when sh is missing.
func TestDemoHook_CodexWindowsCommandKeepsExitCode(t *testing.T) {
	dir := syncDemoProject(t)
	got := preToolUseHandler(t, filepath.Join(dir, ".codex", "hooks.json"), "commandWindows")
	if want := demoWindowsCommand(""); got != want {
		t.Errorf("commandWindows = %q\nwant %q", got, want)
	}
}

// git in the root lookup resets $LASTEXITCODE, so a missing sh must fail
// the hook on its own check rather than through a stale exit code.
func TestDemoHook_CodexWindowsCommandFailsWithoutSh(t *testing.T) {
	var powershell string
	for _, name := range []string{"powershell.exe", "pwsh"} {
		if path, err := exec.LookPath(name); err == nil {
			powershell = path
			break
		}
	}
	if powershell == "" {
		t.Skip("PowerShell is not on PATH")
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not on PATH")
	}
	dir := syncDemoProject(t)
	bin := filepath.Dir(gitPath)
	if _, err := exec.LookPath(filepath.Join(bin, "sh")); err == nil {
		bin = t.TempDir()
		if err := os.Symlink(gitPath, filepath.Join(bin, filepath.Base(gitPath))); err != nil {
			t.Skipf("no PATH with git and without sh: %v", err)
		}
	}

	cmd := exec.Command(powershell, "-NoProfile", "-Command", preToolUseHandler(t, filepath.Join(dir, ".codex", "hooks.json"), "commandWindows"))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+bin)
	cmd.Stdin = strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"git push --force"}}`)
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() == 2 {
		t.Errorf("exit = %v, want a hook error other than 0 or 2\n%s", err, out)
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

// A project below the Git root keeps its path in every emitted command,
// the Windows one included.
func TestDemoHook_BlocksInProjectBelowGitRoot(t *testing.T) {
	dir := syncDemoProjectBelowGitRoot(t, "packages/app")
	codexFile := filepath.Join(dir, ".codex", "hooks.json")
	windows := preToolUseHandler(t, codexFile, "commandWindows")
	if want := demoWindowsCommand("packages/app/"); windows != want {
		t.Errorf("commandWindows = %q\nwant %q", windows, want)
	}

	descendant := filepath.Join(dir, "src")
	if err := os.MkdirAll(descendant, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"git push --force origin main"}}`
	var runs []struct {
		name string
		argv []string
	}
	add := func(name string, argv ...string) {
		runs = append(runs, struct {
			name string
			argv []string
		}{name, argv})
	}
	if runtime.GOOS != "windows" {
		add("claude", "bash", "-c", preToolUseHandler(t, filepath.Join(dir, ".claude", "settings.json"), "command"))
		add("codex", "sh", "-c", preToolUseHandler(t, codexFile, "command"))
	}
	if _, err := exec.LookPath("sh"); err == nil {
		for _, powershell := range []string{"powershell.exe", "pwsh"} {
			if _, err := exec.LookPath(powershell); err == nil {
				add("codex-windows", powershell, "-NoProfile", "-Command", windows)
				break
			}
		}
	}
	for _, r := range runs {
		t.Run(r.name, func(t *testing.T) {
			cmd := exec.Command(r.argv[0], r.argv[1:]...)
			cmd.Dir = descendant
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
		{"git push --repo -f origin", "block"},
		{`git commit -m "drop -f" && git push`, "allow"},
		{"git commit -m 'git push --force is unsafe'", "allow"},
		{`echo "a; git push --force"`, "allow"},
		{"echo a#b; git push origin main", "allow"},
		{"cat <<'EOF'\ngit push --force origin main\nEOF", "allow"},
		{"git commit -F - <<'EOF'\ndocs: warn about force pushes\n\ngit push --force origin main\nEOF", "allow"},
		{"cat <<\"EOF\" > notes\ngit push -f\nEOF\ngit status", "allow"},
		{"cat <<-EOF\n\tgit push --force\n\tEOF\ngit status", "allow"},
		{"cat <<EOF; echo done\ngit push --force\nEOF", "allow"},
		{"cat <<\ngit push --force", "allow"},
		{"cat <<EOF\ngit push --force\nEOF\ngit push --force origin main", "block"},
		{"git status <<<'x'; git push --force", "block"},
		{"if git status --porcelain; then git push --force origin main; fi", "block"},
		{"while true; do git push -f origin main; done", "block"},
		{"if true; then git push --force-with-lease origin main; fi", "allow"},
		{"! time -p git push --force", "block"},
		{"{ git push --force; }", "block"},
		{"env FOO=1 git push --force", "block"},
		{"env -u HOME -i FOO=1 git push --force", "block"},
		{"env FOO=1 git push --force-with-lease", "allow"},
		{"GIT_TRACE=1 git push --force-with-lease", "allow"},
		{"command git push --force", "block"},
		{"exec -a pusher git push --force", "block"},
		{"nohup nice -n 5 git push --force", "block"},
		{"echo then git push --force", "block"},
		{`"if" git push --force`, "block"},
		{"echo 'then git push --force'", "allow"},
		{"git push 2>&1 --force origin main", "block"},
		{"2>/dev/null git push --force", "block"},
		{"git 2>&1 push --force", "block"},
		{"git push &>log --force", "block"},
		{"git push &>> log --force", "block"},
		{"git push --force >| out 2> err < /dev/null", "block"},
		{"git push 2>&1 --force-with-lease origin main", "allow"},
		{"git 2>&1 push --force-with-lease", "allow"},
		{"git push &>log --force-with-lease", "allow"},
		{"git push <<< x --force-with-lease", "allow"},
		{"cd repo && git push --force", "block"},
		{"git push --force | tee log", "block"},
		{"(git push --force)", "block"},
		{"echo $(git push --force)", "block"},
		{`out="$(git push --force 2>&1)"`, "block"},
		{"echo `git push --force`", "block"},
		{"echo \"`git push -f`\"", "block"},
		{`echo "$(git status) done"; git push --force-with-lease`, "allow"},
		{"git -c core.x=y push -f", "block"},
		{"sudo git push --force", "block"},
		{"sudo -u deploy git push --force", "block"},
		{"git push --force=true origin main", "allow"},
		{"git push --force origin HEAD:main", "block"},
		{"git push origin +main", "block"},
		{"git push origin 'feature:main' +HEAD:release", "block"},
		{"git push --force-with-lease origin +main", "allow"},
		{"git push origin main:main", "allow"},
		{"git push --no-force origin main", "allow"},
		{"git stash push -f", "allow"},
		{"sudo --user deploy git push --force", "block"},
		{"sudo --user deploy git push --force-with-lease", "allow"},
		{"env --chdir /tmp git push -f", "block"},
		{"git push origin HEAD:main 2> >(tee push.log) --force", "block"},
		{"git push origin HEAD:main 2> >(tee push.log) --force-with-lease", "allow"},
		{"diff <(git log -1) <(git log -1 origin/main)", "allow"},
		{"echo main | xargs git push --force origin", "block"},
		{"echo main | xargs git push --force-if-includes --force-with-lease origin", "allow"},
		{"git stash push -f; git push --no-force origin main", "allow"},
		{"echo main | xargs git push --force --force-if-includes origin", "block"},
		{"echo main | xargs -n 1 -P 2 git push -f origin", "block"},
		{"git push --force --force-with-lease origin main", "block"},
		{"git push --force-with-lease=main:abc123 -f origin main", "block"},
		{"git push --force-with-lease --force-if-includes origin main", "allow"},
		{"git push --force-if-includes origin +main", "block"},
		{"git push --force-with-lease=main origin +main", "allow"},
		{"git push origin $(git branch --show-current) --force", "block"},
		{"git push origin \"$(git branch --show-current)\" --force-with-lease", "allow"},
		{"git commit -m \"$(cat <<'EOF'\nfix: stop git push --force\nEOF\n)\"", "allow"},
		{"git commit -m \"$(cat <<'EOF'\nfix: a\nEOF\n)\" && git push --force", "block"},
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
