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
// ask rules a hook allow would skip, does not read.
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

// ClaudeSettingsFiles lists the settings files whose ask rules may apply
// to a session in cwd: the user file, the managed file and its drop-in
// folder, and .claude/settings.json and settings.local.json in cwd, in
// projectDir, in every folder above them, and at the main checkout of a
// linked worktree. Claude Code reads fewer of these; reading more only
// adds rules that ask.
func ClaudeSettingsFiles(cwd, projectDir, userSettings, managedDir string) []string {
	var files []string
	add := func(path string) {
		if path != "" && !slices.Contains(files, path) {
			files = append(files, path)
		}
	}
	add(userSettings)
	if managedDir != "" {
		add(filepath.Join(managedDir, "managed-settings.json"))
		add(filepath.Join(managedDir, "managed-settings.d"))
	}
	for _, start := range []string{cwd, projectDir} {
		if !filepath.IsAbs(start) {
			continue
		}
		for dir := filepath.Clean(start); ; dir = filepath.Dir(dir) {
			add(filepath.Join(dir, ".claude", "settings.json"))
			add(filepath.Join(dir, ".claude", "settings.local.json"))
			if main := mainCheckout(dir); main != "" {
				add(filepath.Join(main, ".claude", "settings.json"))
				add(filepath.Join(main, ".claude", "settings.local.json"))
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

// ClaudeAskRules returns permissions.ask from each settings file in
// paths. A path that is a folder adds every .json file in it. A missing
// file adds nothing; one that exists but does not read or parse is an
// error, so the caller asks rather than skip a rule it cannot see.
func ClaudeAskRules(paths []string) ([]string, error) {
	var rules []string
	for _, path := range paths {
		info, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		files := []string{path}
		if info.IsDir() {
			if files, err = jsonFiles(path); err != nil {
				return nil, err
			}
		}
		for _, file := range files {
			ask, err := claudeAskRules(file)
			if err != nil {
				return nil, err
			}
			rules = append(rules, ask...)
		}
	}
	return rules, nil
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

func claudeAskRules(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var settings struct {
		Permissions *struct {
			Ask []string `json:"ask"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if settings.Permissions == nil {
		return nil, nil
	}
	return settings.Permissions.Ask, nil
}
