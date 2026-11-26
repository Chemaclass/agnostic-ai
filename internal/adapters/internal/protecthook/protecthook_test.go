package protecthook_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/protecthook"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/protecthook/hooktest"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// fileReader takes the one path in a {"file": "..."} payload.
const fileReader = `function start() { buf = "" }
function text(s) { buf = buf s }
function finish(    p) {
  if (match(buf, /"file":"[^"]*"/)) {
    p = substr(buf, RSTART + 8, RLENGTH - 9)
    paths[++npaths] = p
  }
}
`

// The tool folders sync writes hook scripts to, two, three and four
// levels below the project root (Claude Code, Copilot, Goose).
var hookDirs = []string{".claude/hooks", ".github/hooks/scripts", ".agents/plugins/agnostic-ai/hooks"}

// writeHook renders the hook for a tool whose scripts sync writes to
// dir and puts it at root/at.
func writeHook(t *testing.T, root, dir, at string) string {
	t.Helper()
	script := protecthook.Render(protecthook.Reader{Edits: "test edits", Tool: "Test", Dir: dir, Awk: fileReader},
		[]spec.ProtectGroup{{Paths: []string{"secret.txt"}, Decision: spec.ProtectDeny}})
	path := filepath.Join(root, filepath.FromSlash(at), protecthook.ScriptName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// editSecret runs the hook from cwd on an edit to secret.txt there.
func editSecret(t *testing.T, script, cwd string) hooktest.Result {
	t.Helper()
	return hooktest.RunAll(t, script, cwd, []byte(`{"file":"secret.txt"}`))
}

func wantBlocked(t *testing.T, run hooktest.Result) {
	t.Helper()
	if run.Code != 2 || !strings.Contains(run.Stderr, "secret.txt is protected") {
		t.Errorf("exit = %d, stderr = %q; want secret.txt blocked", run.Code, run.Stderr)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "init.defaultBranch=main"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func needGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	for _, dir := range []string{"/usr/bin", "/bin"} {
		if _, err := os.Stat(filepath.Join(dir, "git")); err == nil {
			return
		}
	}
	t.Skip("the hook runs with PATH=/usr/bin:/bin, which has no git")
}

// While the script sits where sync wrote it, the folder that holds the
// tool folder is the root, at any depth and with no config or Git.
func TestRootLookup_FindsTheRootFromWhereSyncWroteTheScript(t *testing.T) {
	for _, dir := range hookDirs {
		t.Run(dir, func(t *testing.T) {
			root := t.TempDir()
			wantBlocked(t, editSecret(t, writeHook(t, root, dir, dir), root))
		})
	}
}

// A script copied to another tool's folder finds the root from the
// config file above it, at any depth, outside Git.
func TestRootLookup_CopiedScriptWalksUpToTheConfig(t *testing.T) {
	for _, config := range []string{"agnostic-ai.yaml", "agnostic.config.yaml"} {
		for _, dir := range hookDirs {
			t.Run(config+"/"+dir, func(t *testing.T) {
				root := t.TempDir()
				touch(t, filepath.Join(root, config))
				wantBlocked(t, editSecret(t, writeHook(t, root, ".codex/hooks", dir), root))
			})
		}
	}
}

// A project below the Git root keeps its own root: the config wins
// over the Git worktree root.
func TestRootLookup_ConfigWinsOverTheGitRoot(t *testing.T) {
	needGit(t)
	top := t.TempDir()
	git(t, top, "init", "-q")
	root := filepath.Join(top, "apps", "web")
	touch(t, filepath.Join(root, "agnostic-ai.yaml"))
	for _, dir := range hookDirs {
		t.Run(dir, func(t *testing.T) {
			wantBlocked(t, editSecret(t, writeHook(t, root, ".codex/hooks", dir), root))
		})
	}
}

// With no config above a copied script, the Git worktree root is the
// root, and in a linked worktree it is that worktree, not the main
// checkout.
func TestRootLookup_CopiedScriptFallsBackToTheGitWorktree(t *testing.T) {
	needGit(t)
	main := filepath.Join(t.TempDir(), "main")
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, main, "init", "-q")
	git(t, main, "commit", "-q", "--allow-empty", "-m", "init")
	linked := filepath.Join(filepath.Dir(main), "linked")
	git(t, main, "worktree", "add", "-q", linked)

	for name, root := range map[string]string{"main": main, "linked worktree": linked} {
		for _, dir := range hookDirs {
			t.Run(name+"/"+dir, func(t *testing.T) {
				wantBlocked(t, editSecret(t, writeHook(t, root, ".codex/hooks", dir), root))
			})
		}
	}
	t.Run("linked worktree at the synced path", func(t *testing.T) {
		wantBlocked(t, editSecret(t, writeHook(t, linked, ".codex/hooks", ".codex/hooks"), linked))
	})
}

// A copied script with no config and no Git above it cannot know the
// root, so it blocks the edit rather than let it through unchecked.
func TestRootLookup_BlocksWhenTheRootIsUnknown(t *testing.T) {
	for _, dir := range hookDirs {
		t.Run(dir, func(t *testing.T) {
			root := t.TempDir()
			run := hooktest.RunAll(t, writeHook(t, root, ".codex/hooks", dir), root, []byte(`{"file":"other.txt"}`))
			if run.Code != 2 || !strings.Contains(run.Stderr, "the project root is unknown") {
				t.Errorf("exit = %d, stderr = %q; want the edit blocked for an unknown root", run.Code, run.Stderr)
			}
		})
	}
}

// An edit outside the protected paths still passes once the root is
// found, so the lookup does not block every edit.
func TestRootLookup_AllowsUnprotectedEdits(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "agnostic-ai.yaml"))
	run := hooktest.RunAll(t, writeHook(t, root, ".codex/hooks", hookDirs[2]), root, []byte(`{"file":"other.txt"}`))
	if run.Code != 0 {
		t.Errorf("exit = %d, stderr = %q; want the edit allowed", run.Code, run.Stderr)
	}
}
