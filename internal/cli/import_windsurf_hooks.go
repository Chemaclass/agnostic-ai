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

// windsurfHookGroup mirrors one `{matcher, hooks}` object in a
// `.devin/hooks.v1.json` event array. A local struct rather than
// claudehooks.Group: Devin's own hook entries carry `prompt` in place
// of `command` for a `type: prompt` hook, a field claudehooks.
// CommandEntry has no slot for.
type windsurfHookGroup struct {
	Matcher string              `json:"matcher"`
	Hooks   []windsurfHookEntry `json:"hooks"`
}

type windsurfHookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Prompt  string `json:"prompt"`
	Timeout int    `json:"timeout"`
}

// importWindsurfHooks reads `.devin/hooks.v1.json` and writes one yaml
// per matcher group into dstDir. Unlike Claude Code, Codex, and
// OpenHands, the file carries no `"hooks"` wrapper key: "the hooks
// object is the entire file" (docs.devin.ai/cli/extensibility/hooks/
// overview), so this decodes straight into a `map[string][]
// windsurfHookGroup` rather than reusing claudehooks.Settings. A
// `type: prompt` entry (no `command`) imports with `type: prompt` and
// `prompt: <text>` on the spec instead of `command:`, since
// agnostic-ai's generic hook spec has no dedicated prompt field.
// No-op when the file is absent.
func importWindsurfHooks(root, dstDir string) (int, error) {
	src := filepath.Join(root, windsurfHooksFile)
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	var doc map[string][]windsurfHookGroup
	if err := json.Unmarshal(data, &doc); err != nil {
		return 0, fmt.Errorf("parse %s: %w", src, err)
	}
	if len(doc) == 0 {
		return 0, nil
	}
	events := make([]string, 0, len(doc))
	for e := range doc {
		events = append(events, e)
	}
	sort.Strings(events)

	count := 0
	for _, event := range events {
		for _, g := range doc[event] {
			n, err := writeWindsurfHookGroup(dstDir, event, g)
			if err != nil {
				return count, err
			}
			count += n
		}
	}
	return count, nil
}

// writeWindsurfHookGroup writes one spec per distinct type inside a
// matcher group: every command-type entry collapses into a single
// `command:` list spec (same collapsing rule claude/codex apply), and
// every prompt-type entry writes its own spec, since two prompts
// sharing a matcher are not one logical hook.
func writeWindsurfHookGroup(dstDir, event string, g windsurfHookGroup) (int, error) {
	count := 0
	var commands []string
	var timeout int
	for _, h := range g.Hooks {
		switch h.Type {
		case "prompt":
			if h.Prompt == "" {
				continue
			}
			name := hookSpecName(event, g.Matcher, []string{"prompt:" + h.Prompt})
			docMap := map[string]any{
				"name":    name,
				"event":   event,
				"matcher": g.Matcher,
				"type":    "prompt",
				"prompt":  h.Prompt,
				"target":  "windsurf",
			}
			if h.Timeout != 0 {
				docMap["timeout"] = h.Timeout
			}
			if err := writeWindsurfHookSpec(dstDir, name, docMap); err != nil {
				return count, err
			}
			count++
		default:
			if h.Command == "" {
				continue
			}
			commands = append(commands, h.Command)
			if h.Timeout != 0 && timeout == 0 {
				timeout = h.Timeout
			}
		}
	}
	if len(commands) == 0 {
		return count, nil
	}
	name := hookSpecName(event, g.Matcher, commands)
	docMap := map[string]any{
		"name":    name,
		"event":   event,
		"matcher": g.Matcher,
		"target":  "windsurf",
	}
	if len(commands) == 1 {
		docMap["command"] = commands[0]
	} else {
		docMap["command"] = commands
	}
	if timeout != 0 {
		docMap["timeout"] = timeout
	}
	if err := writeWindsurfHookSpec(dstDir, name, docMap); err != nil {
		return count, err
	}
	return count + 1, nil
}

func writeWindsurfHookSpec(dstDir, name string, docMap map[string]any) error {
	raw, err := yaml.Marshal(docMap)
	if err != nil {
		return fmt.Errorf("marshal hook %s: %w", name, err)
	}
	path := filepath.Join(dstDir, name+".yaml")
	if err := importWriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
