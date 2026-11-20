package approvals

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// RuleToolSettings names the ask when a tool's own settings file, whose
// ask rules a hook allow would skip, does not read, or when a project
// settings file moves the folders the hook reads its policy from.
const RuleToolSettings = "tool-settings"

// ClaudeManagedDir is the folder of Claude Code's managed settings files
// on goos (code.claude.com/docs/en/managed-settings.md), or "" where the
// hook does not run.
func ClaudeManagedDir(goos string) string {
	switch goos {
	case "darwin":
		return "/Library/Application Support/ClaudeCode"
	case "linux":
		return "/etc/claude-code"
	}
	return ""
}

// ClaudeSettingsFile is one settings file, or a folder of them, that
// Claude Code may read. Project marks a file inside a checkout, which a
// repository can ship.
type ClaudeSettingsFile struct {
	Path    string
	Project bool
}

// ClaudeSettingsFiles lists the settings files whose ask rules may apply
// to a session in cwd: the user file, the managed file and its drop-in
// folder, and .claude/settings.json and settings.local.json in cwd, in
// projectDir, in every folder above them, and at the main checkout of a
// linked worktree, each walked both as written and with links resolved.
// Claude Code reads fewer of these; reading more only adds rules that ask.
func ClaudeSettingsFiles(cwd, projectDir, userSettings, managedDir string) []ClaudeSettingsFile {
	var files []ClaudeSettingsFile
	add := func(path string, project bool) {
		if path != "" && !slices.ContainsFunc(files, func(f ClaudeSettingsFile) bool { return f.Path == path }) {
			files = append(files, ClaudeSettingsFile{Path: path, Project: project})
		}
	}
	addProject := func(dir string) {
		add(filepath.Join(dir, ".claude", "settings.json"), true)
		add(filepath.Join(dir, ".claude", "settings.local.json"), true)
	}
	add(userSettings, false)
	if managedDir != "" {
		add(filepath.Join(managedDir, "managed-settings.json"), false)
		add(filepath.Join(managedDir, "managed-settings.d"), false)
	}
	var starts []string
	for _, start := range []string{cwd, projectDir} {
		if filepath.IsAbs(start) {
			starts = append(starts, filepath.Clean(start), ResolveLinks(start))
		}
	}
	for _, start := range starts {
		for dir := start; ; dir = filepath.Dir(dir) {
			addProject(dir)
			if main := mainCheckout(dir); main != "" {
				addProject(main)
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
	return files
}

// mainCheckout is the main checkout of the linked worktree at dir, or ""
// when dir is not the root of one.
func mainCheckout(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, ".git"))
	if err != nil {
		return ""
	}
	gitdir, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
	if !ok {
		return ""
	}
	gitdir = strings.TrimSpace(gitdir)
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(dir, gitdir)
	}
	common, err := os.ReadFile(filepath.Join(gitdir, "commondir"))
	if err != nil {
		return ""
	}
	commonDir := strings.TrimSpace(string(common))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(gitdir, commonDir)
	}
	commonDir = filepath.Clean(commonDir)
	if filepath.Base(commonDir) != ".git" {
		return ""
	}
	return filepath.Dir(commonDir)
}

// ClaudeSettings is what the hook needs from Claude Code's settings.
type ClaudeSettings struct {
	// Ask are the permissions.ask rules of every file.
	Ask []string
	// RuleRoots are the folders that hold a project file with ask
	// rules; a rule's path may be anchored there.
	RuleRoots []string
	// MovedEnv are the variables a project file's env block sets that
	// change where the hook reads its policy or the person's settings.
	MovedEnv []string
}

// policyEnv are the variables that choose the files the hook reads. A
// repository's settings file must not point them elsewhere.
var policyEnv = []string{"AGNOSTIC_AI_HOME", "HOME", "CLAUDE_CONFIG_DIR", "USERPROFILE"}

// ReadClaudeSettings reads each file in files. A path that is a folder
// adds every .json file in it. A missing file adds nothing; one that
// exists but does not read or parse is an error, so the caller asks
// rather than skip a rule it cannot see.
func ReadClaudeSettings(files []ClaudeSettingsFile) (ClaudeSettings, error) {
	var out ClaudeSettings
	for _, file := range files {
		info, err := os.Stat(file.Path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return ClaudeSettings{}, fmt.Errorf("read %s: %w", file.Path, err)
		}
		paths := []string{file.Path}
		if info.IsDir() {
			if paths, err = jsonFiles(file.Path); err != nil {
				return ClaudeSettings{}, err
			}
		}
		for _, path := range paths {
			settings, err := readClaudeSettingsFile(path)
			if err != nil {
				return ClaudeSettings{}, err
			}
			out.Ask = append(out.Ask, settings.Permissions.Ask...)
			if !file.Project {
				continue
			}
			if root := filepath.Dir(filepath.Dir(path)); len(settings.Permissions.Ask) > 0 && !slices.Contains(out.RuleRoots, root) {
				out.RuleRoots = append(out.RuleRoots, root)
			}
			for _, key := range policyEnv {
				if _, set := settings.Env[key]; set && !slices.Contains(out.MovedEnv, key) {
					out.MovedEnv = append(out.MovedEnv, key)
				}
			}
		}
	}
	return out, nil
}

func jsonFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") && !strings.HasPrefix(entry.Name(), ".") {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}

type claudeSettingsFile struct {
	Permissions struct {
		Ask []string `json:"ask"`
	} `json:"permissions"`
	Env map[string]any `json:"env"`
}

func readClaudeSettingsFile(path string) (claudeSettingsFile, error) {
	var settings claudeSettingsFile
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return settings, nil
	}
	if err != nil {
		return settings, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return settings, fmt.Errorf("parse %s: %w", path, err)
	}
	return settings, nil
}
