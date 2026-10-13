package claude

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// NativeMemoryDir returns Claude Code's default auto memory store for the
// project at root, <config>/projects/<key>/memory, which every worktree
// of a repository shares (code.claude.com/docs/en/memory#storage-location).
func (Adapter) NativeMemoryDir(root string) (string, error) {
	configDir := os.Getenv("CLAUDE_CONFIG_DIR")
	key := ""
	if configDir != "" {
		// Claude Code reads the project folder name only together with
		// CLAUDE_CONFIG_DIR (code.claude.com/docs/en/env-vars).
		key = os.Getenv("CLAUDE_CODE_PROJECT_DIR_NAME")
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("claude: auto memory store: %w", err)
		}
		configDir = filepath.Join(home, ".claude")
	}
	if key == "" {
		key = projectKey(memoryProjectRoot(root))
	}
	return filepath.Join(configDir, "projects", key, "memory"), nil
}

// memoryProjectRoot returns the folder Claude Code keys auto memory on:
// the main worktree of the repository at root, or root outside Git.
func memoryProjectRoot(root string) string {
	abs := canonicalPath(root)
	out, err := exec.Command("git", "-C", abs, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return abs
	}
	common := canonicalPath(strings.TrimSpace(string(out)))
	if filepath.Base(common) == ".git" {
		return filepath.Dir(common)
	}
	// A bare repository, or one with a separate Git dir: the first
	// worktree Git lists that has files.
	out, err = exec.Command("git", "-C", abs, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return abs
	}
	for _, block := range strings.Split(strings.TrimSpace(string(out)), "\n\n") {
		lines := strings.Split(block, "\n")
		path, ok := strings.CutPrefix(lines[0], "worktree ")
		if ok && !strings.Contains("\n"+block+"\n", "\nbare\n") {
			return canonicalPath(path)
		}
	}
	return abs
}

// projectKey is Claude Code's folder name for a project path: every
// character outside [A-Za-z0-9] becomes a dash. Observed on disk, for
// example ~/.claude/projects/-Users-me-code-my-app for /Users/me/code/my.app;
// Claude Code does not document it.
func projectKey(path string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, path)
}

// canonicalPath returns path absolute with symlinks resolved, as Claude
// Code names a project after the real folder.
func canonicalPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return path
}
