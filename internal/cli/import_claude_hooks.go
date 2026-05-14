package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

type claudeHookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

type claudeHookGroup struct {
	Matcher string              `json:"matcher"`
	Hooks   []claudeHookCommand `json:"hooks"`
}

type claudeSettings struct {
	Hooks map[string][]claudeHookGroup `json:"hooks"`
}

// importClaudeHooks reads .claude/settings.json and writes one yaml per
// matcher group into dstDir. Filenames come from hookSpecName so the
// same hook always lands at the same path across re-imports.
//
// A matcher block with multiple inner commands renders as a single yaml
// whose `command:` field is a list. The emit side merges the same
// event+matcher specs back into one matcher block, so the round-trip
// preserves multi-command groups without exploding into N files.
func importClaudeHooks(root, dstDir string) (int, error) {
	src := filepath.Join(root, claudeDir, "settings.json")
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	var s claudeSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return 0, fmt.Errorf("parse %s: %w", src, err)
	}
	if len(s.Hooks) == 0 {
		return 0, nil
	}
	events := make([]string, 0, len(s.Hooks))
	for e := range s.Hooks {
		events = append(events, e)
	}
	sort.Strings(events)

	count := 0
	for _, event := range events {
		for _, g := range s.Hooks[event] {
			cmds := make([]string, 0, len(g.Hooks))
			for _, h := range g.Hooks {
				if h.Command != "" {
					cmds = append(cmds, h.Command)
				}
			}
			if len(cmds) == 0 {
				continue
			}
			name := hookSpecName(event, g.Matcher, cmds)
			doc := map[string]any{
				"name":    name,
				"event":   event,
				"matcher": g.Matcher,
			}
			if len(cmds) == 1 {
				doc["command"] = cmds[0]
			} else {
				doc["command"] = cmds
			}
			raw, err := yaml.Marshal(doc)
			if err != nil {
				return count, fmt.Errorf("marshal hook %s: %w", name, err)
			}
			path := filepath.Join(dstDir, name+".yaml")
			if err := os.WriteFile(path, raw, 0o644); err != nil {
				return count, fmt.Errorf("write %s: %w", path, err)
			}
			count++
		}
	}
	return count, nil
}
