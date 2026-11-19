package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
)

type globalState struct {
	TOMLArrays map[string]map[string]tomlArrayLayout `json:"tomlArrays,omitempty"`
	HookPaths  map[string]string                     `json:"hookPaths,omitempty"`
	Version    int                                   `json:"version"`
	Files      []string                              `json:"files"`
	Agents     map[string][]string                   `json:"agents,omitempty"`
	// Skills records the files each target placed under its skills
	// directory. Several targets share ~/.agents/skills/, so a sync of
	// one must keep what the others placed there.
	Skills map[string][]string `json:"skills,omitempty"`
	// HookFiles tracks standalone definitions and scripts across configuration root moves.
	HookFiles map[string][]string `json:"hookFiles,omitempty"`
	// Sums maps each owned file to the sum of what sync last wrote there,
	// covering only the managed block of an instructions file.
	Sums map[string]string `json:"sums,omitempty"`
	// AgentEfforts records the effortLevel written per target and agent
	// name, so a later sync removes only values it placed.
	AgentEfforts map[string]map[string]string `json:"agentEfforts,omitempty"`
	// Settings records the value written per target and native key of
	// its user settings file, so a later sync changes or removes only
	// keys it placed.
	Settings map[string]map[string]any `json:"settings,omitempty"`
	// PermissionRules records, per target and permission list, the rules
	// sync wrote into its user settings file, so a later sync removes
	// only its own and import leaves them out.
	PermissionRules map[string]map[string][]string `json:"permissionRules,omitempty"`
	// SettingsOriginals records, per target and native key, what a key a
	// personal portable field writes held before, so removal restores it.
	SettingsOriginals map[string]map[string]globalSettingOriginal `json:"settingsOriginals,omitempty"`
	// SessionRetention is the version 9 layout of SettingsOriginals, read
	// for migration only.
	SessionRetention map[string]legacyRetentionOriginal `json:"sessionRetention,omitempty"`
	// SettingsParents records, per target, the JSON objects sync created
	// as settings or above the settings keys it owns. Removal takes away only these
	// once empty, so an object the user wrote stays.
	SettingsParents map[string][]string `json:"settingsParents,omitempty"`
	// MCP records each MCP server written per target, by its key in the
	// target's user MCP file, so a later sync removes only its own.
	MCP map[string]map[string]any `json:"mcp,omitempty"`
	// SettingsPaths and MCPPaths record the file each target's owned keys
	// live in, so a moved configuration root sweeps the old file.
	SettingsPaths map[string]string `json:"settingsPaths,omitempty"`
	MCPPaths      map[string]string `json:"mcpPaths,omitempty"`
	// Created lists the user files sync created, which it removes once
	// nothing is left in them. They also sit in Files, but a moved
	// configuration root is expected for them, so they are not proof of
	// another HOME.
	Created []string `json:"created,omitempty"`
	// removalGuards is each created file this run removes, as the plan
	// read it, so a write another program made since stops the removal.
	removalGuards map[string]*diskSnapshot
	// Hooks records the managed hook entries per target, keyed by
	// target name then event, so a later sync can remove exactly what
	// it added and leave user-authored entries alone.
	Hooks map[string]map[string][]any `json:"hooks,omitempty"`
	// HookContainers records per target what of its hooks file sync
	// created: "<file>", "hooks", "hooks.<event>", "env", "version", or
	// "<key>=null" for a null it filled. Sync removes only these once
	// they are empty, so a container the user wrote stays.
	HookContainers map[string][]string `json:"hookContainers,omitempty"`
	// legacyHookContainers marks state from before HookContainers, when
	// sync treated every empty container in a hooks file as its own.
	legacyHookContainers bool
	// ClaudeHooks and CursorHooks are the version-1 layout, read for
	// migration only.
	ClaudeHooks map[string][]any `json:"claudeHooks,omitempty"`
	CursorHooks map[string][]any `json:"cursorHooks,omitempty"`
}

func globalStateMissing(path string) bool {
	_, err := os.Lstat(path)
	return errors.Is(err, fs.ErrNotExist)
}

