package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/chemaclass/agnostic-ai/internal/adapters/claudehooks"
)

// importClaudeHooks reads .claude/settings.json and writes one yaml per
// matcher group into dstDir. Filenames come from claudeHookNamer so the
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
	var s claudehooks.Settings
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

	pin := newClaudeHookPin(root)
	namer := newClaudeHookNamer(dstDir)
	count := 0
	for _, event := range events {
		for _, g := range s.Hooks[event] {
			cmds := make([]string, 0, len(g.Hooks))
			timeout := 0
			statusMessage, shell, ifRule := "", "", ""
			async, asyncRewake, once := false, false, false
			var args []string
			for _, h := range g.Hooks {
				if h.Type != "" && h.Type != "command" {
					n, err := importClaudeNonCommandHook(root, dstDir, event, g.Matcher, h, pin, namer)
					if err != nil {
						return count, err
					}
					count += n
					continue
				}
				if h.Command == "" || claudehooks.IsWorktreeSetupCommand(h.Command) || importLocal.dropsHookCommand("claude", event, g.Matcher, h.Command) {
					continue
				}
				cmds = append(cmds, h.Command)
				if len(h.Args) > 0 && args == nil {
					args = h.Args
				}
				if h.Timeout != 0 && timeout == 0 {
					timeout = h.Timeout
				}
				if h.StatusMessage != "" && statusMessage == "" {
					statusMessage = h.StatusMessage
				}
				if h.Shell != "" && shell == "" {
					shell = h.Shell
				}
				if h.If != "" && ifRule == "" {
					ifRule = h.If
				}
				async = async || h.Async
				asyncRewake = asyncRewake || h.AsyncRewake
				once = once || h.Once
			}
			if len(cmds) == 0 {
				continue
			}
			name := namer.name(event, g.Matcher, hookRunLabel(cmds[0]), cmds, map[string]any{"command": cmds})
			doc := map[string]any{
				"name":        name,
				"description": hookDescription(event, g.Matcher, cmds),
				"event":       event,
				"matcher":     g.Matcher,
			}
			if len(cmds) == 1 {
				doc["command"] = cmds[0]
			} else {
				doc["command"] = cmds
			}
			if len(args) > 0 {
				doc["args"] = args
			}
			if timeout != 0 {
				doc["timeout"] = timeout
			}
			if statusMessage != "" {
				doc["statusMessage"] = statusMessage
			}
			if async {
				doc["async"] = true
			}
			if asyncRewake {
				doc["asyncRewake"] = true
			}
			if once {
				doc["once"] = true
			}
			if shell != "" {
				doc["shell"] = shell
			}
			if ifRule != "" {
				doc["if"] = ifRule
			}
			pin.apply(doc, root, filepath.Join(dstDir, name+".yaml"))
			if err := writeHookSpecFile(dstDir, name, doc); err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}

// Non-command handlers need separate specs because each has a distinct
// payload. Command groups retain their existing command-list format.
func importClaudeNonCommandHook(root, dstDir, event, matcher string, h claudehooks.CommandEntry, pin claudeHookPin, namer *claudeHookNamer) (int, error) {
	var target string
	switch h.Type {
	case "http":
		if h.URL == "" {
			return 0, nil
		}
		target = h.URL
	case "mcp_tool":
		if h.Server == "" || h.Tool == "" {
			return 0, nil
		}
		target = h.Server + "/" + h.Tool
	case "prompt":
		if h.Prompt == "" {
			return 0, nil
		}
	default:
		return 0, nil
	}
	payload, err := json.Marshal(h)
	if err != nil {
		return 0, fmt.Errorf("marshal %s hook: %w", event, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(payload, &doc); err != nil {
		return 0, fmt.Errorf("parse %s hook: %w", event, err)
	}
	name := namer.name(event, matcher, hookHandlerLabel(h.Type, target), []string{string(payload)},
		map[string]any{"type": h.Type, "url": h.URL, "server": h.Server, "tool": h.Tool, "prompt": h.Prompt})
	doc["name"], doc["event"], doc["matcher"] = name, event, matcher
	doc["description"] = hookHandlerDescription(h.Type, target, event, matcher)
	pin.apply(doc, root, filepath.Join(dstDir, name+".yaml"))
	if err := writeHookSpecFile(dstDir, name, doc); err != nil {
		return 0, err
	}
	return 1, nil
}
