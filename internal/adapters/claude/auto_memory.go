package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// autoMemoryDirectoryKey is Claude Code's setting for where auto memory
// lives. It takes an absolute path only, so sync writes it to the
// personal, git-ignored settings.local.json (code.claude.com/docs/en/memory).
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

// AutoMemoryLoadsPersonalIndex reports whether Claude Code's auto memory
// loads a personal memory index after sync: the memory built-in is on,
// no setting or variable sync can read turns auto memory off, and the
// autoMemoryDirectory Claude Code reads names a personal store, as sync
// writes it, as the user set it, or as a worktree Claude Code created
// copied it from the main checkout.
func AutoMemoryLoadsPersonalIndex(cfg *config.Config) (bool, error) {
	if !slices.Contains(cfg.Builtins, emit.MemoryBuiltin) {
		return false, nil
	}
	dir := emit.OutputDir(cfg, target, defaultDir)
	path := filepath.Join(dir, "settings.local.json")
	mainSettings, mainStore := mainCheckout(path)
	first, own := readAutoMemorySettings(mainSettings), readAutoMemorySettings(path)
	if !autoMemoryOn(first, own, readAutoMemorySettings(filepath.Join(dir, "settings.json")), readAutoMemorySettings(userSettingsPath())) {
		return false, nil
	}
	value := first.directory
	if value == nil {
		if own.directory == nil || emit.ClaimsKey(path, autoMemoryDirectoryKey) {
			return true, nil
		}
		value = own.directory
	}
	var named string
	if json.Unmarshal(value, &named) != nil || !filepath.IsAbs(filepath.FromSlash(named)) {
		return false, nil
	}
	store, err := emit.PersonalMemoryDir(cfg, ".")
	if err != nil {
		return false, err
	}
	if store, err = filepath.Abs(store); err != nil {
		return false, fmt.Errorf("resolve %s: %w", emit.PersonalMemoryIndexPath, err)
	}
	named = filepath.FromSlash(named)
	return emit.SamePath(named, store) || mainStore != "" && emit.SamePath(named, mainStore), nil
}

// mainCheckout returns the local settings file Claude Code reads before
// the project's own at path, at the root of the repository's main
// checkout, or "" when there is none. In a linked worktree it also
// returns the project's personal store in the main checkout, which a
// worktree Claude Code creates copies into its own settings.
func mainCheckout(path string) (settings, store string) {
	root, project, ok := emit.MainCheckout()
	if !ok {
		return "", ""
	}
	if wd, err := os.Getwd(); err == nil && !emit.SamePath(project, wd) {
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

// autoMemorySettings holds the auto memory keys of one Claude Code
// settings file. A nil field is unset.
type autoMemorySettings struct {
	enabled   *bool
	directory json.RawMessage
}

// readAutoMemorySettings reads the auto memory keys of the Claude Code
// settings file at path. A missing or unreadable file sets none.
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
	return s
}

// autoMemoryOn reports whether Claude Code runs auto memory with these
// settings, highest precedence first. CLAUDE_CODE_DISABLE_AUTO_MEMORY
// decides before any file: 0 forces it on, and any other value counts as
// off, so a value sync cannot read keeps the personal import.
func autoMemoryOn(scopes ...autoMemorySettings) bool {
	switch os.Getenv("CLAUDE_CODE_DISABLE_AUTO_MEMORY") {
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
