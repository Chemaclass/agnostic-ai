package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/claudehooks"
	"github.com/chemaclass/agnostic-ai/internal/hookrun"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const worktreeRefreshMarker = "agnostic-ai-worktree-setup"

func newHookWorktreeRefreshCmd() *cobra.Command {
	var repo, worktree string
	cmd := &cobra.Command{
		Use:   "worktree-refresh",
		Short: "Rerun synced Claude setup in one registered linked worktree",
		Long: "Checks repository ownership and the current generated Claude setup script before resetting " +
			"only this worktree's completion marker and rerunning setup. Refuses the main checkout and " +
			"unregistered paths. Run sync in the worktree first when setup changes. Setup runs with a " +
			"600-second limit and requires Git Bash on Windows. A failed setup leaves no completion marker.",
		Example: `  agnostic-ai hook worktree-refresh --repo /path/to/main --worktree "/path/to/linked worktree"`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(repo) == "" || strings.TrimSpace(worktree) == "" {
				return fmt.Errorf("worktree refresh requires nonempty --repo and --worktree")
			}
			return refreshHookWorktree(cmd, repo, worktree)
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "Repository whose registered linked worktree may run setup")
	cmd.Flags().StringVar(&worktree, "worktree", "", "Registered linked checkout whose Claude setup should run again")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("worktree")
	return cmd
}

func refreshHookWorktree(cmd *cobra.Command, repo, worktree string) error {
	worktree, err := filepath.Abs(worktree)
	if err != nil {
		return fmt.Errorf("resolve worktree: %w", err)
	}
	worktree, err = filepath.EvalSymlinks(worktree)
	if err != nil {
		return fmt.Errorf("resolve worktree: %w", err)
	}
	if err := registeredRefreshWorktree(repo, worktree); err != nil {
		return err
	}
	previous, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	if err := os.Chdir(worktree); err != nil {
		return fmt.Errorf("%s: %w", worktree, err)
	}
	defer func() { _ = os.Chdir(previous) }()
	script, err := currentWorktreeSetup()
	if err != nil {
		return err
	}
	argv := hookrun.Argv("claude", runtime.GOOS, hookrun.Handler{Shell: "bash"})
	if runtime.GOOS == "windows" && !filepath.IsAbs(argv[0]) {
		return fmt.Errorf("claude worktree setup requires Git Bash; install Git for Windows or set CLAUDE_CODE_GIT_BASH_PATH")
	}
	if _, err := exec.LookPath(argv[0]); err != nil {
		return fmt.Errorf("claude worktree setup shell %s: %w", argv[0], err)
	}
	gitDir, err := worktreeGit(worktree, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return err
	}
	marker := filepath.Join(strings.TrimSpace(string(gitDir)), worktreeRefreshMarker)
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reset %s: %w", marker, err)
	}
	payload, err := json.Marshal(map[string]string{"cwd": filepath.ToSlash(worktree)})
	if err != nil {
		return fmt.Errorf("encode worktree setup payload: %w", err)
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 600*time.Second)
	defer cancel()
	setup := exec.CommandContext(ctx, argv[0], filepath.ToSlash(script))
	setup.Dir = worktree
	setup.Stdin = bytes.NewReader(payload)
	setup.Stdout, setup.Stderr = cmd.OutOrStdout(), cmd.ErrOrStderr()
	setup.WaitDelay = 250 * time.Millisecond
	setup.Env = worktreeGitEnvironment(os.Environ())
	setup.Env = append(setup.Env, adapters.HookTargetEnv+"=claude")
	if err := runOwnedWorktreeSetup(ctx, setup); err != nil {
		if ctx.Err() != nil {
			err = errors.Join(err, ctx.Err())
		}
		if cleanupErr := os.Remove(marker); cleanupErr != nil && !os.IsNotExist(cleanupErr) {
			err = errors.Join(err, fmt.Errorf("remove %s: %w", marker, cleanupErr))
		}
		return fmt.Errorf("claude worktree setup %s: %w", script, err)
	}
	if _, err := os.Stat(marker); err != nil {
		return fmt.Errorf("claude worktree setup did not record completion at %s: %w", marker, err)
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "refreshed Claude setup: %s\n", worktree)
	return err
}

func registeredRefreshWorktree(repo, worktree string) error {
	raw, err := worktreeGit(repo, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return err
	}
	first, registered := true, false
	for _, entry := range strings.Split(string(raw), "\x00") {
		path, ok := strings.CutPrefix(entry, "worktree ")
		if !ok {
			continue
		}
		if sameWorktreePath(path, worktree) {
			if first {
				return fmt.Errorf("%s: refusing to refresh the main checkout", worktree)
			}
			registered = true
		}
		first = false
	}
	if !registered {
		return fmt.Errorf("%s: not a registered linked worktree of %s", worktree, repo)
	}
	return verifyWorktreeRepository(repo, worktree)
}

func currentWorktreeSetup() (string, error) {
	cfg, bundle, err := loadProject(".")
	if err != nil {
		return "", err
	}
	if !slices.Contains(cfg.Targets, "claude") {
		return "", fmt.Errorf("claude worktree setup is not enabled in this project")
	}
	adapter, err := adapters.Resolve("claude")
	if err != nil {
		return "", err
	}
	files, err := captureEmit(adapter, spec.Bundle{Environments: bundle.For("claude").Environments}, cfg)
	if err != nil {
		return "", err
	}
	for _, file := range files {
		if filepath.Base(file.Path) != claudehooks.WorktreeSetupScript {
			continue
		}
		actual, err := os.ReadFile(file.Path)
		if err != nil {
			return "", fmt.Errorf("%s: run `agnostic-ai sync` in this worktree first: %w", file.Path, err)
		}
		if string(actual) != file.Content {
			return "", fmt.Errorf("%s: setup differs from current source; run `agnostic-ai sync` in this worktree first", file.Path)
		}
		return file.Path, nil
	}
	return "", fmt.Errorf("claude worktree setup is not configured; add an environment setup command and run `agnostic-ai sync`")
}
