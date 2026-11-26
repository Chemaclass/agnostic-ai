package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

// autosyncCloudInstallWait bounds how long a Claude Code cloud session
// waits for the environment install hook, which Claude Code starts in
// parallel, so the sync sees what the install wrote. It stays under the
// hook's 120-second timeout.
const autosyncCloudInstallWait = 90 * time.Second

// autosyncSync runs `sync --keep-edits --quiet` in root and returns its
// stderr. A variable so unit tests can stand in for the binary.
var autosyncSync = func(ctx context.Context, root string) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find agnostic-ai: %w", err)
	}
	child := exec.CommandContext(ctx, self, "sync", "--keep-edits", "--quiet")
	var stderr bytes.Buffer
	child.Dir, child.Stdin, child.Stdout, child.Stderr = root, nil, io.Discard, &stderr
	child.Env = append(os.Environ(), envNoUpdateCheck+"=1")
	err = child.Run()
	return stderr.String(), err
}

func newHookAutosyncCmd() *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "autosync",
		Short: "Sync the project from inside a session-start hook, keeping hand edits",
		Long: "Runs `sync --keep-edits --quiet` in the project the session opens, so a fresh clone, " +
			"branch switch, pull or new worktree starts with current tool files. It never fails the session: " +
			"it exits 0 outside a project, when another sync holds the project lock, and when the sync fails, " +
			"then printing a short note to stderr. On success it prints only the files it kept, to stderr. " +
			"In a Claude Code cloud session it first waits for the environment install hook.",
		Example: "  agnostic-ai project -- hook autosync --target codex",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if target == "" {
				target = os.Getenv(adapters.HookTargetEnv)
			}
			root := autosyncProjectRoot(target)
			if root == "" {
				return nil
			}
			if target == "claude" && os.Getenv("CLAUDE_CODE_REMOTE") == "true" {
				waitForCloudInstall(root, autosyncCloudInstallWait)
			}
			stderr, err := autosyncSync(cmd.Context(), root)
			reportAutosync(cmd.ErrOrStderr(), stderr, err)
			return nil
		},
	}
	cmd.Flags().StringVarP(&target, "target", "t", "", "Target that runs the hook (default $"+adapters.HookTargetEnv+")")
	return cmd
}

// autosyncProjectRoot is the nearest ancestor of the hook's directory
// with a project config, never the user source root, or "" outside a
// project. Unlike the memory hook it never falls back to the Git root:
// a checkout with no config has nothing to sync.
func autosyncProjectRoot(target string) string {
	wd := os.Getenv(hookProjectDirEnv[target])
	if wd == "" {
		if target == "claude" {
			wd = os.Getenv("CLAUDE_PROJECT_DIR")
		}
	}
	if wd == "" {
		var err error
		if wd, err = os.Getwd(); err != nil {
			return ""
		}
	}
	return configuredMemoryProject(canonicalDir(wd), "", userRootMatcher())
}

// reportAutosync names the outputs sync kept, stays silent when another
// sync holds the project lock, and otherwise turns a failure into a
// short note.
func reportAutosync(out io.Writer, stderr string, err error) {
	if err == nil {
		scanner := bufio.NewScanner(strings.NewReader(stderr))
		for scanner.Scan() {
			if strings.HasPrefix(strings.TrimSpace(scanner.Text()), "~ kept") {
				_, _ = fmt.Fprintln(out, "agnostic-ai: "+strings.TrimSpace(scanner.Text()))
			}
		}
		return
	}
	if strings.Contains(stderr, "project is locked by agnostic-ai") {
		return
	}
	_, _ = fmt.Fprintln(out, "agnostic-ai: autosync could not sync; run `agnostic-ai sync` to see why")
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	if len(lines) > 3 {
		lines = lines[len(lines)-3:]
	}
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			_, _ = fmt.Fprintln(out, "  "+line)
		}
	}
}

// waitForCloudInstall waits, up to limit, while the Claude Code cloud
// install script holds its lock in the Git directory. Claude Code runs
// SessionStart hooks in parallel, so without this the sync could run
// before the install finishes. No lock file means no install hook.
func waitForCloudInstall(root string, limit time.Duration) {
	gitDir, err := gitOutput(root, nil, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return
	}
	path := filepath.Join(strings.TrimSpace(gitDir), "agnostic-ai-cloud-install.lock")
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	deadline := time.Now().Add(limit)
	for {
		err := tryLockProjectFile(file)
		if err == nil || !projectLockContended(err) || time.Now().After(deadline) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}
