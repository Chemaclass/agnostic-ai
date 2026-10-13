package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// Sync writes this checkout's absolute path in its local settings.
const autoMemoryDirectoryKey = "autoMemoryDirectory"

// emitAutoMemoryDirectory points Claude Code's auto memory at the shared
// personal store, so what Claude saves on its own reaches every tool
// (#1846) and Claude Code loads the personal index itself. A value the
// user set stays theirs.
func emitAutoMemoryDirectory(sess *emit.Session, dir string, cfg *config.Config, dryRun bool) error {
	if !slices.Contains(cfg.Builtins, emit.MemoryBuiltin) {
		return nil
	}
	path := filepath.Join(dir, "settings.local.json")
	if sess.HasJSONKey(path, autoMemoryDirectoryKey, dryRun) && !emit.ClaimsKey(path, autoMemoryDirectoryKey) {
		return nil
	}
	store, err := emit.PersonalMemoryDir(cfg, ".")
	if err != nil {
		return err
	}
	if err := sess.CreateRepoMemoryStore(cfg, store, dryRun); err != nil {
		return err
	}
	value, err := autoMemoryDirectoryValue(store)
	if err != nil {
		return err
	}
	// The file holds this checkout's absolute path, so it must never be
	// committed, even when sync creates it before Claude Code ignores it.
	if !dryRun && !sess.IsCapturing() {
		if err := emit.ExcludeFromGit("**/" + filepath.ToSlash(path)); err != nil {
			return err
		}
	}
	return sess.MergeJSONFile(path, map[string]any{autoMemoryDirectoryKey: value}, dryRun)
}

// autoMemoryDirectoryValue is the autoMemoryDirectory value sync writes
// for the personal store.
func autoMemoryDirectoryValue(store string) (string, error) {
	abs, err := filepath.Abs(store)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", emit.PersonalMemoryIndexPath, err)
	}
	return filepath.ToSlash(abs), nil
}

func AutoMemoryLoadsPersonalIndex(cfg *config.Config) (bool, error) {
	dir, err := New().NativePersonalMemoryDir(cfg, ".")
	return dir != "", err
}

func (Adapter) NativePersonalMemoryDir(cfg *config.Config, root string) (string, error) {
	if !slices.Contains(cfg.Builtins, emit.MemoryBuiltin) {
		return "", nil
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", root, err)
	}
	root = absRoot
	dir := emit.OutputDir(cfg, target, defaultDir)
	path := filepath.Join(dir, "settings.local.json")
	settingsPath := path
	if !filepath.IsAbs(settingsPath) {
		settingsPath = filepath.Join(root, path)
	}
	mainSettings, mainStore := mainCheckout(root, settingsPath)
	projectPath := filepath.Join(dir, "settings.json")
	if !filepath.IsAbs(projectPath) {
		projectPath = filepath.Join(root, projectPath)
	}
	first, own := readAutoMemorySettings(mainSettings), readAutoMemorySettings(settingsPath)
	scopes := []autoMemorySettings{first, own, readAutoMemorySettings(projectPath), readAutoMemorySettings(userSettingsPath())}
	if !autoMemoryOn(scopes...) {
		return "", nil
	}
	store, err := emit.PersonalMemoryDir(cfg, root)
	if err != nil {
		return "", err
	}
	value := first.directory
	directorySource := mainSettings
	if value == nil {
		directorySource = settingsPath
		if !cfg.IsUnmanaged(path) && (own.directory == nil || emit.ClaimsKey(path, autoMemoryDirectoryKey)) {
			if autoMemoryDirectoryBlocked(scopes, directorySource) {
				return "", nil
			}
			return store, nil
		}
		value = own.directory
	}
	if autoMemoryDirectoryBlocked(scopes, directorySource) {
		return "", nil
	}
	var named string
	if json.Unmarshal(value, &named) != nil || !filepath.IsAbs(filepath.FromSlash(named)) {
		return "", nil
	}
	absStore, err := filepath.Abs(store)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", emit.PersonalMemoryIndexPath, err)
	}
	named = filepath.FromSlash(named)
	if emit.SamePath(named, absStore) {
		return store, nil
	}
	if mainStore != "" && emit.SamePath(named, mainStore) {
		return mainStore, nil
	}
	return "", nil
}

