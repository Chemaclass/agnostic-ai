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

const preCommitScript = "#!/bin/sh\nagnostic-ai sync --check\n"

func newInstallHookCmd() *cobra.Command {
	var shared bool
	return &cobra.Command{
		Use:   "install-hook",
		Short: "Install a pre-commit hook that runs sync --check.",
		Long: "Writes .git/hooks/pre-commit (or appends to an existing file). " +
			"With --shared, writes to .githooks/pre-commit and sets core.hooksPath so " +
			"the hook is committed alongside the project.",
		Example: `  # Install into .git/hooks/pre-commit (local only)
  agnostic-ai install-hook

  # Install into .githooks/ and set core.hooksPath (shared with team)
  agnostic-ai install-hook --shared`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return installPreCommitHook(".", shared, cmd.OutOrStdout())
		},
	}
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
	hookPath, err := writeHookAt(filepath.Join(root, ".githooks"))
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
	hookPath, err := writeHookAt(filepath.Join(gitDir, "hooks"))
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "✓ installed %s\n", hookPath)
	return nil
}

// writeHookAt ensures dir exists and writes the pre-commit hook into it,
// returning the hook's full path.
func writeHookAt(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	hookPath := filepath.Join(dir, "pre-commit")
	if err := writeOrAppendHook(hookPath); err != nil {
		return "", err
	}
	return hookPath, nil
}

// writeOrAppendHook writes the agnostic-ai sync --check line to path.
// If the file exists and already contains the line, it is a no-op.
// If it exists without the line, the line is appended.
// If it does not exist, the full shebang script is written.
func writeOrAppendHook(path string) error {
	const marker = "agnostic-ai sync --check"
	existing, err := os.ReadFile(path)
	if err == nil {
		if strings.Contains(string(existing), marker) {
			return nil
		}
		content := strings.TrimRight(string(existing), "\n") + "\n\n" + marker + "\n"
		return os.WriteFile(path, []byte(content), 0o755)
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return os.WriteFile(path, []byte(preCommitScript), 0o755)
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
