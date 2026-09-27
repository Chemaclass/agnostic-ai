package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const projectHookCheck = "agnostic-ai sync --check"

// globalHookChecks runs from the home's repository root, so the checks
// read the home being committed even when AGNOSTIC_AI_HOME names another.
const globalHookChecks = `AGNOSTIC_AI_HOME="$(git rev-parse --show-toplevel)" || exit 1
export AGNOSTIC_AI_HOME
agnostic-ai lint --global --strict || exit 1
agnostic-ai validate --global || exit 1
agnostic-ai sync --global --check || exit 1`

func newInstallHookCmd() *cobra.Command {
	var shared, global bool
	cmd := &cobra.Command{
		Use:   "install-hook",
		Short: "Install a pre-commit hook that runs sync --check.",
		Long: "Writes .git/hooks/pre-commit (or appends to an existing file). " +
			"With --shared, writes to .githooks/pre-commit and sets core.hooksPath so " +
			"the hook is committed alongside the project. With --global, run in the " +
			"global home kept in git, writes a hook that runs lint --global --strict, " +
			"validate --global, and sync --global --check.",
		Example: `  # Install into .git/hooks/pre-commit (local only)
  agnostic-ai install-hook

  # Install into .githooks/ and set core.hooksPath (shared with team)
  agnostic-ai install-hook --shared

  # Gate commits to the global home, run inside ~/.agnostic-ai
  agnostic-ai install-hook --global`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if global {
				return installGlobalPreCommitHook(".", cmd.OutOrStdout())
			}
			if err := refuseGlobalHome(".", globalHomeHookRemedy); err != nil {
				return err
			}
			return installPreCommitHook(".", shared, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&shared, "shared", false, "Write .githooks/pre-commit and set core.hooksPath so the hook is shared with the team.")
	cmd.Flags().BoolVar(&global, "global", false, "Write the pre-commit hook of the global home in $AGNOSTIC_AI_HOME (default ~/.agnostic-ai), which must be the root of a git repository.")
	cmd.MarkFlagsMutuallyExclusive("shared", "global")
	return cmd
}

// installGlobalPreCommitHook writes the global home's pre-commit hook.
// dir must be the home itself, at the root of its git repository, so
// the hook's checks read the same home the commit is for.
func installGlobalPreCommitHook(dir string, out io.Writer) error {
	source, err := globalSourceRoot()
	if err != nil {
		return err
	}
	home, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("global home %s: %w", source, err)
	}
	if here, err := os.Stat(dir); err != nil || !os.SameFile(here, home) {
		return fmt.Errorf("install-hook --global runs in the global home %s; cd there first, or set %s to this directory", source, envUserGlobalRoot)
	}
	top, err := gitRevParse(source, "--show-toplevel")
	if err != nil {
		return fmt.Errorf("%s is not the root of a git repository; run `git init` there first", source)
	}
	if info, err := os.Stat(top); err != nil || !os.SameFile(info, home) {
		return fmt.Errorf("%s is not the root of a git repository (it sits inside %s); keep the global home in its own repository", source, top)
	}
	commonDir, err := gitRevParse(source, "--git-common-dir")
	if err != nil {
		return err
	}
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(source, commonDir)
	}
	hookPath, err := writeHookAt(filepath.Join(commonDir, "hooks"), globalHookChecks, "agnostic-ai sync --global --check")
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "✓ installed %s\n", hookPath)
	return nil
}

func gitRevParse(dir, arg string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", arg).Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s in %s: %w", arg, dir, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func installPreCommitHook(root string, shared bool, out io.Writer) error {
	if shared {
		return installSharedHook(root, out)
	}
	return installLocalHook(root, out)
}

// installSharedHook writes .githooks/pre-commit and sets core.hooksPath so the
// hook lives in the repo and runs for every collaborator.
func installSharedHook(root string, out io.Writer) error {
	hookPath, err := writeHookAt(filepath.Join(root, ".githooks"), projectHookCheck, projectHookCheck)
	if err != nil {
		return err
	}
	if err := exec.Command("git", "-C", root, "config", "core.hooksPath", ".githooks").Run(); err != nil {
		return fmt.Errorf("git config core.hooksPath: %w", err)
	}
	_, _ = fmt.Fprintf(out, "✓ installed %s (core.hooksPath → .githooks)\n", hookPath)
	return nil
}

// installLocalHook writes .git/hooks/pre-commit, scoped to the local clone.
func installLocalHook(root string, out io.Writer) error {
	gitDir, err := findGitDir(root)
	if err != nil {
		return err
	}
	hookPath, err := writeHookAt(filepath.Join(gitDir, "hooks"), projectHookCheck, projectHookCheck)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "✓ installed %s\n", hookPath)
	return nil
}

// writeHookAt ensures dir exists and writes the pre-commit hook into it,
// returning the hook's full path.
func writeHookAt(dir, checks, marker string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	hookPath := filepath.Join(dir, "pre-commit")
	if err := writeOrAppendHook(hookPath, checks, marker); err != nil {
		return "", err
	}
	return hookPath, nil
}

// writeOrAppendHook writes the checks to path.
// If the file exists and already contains marker, it is a no-op.
// If it exists without marker, the checks are appended.
// If it does not exist, a shebang script with the checks is written.
func writeOrAppendHook(path, checks, marker string) error {
	existing, err := os.ReadFile(path)
	if err == nil {
		if strings.Contains(string(existing), marker) {
			return nil
		}
		content := strings.TrimRight(string(existing), "\n") + "\n\n" + checks + "\n"
		return os.WriteFile(path, []byte(content), 0o755)
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return os.WriteFile(path, []byte("#!/bin/sh\n"+checks+"\n"), 0o755)
}

func findGitDir(root string) (string, error) {
	// Walk up from root looking for .git.
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	for dir := abs; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, ".git")
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", fmt.Errorf("not a git repository (no .git directory found)")
}
