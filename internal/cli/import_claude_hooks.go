package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
// hook command into <dstDir>/<event>-<group>-<index>.yaml.
func importClaudeHooks(root, dstDir string) (int, error) {
	src := filepath.Join(root, ".claude", "settings.json")
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
		for gi, g := range s.Hooks[event] {
			for hi, h := range g.Hooks {
				name := fmt.Sprintf("%s-%d-%d", strings.ToLower(event), gi+1, hi+1)
				doc := map[string]any{
					"name":    name,
					"event":   event,
					"matcher": g.Matcher,
					"command": h.Command,
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
	}
	return count, nil
}
