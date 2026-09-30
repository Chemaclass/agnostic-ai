package claude

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/claudehooks"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func environmentBundle(meta map[string]any) spec.Bundle {
	meta["name"] = "dev"
	return spec.NewBundle([]spec.Entry{{Kind: spec.KindEnvironment, Name: "dev", Path: "environments/dev.yaml", Meta: meta}})
}

func readHookSettings(t *testing.T, cwd string) claudehooks.Settings {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(cwd, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s claudehooks.Settings
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("parse settings.json: %v\n%s", err, raw)
	}
	return s
}

// An environment spec's setup runs from a SessionStart hook for a new
// session, a SubagentStart hook for a subagent worktree, and a
// PostToolUse hook after EnterWorktree (#1498).
func TestEmit_EnvironmentSetupWritesWorktreeHooks(t *testing.T) {
	cwd := t.TempDir()
	testutil.Chdir(t, cwd)
	b := environmentBundle(map[string]any{"setup": "composer install"})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	s := readHookSettings(t, cwd)
	want := `f="$CLAUDE_PROJECT_DIR/.claude/hooks/agnostic-ai-worktree-setup.sh"; [ ! -f "$f" ] || sh "$f"`
	for event, matcher := range map[string]string{"SessionStart": "startup", "SubagentStart": "", "PostToolUse": "EnterWorktree"} {
		groups := s.Hooks[event]
		if len(groups) != 1 || groups[0].Matcher != matcher || len(groups[0].Hooks) != 1 {
			t.Errorf("%s hooks = %+v, want one %q group with one handler", event, groups, matcher)
			continue
		}
		if h := groups[0].Hooks[0]; h.Command != want || h.Shell != "bash" {
			t.Errorf("%s handler = %+v, want bash running %s", event, h, want)
		}
	}
	script := filepath.Join(cwd, ".claude", "hooks", claudehooks.WorktreeSetupScript)
	body, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "\ncomposer install\n") {
		t.Errorf("script does not run setup:\n%s", body)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(script); err != nil || info.Mode().Perm()&0o100 == 0 {
			t.Errorf("script mode = %v, %v; want executable", info.Mode(), err)
		}
	}
}

// A worktree checked out without the script, such as one created before
// the first sync, runs a hook command that does nothing.
func TestEmit_WorktreeSetupCommandSkipsMissingScript(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not on PATH")
	}
	cwd := t.TempDir()
	testutil.Chdir(t, cwd)
	if err := New().Emit(emit.NewSession(), environmentBundle(map[string]any{"setup": "false"}), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	command := readHookSettings(t, cwd).Hooks["SessionStart"][0].Hooks[0].Command
	if err := os.Remove(filepath.Join(cwd, ".claude", "hooks", claudehooks.WorktreeSetupScript)); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", command)
	cmd.Env = append(os.Environ(), "CLAUDE_PROJECT_DIR="+cwd, "AGNOSTIC_AI_TARGET=claude")
	cmd.Stdin = strings.NewReader(`{"cwd":"` + cwd + `"}`)
	out, err := cmd.CombinedOutput()
	if err != nil || len(out) != 0 {
		t.Errorf("hook with no script: err %v, output %q; want exit 0 and no output", err, out)
	}
}

// `x-claude.setup: false` keeps setup for other tools and writes no hook.
func TestEmit_EnvironmentSetupOptOut(t *testing.T) {
	cwd := t.TempDir()
	testutil.Chdir(t, cwd)
	b := environmentBundle(map[string]any{"setup": "composer install", "x-claude": map[string]any{"setup": false}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Errorf("settings.json written for an opted-out setup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, ".claude", "hooks", claudehooks.WorktreeSetupScript)); !os.IsNotExist(err) {
		t.Errorf("setup script written for an opted-out setup: %v", err)
	}
}

// The script follows outputs.claude.dir, as settings.json does.
func TestEmit_EnvironmentSetupFollowsOutputDir(t *testing.T) {
	cwd := t.TempDir()
	testutil.Chdir(t, cwd)
	cfg := &config.Config{Outputs: map[string]config.Output{"claude": {Dir: "tools/claude"}}}
	if err := New().Emit(emit.NewSession(), environmentBundle(map[string]any{"setup": []any{"a", "b"}}), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(cwd, "tools", "claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `$CLAUDE_PROJECT_DIR/tools/claude/hooks/agnostic-ai-worktree-setup.sh`) {
		t.Errorf("settings.json does not point at the moved script:\n%s", raw)
	}
	body, err := os.ReadFile(filepath.Join(cwd, "tools", "claude", "hooks", claudehooks.WorktreeSetupScript))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "\na\nb\n") {
		t.Errorf("script does not run each setup command:\n%s", body)
	}
}

// The generated script runs setup once in each linked worktree, never in
// the main checkout, and retries after a failed run. It runs from the
// worktree root when the session starts in a subdirectory, and an `exit`
// in setup still records the run.
func TestWorktreeSetupScript_RunsOncePerLinkedWorktree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the script runs through sh")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	main := filepath.Join(root, "main")
	counter := filepath.Join(root, "runs")
	gate := filepath.Join(root, "fail")
	gitRun := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(main, "init", "-q")
	gitRun(main, "commit", "-q", "--allow-empty", "-m", "init")
	worktree := filepath.Join(root, "wt")
	gitRun(main, "worktree", "add", "-q", worktree)

	script := filepath.Join(root, "setup.sh")
	setup := "test ! -e " + emit.ShellQuote(gate) + "\npwd >> " + emit.ShellQuote(counter) + "\nexit 0"
	sub := filepath.Join(worktree, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte(worktreeSetupScript(setup)), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(cwd string) (string, error) {
		t.Helper()
		payload, _ := json.Marshal(map[string]string{"hook_event_name": "SessionStart", "cwd": cwd})
		cmd := exec.Command("sh", script)
		cmd.Dir = main
		cmd.Stdin = strings.NewReader(string(payload))
		cmd.Env = append(os.Environ(), "AGNOSTIC_AI_TARGET=claude")
		out, err := cmd.Output()
		return string(out), err
	}
	runs := func() []string {
		raw, _ := os.ReadFile(counter)
		return strings.Fields(string(raw))
	}

	if _, err := run(main); err != nil {
		t.Fatalf("main checkout: %v", err)
	}
	if got := runs(); len(got) != 0 {
		t.Fatalf("setup ran in the main checkout: %v", got)
	}

	if err := os.WriteFile(gate, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run(worktree); err == nil {
		t.Fatal("a failing setup exited 0")
	}
	if err := os.Remove(gate); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		out, err := run(sub)
		if err != nil {
			t.Fatalf("worktree: %v", err)
		}
		if out != "" {
			t.Errorf("setup wrote to stdout, which Claude Code adds to the context: %q", out)
		}
	}
	got := runs()
	if len(got) != 1 {
		t.Fatalf("setup runs = %v, want one", got)
	}
	resolved, _ := filepath.EvalSymlinks(worktree)
	if got[0] != worktree && got[0] != resolved {
		t.Errorf("setup ran in %s, want the worktree %s", got[0], worktree)
	}
}
