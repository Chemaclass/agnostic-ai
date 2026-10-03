package adapters

import (
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/adapters/antigravity"
	"github.com/chemaclass/agnostic-ai/internal/adapters/augment"
	"github.com/chemaclass/agnostic-ai/internal/adapters/claude"
	"github.com/chemaclass/agnostic-ai/internal/adapters/cline"
	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
	"github.com/chemaclass/agnostic-ai/internal/adapters/copilot"
	"github.com/chemaclass/agnostic-ai/internal/adapters/crush"
	"github.com/chemaclass/agnostic-ai/internal/adapters/cursor"
	"github.com/chemaclass/agnostic-ai/internal/adapters/factory"
	"github.com/chemaclass/agnostic-ai/internal/adapters/gemini"
	"github.com/chemaclass/agnostic-ai/internal/adapters/goose"
	"github.com/chemaclass/agnostic-ai/internal/adapters/openhands"
	"github.com/chemaclass/agnostic-ai/internal/adapters/qoder"
	"github.com/chemaclass/agnostic-ai/internal/adapters/trae"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/hookrun"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// HookHandlers returns the command handlers sync writes for h on target,
// for the targets hookrun builds payloads for.
func HookHandlers(cfg *config.Config, target string, h spec.Entry) ([]hookrun.Handler, error) {
	var out []hookrun.Handler
	switch target {
	case "claude":
		for _, c := range claude.CommandHandlers(h) {
			out = append(out, hookrun.Handler{Command: c.Command, Args: c.Args, Shell: c.Shell, If: c.If, Timeout: time.Duration(c.Timeout) * time.Second})
		}
	case "codex":
		for _, c := range codex.HookCommands(h) {
			out = append(out, hookrun.Handler{Command: c.Command, CommandWindows: c.CommandWindows, Timeout: time.Duration(c.Timeout) * time.Second})
		}
	case "gemini":
		for _, c := range gemini.HookCommands(h) {
			out = append(out, hookrun.Handler{Command: c.Command, Env: c.Env, Timeout: c.Timeout})
		}
	case "cursor":
		doc, err := cursor.HookDoc(h)
		if err != nil {
			return nil, err
		}
		event, _ := h.Meta["event"].(string)
		handlers, err := hookrun.CursorHandlers(doc, event, cursor.HookTargetCommand)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", h.Path, err)
		}
		return handlers, nil
	case "crush":
		doc, err := crush.HookDoc(h)
		if err != nil {
			return nil, err
		}
		handlers, err := hookrun.CrushHandlers(doc)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", h.Path, err)
		}
		return handlers, nil
	case "copilot":
		doc, err := copilot.HookDoc(h)
		if err != nil {
			return nil, err
		}
		event, _ := h.Meta["event"].(string)
		handlers, err := hookrun.CopilotHandlers(doc, event)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", h.Path, err)
		}
		return handlers, nil
	case "qoder":
		doc, err := qoder.HookDoc(h)
		if err != nil {
			return nil, err
		}
		event, _ := h.Meta["event"].(string)
		handlers, err := hookrun.QoderHandlers(doc, event)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", h.Path, err)
		}
		return handlers, nil
	case "cline":
		// The script sync would write for h alone; the synced one joins
		// every spec on the event.
		event, _ := h.Meta["event"].(string)
		path, script := cline.HookScriptPath(cfg, event), cline.HookScript(h)
		if path == "" || script == "" {
			return nil, nil
		}
		return []hookrun.Handler{{Command: filepath.ToSlash(path), Script: script}}, nil
	default:
		doc, err := hookDoc(cfg, target, h)
		if err != nil {
			return nil, err
		}
		return hookrun.HandlersFromDoc(target, doc)
	}
	return out, nil
}

// hookDoc renders the hooks file sync writes for h alone on a target
// whose file shares the `hooks` block shape, or Factory's, which keys
// the same groups by event at the top level, or Antigravity's, which
// keys them by hook definition name.
func hookDoc(cfg *config.Config, target string, h spec.Entry) ([]byte, error) {
	switch target {
	case "trae":
		return trae.HookDoc(h)
	case "openhands":
		return openhands.HookDoc(h)
	case "goose":
		return goose.HookDoc(cfg, h)
	case "augment":
		return augment.HookDoc(h)
	case "factory":
		return factory.HookDoc(h)
	case "antigravity":
		return antigravity.HookDoc(h)
	}
	return nil, nil
}

// HookScriptSiblings names, sorted, the other specs in hooks that sync
// writes into the same script as h on target: Cline joins every spec on
// an event in one script, with one stdout.
func HookScriptSiblings(cfg *config.Config, target string, hooks []spec.Entry, h spec.Entry) []string {
	if target != "cline" {
		return nil
	}
	event, _ := h.Meta["event"].(string)
	path := cline.HookScriptPath(cfg, event)
	if path == "" {
		return nil
	}
	var names []string
	for _, other := range hooks {
		otherEvent, _ := other.Meta["event"].(string)
		if (other.Name == h.Name && other.Path == h.Path) || !other.EmitsTo(target) ||
			cline.HookScriptPath(cfg, otherEvent) != path || cline.HookScript(other) == "" {
			continue
		}
		names = append(names, other.Name)
	}
	slices.Sort(names)
	return names
}

// HookFile is the native file sync writes target's hooks on event to,
// for the targets hookrun builds payloads for.
func HookFile(cfg *config.Config, target, event string) string {
	switch target {
	case "claude":
		return claude.SettingsFilePath(cfg)
	case "codex":
		return codex.HooksFilePath(cfg)
	case "gemini":
		return gemini.SettingsFilePath(cfg)
	case "trae":
		return trae.HooksFilePath(cfg)
	case "openhands":
		return openhands.HooksFilePath(cfg)
	case "goose":
		return goose.HooksFilePath(cfg)
	case "augment":
		return augment.SettingsFilePath(cfg)
	case "cursor":
		return cursor.HooksFilePath(cfg)
	case "crush":
		return crush.HooksFilePath(cfg)
	case "factory":
		return factory.HooksFilePath(cfg)
	case "copilot":
		return copilot.HooksFilePath(cfg)
	case "qoder":
		return qoder.SettingsFilePath(cfg)
	case "antigravity":
		return antigravity.HooksFilePath(cfg)
	case "cline":
		return cline.HookScriptPath(cfg, event)
	}
	return ""
}

// HookNativeMatcher is the matcher sync writes for a spec's matcher on
// event: Augment and Antigravity drop it on the events that take none,
// and Cline, whose event scripts have no matcher, on every event.
func HookNativeMatcher(target, event, matcher string) string {
	if target == "cline" {
		return ""
	}
	if target == "augment" && augment.SessionOnlyEvent(event) {
		return ""
	}
	if target == "antigravity" && !antigravity.MatcherEvent(event) {
		return ""
	}
	return matcher
}

// HookPluginRoot is the plugin directory a target sets PLUGIN_ROOT to,
// or "" when it has none.
func HookPluginRoot(cfg *config.Config, target string) string {
	if target == "goose" {
		return goose.PluginRoot(cfg)
	}
	return ""
}
