package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/gitquery"
)

// Sync writes this checkout's absolute path in its local settings.
const autoMemoryDirectoryKey = "autoMemoryDirectory"

func emitAutoMemoryDirectory(sess *emit.Session, dir string, cfg *config.Config, dryRun bool) error {
	path := filepath.Join(dir, "settings.local.json")
	if preserved, err := sess.PreserveHandoffConfig(cfg, path, target); preserved || err != nil {
		return err
	}
	keys := map[string]any{}
	handoff, err := sess.HandoffAccessRoot(cfg, path, target, "permissions.additionalDirectories", dryRun)
	if err != nil {
		return err
	}
	if sess.IsUnmanaged(path) {
		return nil
	}
	existing, _ := sess.ExistingJSONObject(path, "permissions", dryRun)["additionalDirectories"].([]any)
	var planned []any
	if handoff != "" {
		planned = emit.WithoutUserItems(path, []string{"permissions", "additionalDirectories"}, existing, []any{handoff})
	}
	if handoff != "" || len(emit.PriorClaimedItems(path, []string{"permissions", "additionalDirectories"})) > 0 {
		merged, claims := emit.MergeOwnedLists(path, []string{"permissions"}, map[string][]any{"additionalDirectories": existing}, map[string][]any{"additionalDirectories": planned})
		list := merged["additionalDirectories"]
		value := emit.CarriedJSONValue(list)
		if len(claims["additionalDirectories"]) > 0 {
			value = emit.ClaimedJSONItems(list, claims["additionalDirectories"])
		} else if len(list) == 0 {
			value = emit.RemoveJSONKey
		}
		keys["permissions"] = map[string]any{"additionalDirectories": value}
	}
	flush := func() error {
		// A file that would only lose the grant is not written, so the
		// release of the earlier output takes out only what sync added.
		if _, grantOnly := keys["permissions"]; len(keys) == 0 || handoff == "" && grantOnly && len(keys) == 1 {
			return nil
		}
		return sess.MergeJSONFileNested(path, keys, []string{"permissions"}, dryRun)
	}
	if !slices.Contains(cfg.Builtins, emit.MemoryBuiltin) {
		noteDroppedAutoMemory(path)
		return flush()
	}
	owned := emit.ClaimsKey(path, autoMemoryDirectoryKey)
	if sess.HasJSONKey(path, autoMemoryDirectoryKey, dryRun) && !owned {
		return flush()
	}
	store, err := emit.PersonalMemoryDir(cfg, ".")
	if err != nil {
		return err
	}
	if err := sess.CreateRepoMemoryStore(cfg, store, dryRun); err != nil {
		return err
	}
	if store, err = filepath.Abs(store); err != nil {
		return fmt.Errorf("resolve %s: %w", emit.PersonalMemoryIndexPath, err)
	}
	// The file holds this checkout's absolute path, so it must never be
	// committed, even when sync creates it before Claude Code ignores it.
	if !dryRun && !sess.IsCapturing() {
		if err := emit.ExcludeFromGit(AutoMemoryGitExclusion(dir)); err != nil {
			return err
		}
		// Linked worktrees share native memory until the main checkout redirects it.
		if !owned && !mainWorktreeRedirectsAutoMemory(".", dir) {
			if native, err := (Adapter{}).NativeMemoryDir("."); err == nil {
				emit.RequestMemorySeed(emit.MemorySeed{Target: target, From: native, To: store})
			}
		}
	}
	keys[autoMemoryDirectoryKey] = filepath.ToSlash(store)
	return flush()
}

// AutoMemoryGitExclusion is the info/exclude pattern that keeps the local
// settings in dir out of Git.
func AutoMemoryGitExclusion(dir string) string {
	return "**/" + filepath.ToSlash(filepath.Join(dir, "settings.local.json"))
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
	if json.Unmarshal(value, &named) != nil {
		return "", nil
	}
	home, _ := os.UserHomeDir()
	named = autoMemoryDirectoryPath(named, home)
	if named == "" {
		return "", nil
	}
	absStore, err := filepath.Abs(store)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", emit.PersonalMemoryIndexPath, err)
	}
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
	// Relative to parent, not to the Git top level: Git spells that by its
	// resolved path, which differs from path under a link or a Windows
	// short name such as RUNNER~1. Outside a work tree Git fails to answer.
	relative, err := filepath.Rel(parent, path)
	if err != nil {
		return true
	}
	tracked, err := gitquery.Tracked(parent, filepath.ToSlash(relative))
	return err != nil || tracked
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

func autoMemoryDirectoryPath(dir, home string) string {
	if rest, ok := strings.CutPrefix(dir, "~/"); ok {
		if home == "" {
			return ""
		}
		return filepath.Join(home, filepath.FromSlash(rest))
	}
	if filepath.IsAbs(dir) {
		return filepath.Clean(dir)
	}
	return ""
}

func mainWorktreeRedirectsAutoMemory(root, dir string) bool {
	if !inLinkedWorktree(root) {
		return false
	}
	main := memoryProjectRoot(root)
	native, err := (Adapter{}).NativeMemoryDir(root)
	if err != nil {
		return false
	}
	home, _ := os.UserHomeDir()
	for _, name := range []string{"settings.local.json", "settings.json"} {
		settingsDir := dir
		if !filepath.IsAbs(settingsDir) {
			settingsDir = filepath.Join(main, settingsDir)
		}
		path := filepath.Join(settingsDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var doc map[string]any
		data, _ = emit.StripJSONC(data)
		if json.Unmarshal(data, &doc) != nil {
			continue
		}
		store, _ := doc[autoMemoryDirectoryKey].(string)
		if store = autoMemoryDirectoryPath(store, home); store != "" {
			return canonicalPath(store) != canonicalPath(native)
		}
	}
	return false
}

// noteDroppedAutoMemory names, on the sync that drops the key with the
// memory built-in, the facts Claude Code stops reading from the store the
// key named. A later sync no longer claims the key and stays quiet.
func noteDroppedAutoMemory(path string) {
	if !emit.ClaimsKey(path, autoMemoryDirectoryKey) {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var doc map[string]any
	if json.Unmarshal(data, &doc) != nil {
		return
	}
	store, _ := doc[autoMemoryDirectoryKey].(string)
	facts, err := emit.MemoryFactFiles(filepath.FromSlash(store))
	if store == "" || err != nil || len(facts) == 0 {
		return
	}
	emit.NoteProject(fmt.Sprintf("claude: with the memory built-in off, Claude Code no longer reads the %d fact(s) in %s; add memory to builtins to load them again", len(facts), store))
}
