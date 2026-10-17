package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/chemaclass/agnostic-ai/internal/gitquery"
)

// NativeMemoryDir returns the auto memory store Claude Code uses for the
// project at root when this project's settings do not move it: the
// autoMemoryDirectory of the user settings, or
// <config>/projects/<key>/memory, which every worktree of a repository
// shares (code.claude.com/docs/en/memory#storage-location).
func (Adapter) NativeMemoryDir(root string) (string, error) {
	configDir := os.Getenv("CLAUDE_CONFIG_DIR")
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("claude: auto memory store: %w", err)
	}
	key := ""
	if configDir != "" {
		// Claude Code reads the project folder name only together with
		// CLAUDE_CONFIG_DIR (code.claude.com/docs/en/env-vars).
		key = projectDirName(os.Getenv("CLAUDE_CODE_PROJECT_DIR_NAME"))
	} else {
		configDir = filepath.Join(home, ".claude")
	}
	if dir := userAutoMemoryDirectory(configDir, home); dir != "" {
		return dir, nil
	}
	if key == "" {
		key = projectKey(memoryProjectRoot(root))
	}
	return filepath.Join(configDir, "projects", key, "memory"), nil
}

// userAutoMemoryDirectory returns the autoMemoryDirectory the user
// settings set, absolute or under ~/, or "".
func userAutoMemoryDirectory(configDir, home string) string {
	data, err := os.ReadFile(filepath.Join(configDir, "settings.json"))
	if err != nil {
		return ""
	}
	var doc map[string]any
	if json.Unmarshal(data, &doc) != nil {
		return ""
	}
	dir, _ := doc[autoMemoryDirectoryKey].(string)
	return autoMemoryDirectoryPath(dir, home)
}

// claudeProjectDirName is the form Claude Code accepts for
// CLAUDE_CODE_PROJECT_DIR_NAME; it ignores any other value.
var claudeProjectDirName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// windowsDeviceName matches a name Windows reserves for a device.
var windowsDeviceName = regexp.MustCompile(`(?i)^(con|prn|aux|nul|com[1-9]|lpt[1-9])$`)

// projectDirName returns name when Claude Code would use it as the project
// folder name, or "".
func projectDirName(name string) string {
	if !claudeProjectDirName.MatchString(name) || windowsDeviceName.MatchString(name) {
		return ""
	}
	return name
}

// memoryProjectRoot returns the folder Claude Code keys auto memory on:
// the main worktree of the repository at root, the Git folder itself for a
// bare repository, or root outside Git.
func memoryProjectRoot(root string) string {
	abs := canonicalPath(root)
	common, err := gitPath(abs, "--git-common-dir")
	if err != nil {
		return abs
	}
	if filepath.Base(common) == ".git" {
		return filepath.Dir(common)
	}
	return common
}

// gitPath returns the absolute path `git rev-parse` prints for flag in the
// checkout at dir, with symlinks resolved.
func gitPath(dir, flag string) (string, error) {
	layout, err := gitquery.LayoutAt(dir)
	if err != nil {
		return "", err
	}
	if flag == "--git-dir" {
		return canonicalPath(strings.TrimSpace(layout.GitDir)), nil
	}
	return canonicalPath(strings.TrimSpace(layout.CommonDir)), nil
}

// inLinkedWorktree reports whether root is a worktree Git added beside the
// main checkout. Claude Code reads the main checkout's settings there, so
// its own store already follows the main checkout's personal store.
func inLinkedWorktree(root string) bool {
	gitDir, err := gitPath(root, "--git-dir")
	if err != nil {
		return false
	}
	common, err := gitPath(root, "--git-common-dir")
	return err == nil && gitDir != common
}

// claudeProjectKeyMax is the longest project folder name Claude Code
// writes before it cuts the name and adds a hash.
const claudeProjectKeyMax = 200

// projectKey is Claude Code's folder name for a project path: every
// character outside [A-Za-z0-9] becomes a dash, and a name longer than 200
// characters is cut there and ends in a dash and a hash of the path.
// Observed in Claude Code 2.1.296, for example
// ~/.claude/projects/-Users-me-code-my-app for /Users/me/code/my.app;
// Claude Code does not document it.
func projectKey(path string) string {
	key := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, path)
	if len(key) <= claudeProjectKeyMax {
		return key
	}
	return key[:claudeProjectKeyMax] + "-" + pathHash(path)
}

// pathHash is the 32-bit string hash Claude Code adds to a long project
// folder name, in base 36: h = h*31 + c over the UTF-16 code units.
func pathHash(path string) string {
	var h int32
	for _, c := range utf16.Encode([]rune(path)) {
		h = h<<5 - h + int32(c)
	}
	n := int64(h)
	if n < 0 {
		n = -n
	}
	return strconv.FormatInt(n, 36)
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