// globalStateCurrent reports whether the recorded state already
// describes next. It compares normalized content, so an older state
// version holding the same ownership is not drift. Sums are left out:
// they mirror file content, which check compares directly.
func globalStateCurrent(path string, next globalState) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	recorded, err := loadGlobalState(path)
	if err != nil {
		return false
	}
	recorded.Sums, next.Sums = nil, nil
	// An empty list and a missing one record the same ownership.
	if len(recorded.Files) == 0 {
		recorded.Files = []string{}
	}
	if len(next.Files) == 0 {
		next.Files = []string{}
	}
	a, errA := json.Marshal(recorded)
	b, errB := json.Marshal(next)
	return errA == nil && errB == nil && bytes.Equal(a, b)
}

func loadGlobalState(path string) (globalState, error) {
	return loadGlobalStateUpTo(path, globalStateVersion)
}

// loadGlobalStateUpTo reads a state file no newer than version, the way
// the release that wrote version reads one.
func loadGlobalStateUpTo(path string, version int) (globalState, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return globalState{Version: globalStateVersion}, nil
	}
	if err != nil {
		return globalState{}, fmt.Errorf("read %s: %w", path, err)
	}
	var state globalState
	if err := json.Unmarshal(data, &state); err != nil || state.Version < 1 || state.Version > version {
		return globalState{}, fmt.Errorf("parse %s: corrupt global ownership state", path)
	}
	if len(state.MCP["pi"]) > 0 {
		ordered, err := piMCPMap(path, data, "mcp", "pi")
		if err != nil {
			return globalState{}, err
		}
		state.MCP["pi"] = ordered
	}
	if state.Hooks == nil {
		state.Hooks = map[string]map[string][]any{}
	}
	for target, hooks := range map[string]map[string][]any{"claude": state.ClaudeHooks, "cursor": state.CursorHooks} {
		if len(hooks) > 0 && state.Hooks[target] == nil {
			state.Hooks[target] = hooks
		}
	}
	state.ClaudeHooks, state.CursorHooks = nil, nil
	for target, original := range state.SessionRetention {
		if state.SettingsOriginals == nil {
			state.SettingsOriginals = map[string]map[string]globalSettingOriginal{}
		}
		if state.SettingsOriginals[target] == nil {
			state.SettingsOriginals[target] = map[string]globalSettingOriginal{}
		}
		state.SettingsOriginals[target][original.Key] = original.globalSettingOriginal
	}
	state.SessionRetention = nil
	state.legacyHookContainers = state.Version < 6
	state.Version = globalStateVersion
	return state, nil
}

// foreignGlobalPath returns a recorded path outside every root this run
// resolves, which means the state belongs to another HOME. Ownership is
// by absolute path, so syncing on would sweep that home's files.
func foreignGlobalPath(old globalState, home string) string {
	roots := globalRoots(home)
	for _, path := range append(slices.Clone(old.Files), old.Created...) {
		if !filepath.IsAbs(path) {
			return path
		}
	}
	for _, paths := range []map[string]string{old.HookPaths, old.SettingsPaths, old.MCPPaths} {
		for _, target := range slices.Sorted(maps.Keys(paths)) {
			if path := paths[target]; path != "" && !filepath.IsAbs(path) {
				return path
			}
		}
	}
	for _, target := range slices.Sorted(maps.Keys(old.HookFiles)) {
		for _, path := range old.HookFiles[target] {
			if !filepath.IsAbs(path) {
				return path
			}
		}
	}
	paths := slices.DeleteFunc(slices.Clone(old.Files), func(p string) bool { return slices.Contains(old.Created, p) })
	for _, owned := range []map[string][]string{old.Agents, old.Skills} {
		for _, target := range slices.Sorted(maps.Keys(owned)) {
			paths = append(paths, owned[target]...)
		}
	}
	for _, path := range paths {
		if !slices.ContainsFunc(roots, func(root string) bool { return path == root || inManagedTree(path, []string{root}) }) {
			return path
		}
	}
	return ""
}
