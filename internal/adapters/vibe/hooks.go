package vibe

import (
	"errors"
	"fmt"
	"maps"
	"runtime"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

var hookAllowsStrict = map[string]bool{
	"pre_tool":      true,
	"post_tool":     true,
	"pre_agent":     true,
	"post_agent":    false,
	"notification":  false,
	"session_start": false,
	"session_end":   false,
	"pre_llm":       true,
	"post_llm":      true,
}

func HookEvents() []string {
	return slices.Sorted(maps.Keys(hookAllowsStrict))
}

func (Adapter) UserHookRecords(hooks []spec.Entry, scriptsDir string) ([]map[string]any, error) {
	return (Adapter{}).UserHookRecordsForPlatform(hooks, scriptsDir, runtime.GOOS)
}
func (Adapter) UserHookRecordsForPlatform(hooks []spec.Entry, scriptsDir, platform string) ([]map[string]any, error) {
	var out []map[string]any
	seen := map[string]bool{}
	for _, h := range hooks {
		m := emit.ResolveMeta(h.Meta, target)
		if _, ok := m["env"]; ok {
			return nil, fmt.Errorf("vibe: hook %s has no native env field; set variables in its command", h.Name)
		}
		for key := range native(h.Meta) {
			switch key {
			case "name", "type", "command", "match", "timeout", "strict", "description", "event", "matcher", "args", "commandWindows":
			default:
				return nil, fmt.Errorf("vibe: hook %s native field %q is not supported", h.Name, key)
			}
		}
		event, _ := m["event"].(string)
		if event == "" {
			event, _ = m["type"].(string)
		}
		allowsStrict, supported := hookAllowsStrict[event]
		if !supported {
			emit.NoteFieldNoOp(target, spec.KindHook, "event", 1, "use native events ("+strings.Join(HookEvents(), ", ")+"). Portable exit decisions differ")
			continue
		}
		if seen[h.Name] {
			return nil, fmt.Errorf("vibe: duplicate hook name %q", h.Name)
		}
		seen[h.Name] = true
		command, _ := m["command"].(string)
		if command == "" {
			return nil, fmt.Errorf("vibe: hook %s requires one shell command", h.Name)
		}
		args := stringsList(m["args"])
		if platform == "windows" {
			if win, ok := m["commandWindows"].(string); ok {
				command = win
			}
		}
		command = emit.RewriteUserHookPath(command, target, scriptsDir, m)
		command, err := nativeHookCommand(command, args, platform)
		if err != nil {
			return nil, fmt.Errorf("vibe: hook %s: %w", h.Name, err)
		}
		d := map[string]any{"name": h.Name, "type": event, "command": command}
		if matcher, ok := m["matcher"]; ok {
			d["match"] = matcher
		}
		for _, k := range []string{"timeout", "strict", "description", "match"} {
			if v, ok := m[k]; ok {
				d[k] = v
			}
		}
		maps.Copy(d, native(h.Meta))
		d["name"] = h.Name
		d["type"] = event
		d["command"] = command
		if !allowsStrict && d["strict"] == true {
			return nil, fmt.Errorf("vibe: %s hook %s cannot set strict", event, h.Name)
		}
		if strings.TrimSpace(h.Name) == "" {
			return nil, errors.New("vibe: hooks require a name")
		}
		out = append(out, d)
	}
	return out, nil
}
