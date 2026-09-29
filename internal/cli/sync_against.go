package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// The two states `sync --check --against` compares: what is staged for the
// next commit, and the last commit.
const (
	againstIndex = "index"
	againstHEAD  = "HEAD"
)

// againstGitTimeout bounds each git call that exports a tree: a large
// repository takes longer than the 2 second budget of a status query.
const againstGitTimeout = 60 * time.Second

func validateAgainst(ref string, check, plan, watch, global bool) error {
	if ref == "" {
		return nil
	}
	if ref != againstIndex && ref != againstHEAD {
		return fmt.Errorf("--against: expected %q or %q, got %q", againstIndex, againstHEAD, ref)
	}
	if !check {
		return errs.Coded(errs.CodeFlagConflict, "--against requires --check")
	}
	if plan || watch || global {
		return errs.Coded(errs.CodeFlagConflict, "--against cannot be combined with --plan, --watch, or --global")
	}
	return nil
}

// enterAgainstTree exports the Git index or HEAD to a temporary directory
// and makes the matching project directory the working directory, so a
// check reads the specs and outputs as Git holds them. The directory is a
// repository of its own, which lets againstIgnored apply the exported
// .gitignore. The caller ends with leave.
func enterAgainstTree(ref string) (_ *againstTree, err error) {
	origin, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	toplevel, err := gitOutput(origin, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("--against %s needs a Git work tree: %w", ref, err)
	}
	prefix, err := gitOutput(origin, nil, "rev-parse", "--show-prefix")
	if err != nil {
		return nil, fmt.Errorf("--against %s: %w", ref, err)
	}
	scratch, err := os.MkdirTemp("", "agnostic-ai-against-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(scratch)
		}
	}()
	root := filepath.Join(scratch, "tree")
	if err := os.Mkdir(root, 0o755); err != nil {
		return nil, err
	}
	toplevel = strings.TrimSpace(toplevel)
	var env []string
	if ref == againstHEAD {
		env = append(os.Environ(), "GIT_INDEX_FILE="+filepath.Join(scratch, "index"))
		if _, err := gitOutput(toplevel, env, "read-tree", "HEAD"); err != nil {
			return nil, fmt.Errorf("--against %s: %w", ref, err)
		}
	}
	if _, err := gitOutput(toplevel, env, "checkout-index", "--all", "--force", "--prefix="+root+string(filepath.Separator)); err != nil {
		return nil, fmt.Errorf("--against %s: export: %w", ref, err)
	}
	if _, err := gitOutput(root, isolatedGitEnv(), "init", "--quiet"); err != nil {
		return nil, fmt.Errorf("--against %s: %w", ref, err)
	}
	entries, err := gitOutput(toplevel, env, "ls-files", "--stage", "-z")
	if err != nil {
		return nil, fmt.Errorf("--against %s: %w", ref, err)
	}
	if err := gitInput(root, isolatedGitEnv(), entries, "update-index", "-z", "--index-info"); err != nil {
		return nil, fmt.Errorf("--against %s: index: %w", ref, err)
	}
	project := filepath.Join(root, filepath.FromSlash(strings.TrimSpace(prefix)))
	if err := os.Chdir(project); err != nil {
		return nil, fmt.Errorf("--against %s: %w", ref, err)
	}
	return &againstTree{origin: origin, scratch: scratch, root: root, gitEnv: isolateGitEnv()}, nil
}

// isolateGitEnv unsets the variables that point git at another
// repository, so every git call the check makes reads the export, and
// returns them for leave to restore. A pre-commit hook sets GIT_DIR and
// GIT_INDEX_FILE to the real repository.
func isolateGitEnv() map[string]string {
	saved := map[string]string{}
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "GIT_") && !strings.HasPrefix(k, "GIT_CONFIG") && !strings.HasPrefix(k, "GIT_TERMINAL") {
			saved[k] = v
			_ = os.Unsetenv(k)
		}
	}
	return saved
}

// againstTree is the export enterAgainstTree made.
type againstTree struct {
	origin, scratch, root string
	gitEnv                map[string]string
}

// leave restores the working directory and git environment, and removes
// the export.
func (t *againstTree) leave() {
	_ = os.Chdir(t.origin)
	for k, v := range t.gitEnv {
		_ = os.Setenv(k, v)
	}
	_ = os.RemoveAll(t.scratch)
}

// ignored returns the paths the exported .gitignore ignores, relative to
// the project directory the working directory now is.
func (t *againstTree) ignored(paths []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(paths) == 0 {
		return out, nil
	}
	var stdin bytes.Buffer
	for _, p := range paths {
		stdin.WriteString(filepath.ToSlash(p))
		stdin.WriteByte(0)
	}
	cmd := exec.Command("git", "check-ignore", "-z", "--stdin")
	cmd.Env = isolatedGitEnv()
	cmd.Stdin = &stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// check-ignore exits 1 when it matches nothing.
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
			return nil, fmt.Errorf("git check-ignore: %w: %s", err, strings.TrimSpace(stderr.String()))
		}
	}
	for _, p := range strings.Split(stdout.String(), "\x00") {
		if p != "" {
			out[p] = true
		}
	}
	return out, nil
}

// trackedDrift narrows reports from a check of an exported tree to what
// that state gets wrong about the outputs Git tracks there: an output
// that differs from what the specs render, or one the state lacks that
// its .gitignore does not ignore, and a tracked file that carries the
// provenance header where a target writes but that no spec produces. The
// ledger is never committed, so ledger findings say nothing about the
// state, and a tracked leftover is one to delete by hand.
func (t *againstTree) trackedDrift(reports []driftReport) ([]driftReport, error) {
	var missing []string
	for _, r := range reports {
		for _, f := range r.Missing {
			missing = append(missing, f.Path)
		}
	}
	ignored, err := t.ignored(missing)
	if err != nil {
		return nil, err
	}
	var out []driftReport
	for _, r := range reports {
		kept := driftReport{Target: r.Target, Stale: r.Stale, Edited: r.Edited}
		if r.Target == unledgeredReportTarget {
			kept.Unledgered = true
			kept.Orphaned = append(append([]string{}, r.Orphaned...), r.Leftover...)
		}
		for _, f := range r.Missing {
			if !ignored[filepath.ToSlash(f.Path)] {
				kept.Missing = append(kept.Missing, f)
			}
		}
		out = append(out, kept)
	}
	return out, nil
}

// againstHint says which step settles drift found against ref.
func againstHint(ref string) string {
	if ref == againstHEAD {
		return "the check compared the last commit; commit the regenerated files"
	}
	return "the check compared the Git index; stage the regenerated files with git add"
}

// gitInput runs git in dir with the environment env, feeding it stdin.
func gitInput(dir string, env []string, stdin string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), againstGitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// gitOutput runs git in dir with the environment env (the process
// environment when nil) and returns its standard output.
func gitOutput(dir string, env []string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), againstGitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// isolatedGitEnv is the process environment without the variables that
// point git at another repository. A git hook runs with GIT_DIR and
// GIT_INDEX_FILE set, which would send a command meant for the export to
// the real repository.
func isolatedGitEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") || strings.HasPrefix(kv, "GIT_CONFIG") || strings.HasPrefix(kv, "GIT_TERMINAL") {
			env = append(env, kv)
		}
	}
	return env
}