func autoMemoryDirectoryBlocked(scopes []autoMemorySettings, source string) bool {
	for _, scope := range scopes {
		if scope.blockRepoDirectory != nil && *scope.blockRepoDirectory {
			return repositorySuppliedLocalSettings(source)
		}
	}
	return false
}

func repositorySuppliedLocalSettings(path string) bool {
	parent := filepath.Dir(path)
	if info, err := os.Lstat(parent); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	for {
		if _, err := os.Stat(parent); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return true
		}
		next := filepath.Dir(parent)
		if next == parent {
			return true
		}
		parent = next
	}
	out, err := exec.Command("git", "-C", parent, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return true
	}
	root := strings.TrimSpace(string(out))
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return true
	}
	cmd := exec.Command("git", "-C", root, "ls-files", "--error-unmatch", "--", filepath.ToSlash(relative))
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
			return false
		}
	}
	return true
}

func mainCheckout(rootPath, path string) (settings, store string) {
	root, project, ok := emit.MainCheckoutAt(rootPath)
	if !ok {
		return "", ""
	}
	if wd, err := filepath.Abs(rootPath); err == nil && !emit.SamePath(project, wd) {
		store = filepath.Join(project, filepath.Dir(filepath.FromSlash(emit.PersonalMemoryIndexPath)))
	}
	// Claude Code reads the starting folder's file alone on Windows and
	// when the repository root is the home folder.
	if home, err := os.UserHomeDir(); runtime.GOOS == "windows" || err == nil && emit.SamePath(root, home) {
		return "", store
	}
	settings = filepath.Join(root, ".claude", "settings.local.json")
	if abs, err := filepath.Abs(path); err == nil && emit.SamePath(settings, abs) {
		return "", store
	}
	return settings, store
}

type autoMemorySettings struct {
	enabled            *bool
	directory          json.RawMessage
	blockRepoDirectory *bool
	disableEnvironment *string
}

func readAutoMemorySettings(path string) autoMemorySettings {
	var s autoMemorySettings
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	stripped, _ := emit.StripJSONC(data)
	var doc map[string]json.RawMessage
	if json.Unmarshal(stripped, &doc) != nil {
		return s
	}
	var enabled *bool
	if raw, ok := doc["autoMemoryEnabled"]; ok && json.Unmarshal(raw, &enabled) == nil {
		s.enabled = enabled
	}
	s.directory = doc[autoMemoryDirectoryKey]
	var permissions struct {
		BlockReads *bool `json:"blockReadsOutsideWorkingDirectories"`
	}
	if json.Unmarshal(doc["permissions"], &permissions) == nil {
		s.blockRepoDirectory = permissions.BlockReads
	}
	var environment map[string]string
	if json.Unmarshal(doc["env"], &environment) == nil {
		if value, ok := environment["CLAUDE_CODE_DISABLE_AUTO_MEMORY"]; ok {
			s.disableEnvironment = &value
		}
	}
	return s
}

func autoMemoryOn(scopes ...autoMemorySettings) bool {
	disabled := os.Getenv("CLAUDE_CODE_DISABLE_AUTO_MEMORY")
	for _, scope := range scopes {
		if scope.disableEnvironment != nil {
			disabled = *scope.disableEnvironment
			break
		}
	}
	switch disabled {
	case "":
	case "0":
		return true
	default:
		return false
	}
	for _, s := range scopes {
		if s.enabled != nil {
			return *s.enabled
		}
	}
	return true
}

// userSettingsPath is Claude Code's user settings file, under
// CLAUDE_CONFIG_DIR when it is set, or "" with no home folder.
func userSettingsPath() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "settings.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "settings.json")
}
