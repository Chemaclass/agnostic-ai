package crush

import (
	"fmt"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func HookEvents() []string {
	return []string{
		"PreToolUse", "PostToolUse", "PostToolUseFailure", "PermissionRequest", "PermissionResult",
		"SessionStart", "SessionEnd", "TurnStart", "TurnEnd", "StopFailure", "Interrupt", "PreCompact", "PostCompact",
	}
}

func CanonicalHookEvent(event string) string {
	folded := normalizeCrushEvent(event)
	for _, known := range HookEvents() {
		if folded == normalizeCrushEvent(known) {
			return known
		}
	}
	return ""
}

func HookAcceptsMatcher(event string) bool {
	switch CanonicalHookEvent(event) {
	case "PreToolUse", "PostToolUse", "PostToolUseFailure", "PermissionRequest", "PermissionResult":
		return true
	}
	return false
}

func normalizeCrushEvent(event string) string {
	return strings.ToLower(strings.ReplaceAll(event, "_", ""))
}

// Claude-style matchers parse but do not match Crush's lowercase tool names.
var crushClaudeToolNames = map[string]bool{
	"Bash": true, "Read": true, "Write": true, "Edit": true,
	"Glob": true, "Grep": true, "WebFetch": true, "WebSearch": true,
}

func buildHooksBlock(hooks []spec.Entry) (map[string]any, error) {
	byEvent := map[string]any{}
	var otherEvents, claudeMatchers int

	for _, h := range hooks {
		rawEvent, _ := h.Meta["event"].(string)
		event := CanonicalHookEvent(rawEvent)
		if event == "" {
			if rawEvent != "" {
				otherEvents++
			}
			continue
		}
		commands := emit.HookCommands(h.Meta["command"])
		if len(commands) == 0 {
			continue
		}
		matcher, _ := h.Meta["matcher"].(string)
		if matcher != "" && !HookAcceptsMatcher(event) {
			source := h.Path
			if source == "" {
				source = h.Name
			}
			return nil, fmt.Errorf("crush hook %s: %s does not accept a matcher; remove matcher or use a tool event", source, event)
		}
		if crushClaudeToolNames[matcher] {
			claudeMatchers++
		}
		timeout := emit.HookIntMeta(h.Meta, "timeout")

		for _, command := range commands {
			// Crush's embedded POSIX shell runs hooks on every platform.
			// The ./ goes on before the args fold may quote the path.
			rewritten := emit.ExecFormCommand(emit.DotSlashSyncedHookScript(command, emit.RewriteHookPath(command, target, h.Meta), target), emit.HookArgs(target, h.Meta))
			rewritten = emit.PortableHookCommand(h, target, rewritten, func(path string) string {
				return emit.DotSlashSyncedHookScript(path, emit.RewriteHookPath(path, target), target)
			})
			entry := map[string]any{"command": emit.ExportHookTarget(rewritten, target)}
			name := h.Name
			if native, ok := h.Meta["x-crush"].(map[string]any); ok {
				if label, ok := native["name"].(string); ok {
					name = label
				}
			}
			if name != "" {
				entry["name"] = name
			}
			if matcher != "" {
				entry["matcher"] = matcher
			}
			if timeout > 0 {
				entry["timeout"] = timeout
			}
			entries, _ := byEvent[event].([]any)
			byEvent[event] = append(entries, entry)
		}
	}

	emit.NoteCoverageGap(target, spec.KindHook, otherEvents,
		"Crush does not support this hook event; use a released lifecycle event")
	emit.NoteFieldNoOp(target, spec.KindHook, "matcher", claudeMatchers,
		"Crush's own tool names are lowercase (bash, edit, write, mcp_<server>_<tool>), not Claude's; a Claude-style matcher parses but matches nothing")

	if len(byEvent) == 0 {
		return nil, nil
	}
	return byEvent, nil
}
