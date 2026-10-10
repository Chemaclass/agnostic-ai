package kiro

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// hooksFile is the `.kiro/hooks/<name>.json` shape
// (kiro.dev/docs/hooks/): a `version` plus a `hooks` array. Kiro's own
// examples wrap a single hook in the array; the array shape is what the
// vendor schema documents regardless of how many entries one file
// carries, so a spec with several `command:` entries shares one file
// (see buildHookEntries) instead of spawning one file per command.
//
// Hooks marshal as `map[string]any` rather than a fixed struct: a Go
// struct has no route for a key it does not declare, at any layer,
// including x-kiro, which is how `description` and `confirm` stayed
// unreachable across three audit passes (#642). `map[string]any` keys
// alpha-sort under `encoding/json`, so key order below is illustrative,
// not the emitted order.
type hooksFile struct {
	// Version is a string, not a number: the vendor field reference
	// documents `version` as `Schema version - currently "v1"`.
	Version string           `json:"version"`
	Hooks   []map[string]any `json:"hooks"`
}

// commandHookAction renders portable commands. Native x-kiro.action
// objects use maps after validating the action type and required field.
type commandHookAction struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// emitHooks writes one `<dir>/<name>.json` per hook spec. A spec's
// `command:` field (string or list) becomes one `hooks[]` entry per
// command in that same file, `name` suffixed `-2`, `-3`, ... past the
// first so entries sharing a file stay unique. A native x-kiro.action
// replaces the command list with one action. Hooks without an event or
// action produce no output; invalid native actions return an error.
// No file materializes for a hook scoped away from kiro by `target:` /
// `targets:` (b.HooksFor filters those out before this function sees them).
func emitHooks(sess *emit.Session, hooks []spec.Entry, dir string, dryRun bool) error {
	for _, h := range hooks {
		doc, err := renderHookFile(h, projectCommands)
		if err != nil {
			return err
		}
		if doc == "" {
			continue
		}
		path := filepath.Join(dir, h.Name+".json")
		if err := sess.WriteFile(path, doc, dryRun); err != nil {
			return err
		}
	}
	return sess.MaterializeNeutralHookScripts(hooks, target, emit.HookScriptsDir(target), dryRun)
}

// renderHookFile is one hook spec as the text of its `<name>.json` file,
// or "" when the spec has nothing to write.
func renderHookFile(h spec.Entry, commands hookCommandForm) (string, error) {
	entries, err := buildHookEntries(h, commands)
	if err != nil {
		return "", fmt.Errorf("kiro hook %s: %w", h.Name, err)
	}
	if len(entries) == 0 {
		return "", nil
	}
	raw, err := emit.MarshalJSONIndent(hooksFile{Version: "v1", Hooks: entries})
	if err != nil {
		return "", fmt.Errorf("kiro hook %s: %w", h.Name, err)
	}
	return string(raw) + "\n", nil
}

// hookCommandForm spells the commands of a hook the way one install
// location needs them: a project file keeps paths relative to the
// workspace, a user file points at the user's own script directory.
type hookCommandForm struct {
	// shell renders one portable `command:` entry with its args folded in.
	shell func(command string, meta map[string]any) string
	// native rewrites the command of an `x-kiro.action`.
	native func(command string) string
}

var projectCommands = hookCommandForm{
	shell: func(command string, meta map[string]any) string {
		return emit.ShellHookCommand(command, target, meta)
	},
	native: func(command string) string {
		return emit.RewriteNeutralHookPath(command, emit.HookScriptsDir(target))
	},
}

// UserHookFiles renders hooks as the files of the user hooks directory,
// keyed by file name, with commands that run the scripts copied into
// scriptsDir from sourceDir. A hook with nothing to write has no file.
func (Adapter) UserHookFiles(hooks []spec.Entry, sourceDir, scriptsDir string) (map[string]string, []emit.HookScript, error) {
	scripts, err := emit.HookScriptsFor(hooks, target, sourceDir, scriptsDir)
	if err != nil {
		return nil, nil, err
	}
	commandDir := commandPath(scriptsDir)
	commands := hookCommandForm{
		shell: func(command string, meta map[string]any) string {
			return emit.ExecFormCommand(emit.RewriteGlobalHookPath(command, target, commandDir, meta), emit.HookArgs(target, meta))
		},
		native: func(command string) string {
			return emit.RewriteNeutralHookPath(command, commandDir)
		},
	}
	files := map[string]string{}
	for _, h := range hooks {
		doc, err := renderHookFile(h, commands)
		if err != nil {
			return nil, nil, err
		}
		if doc != "" {
			files[h.Name+".json"] = doc
		}
	}
	return files, scripts, nil
}

// commandPath spells a directory for a hook command with forward slashes,
// which every shell Kiro runs hooks in reads, unlike a quoted folder or
// mixed separators. A drive or UNC path is converted on any OS; any other
// path only where the separator is a backslash, since a POSIX folder name
// may hold one.
func commandPath(dir string) string {
	windows := len(dir) >= 3 && dir[1] == ':' && dir[2] == '\\' || strings.HasPrefix(dir, `\\`)
	if windows {
		return strings.ReplaceAll(dir, `\`, "/")
	}
	return filepath.ToSlash(dir)
}

// buildHookEntries renders one hooks[] entry per action on h as a
// map[string]any: `name`, `trigger`, `action`, and the optional
// `matcher`/`timeout`/`enabled`/`description` fields agnostic-ai's spec
// already carries, plus additional native keys under `x-kiro` (e.g. `confirm`, the
// vendor's Stop-hook confirmation block, which has no agnostic-ai spec
// equivalent and so is only reachable this way). Every entry sharing
// this hook spec's command list also shares its `description` and
// `x-kiro` passthrough, since both live on the spec, not per-command.
// A native action needs no generic command and replaces the command
// list. Returns no entries when h has no event or no usable action.
func buildHookEntries(h spec.Entry, commands hookCommandForm) ([]map[string]any, error) {
	trigger, _ := h.Meta["event"].(string)
	if trigger == "" {
		return nil, nil
	}
	actions, err := hookActions(h, commands)
	if err != nil {
		return nil, err
	}
	if len(actions) == 0 {
		return nil, nil
	}
	matcher, _ := h.Meta["matcher"].(string)
	timeout, hasTimeout := hookTimeout(h.Meta)
	description, _ := h.Meta["description"].(string)
	disabled, _ := h.Meta["disabled"].(bool)

	entries := make([]map[string]any, 0, len(actions))
	for i, action := range actions {
		name := h.Name
		if i > 0 {
			name = fmt.Sprintf("%s-%d", h.Name, i+1)
		}
		entry := map[string]any{
			"name":    name,
			"trigger": trigger,
			"action":  action,
		}
		var keys []string
		if matcher != "" {
			entry["matcher"] = matcher
		}
		if hasTimeout {
			entry["timeout"] = timeout
		}
		if disabled {
			// The vendor default (enabled) needs no explicit key,
			// mirroring the disabled/enabled convention this adapter
			// already uses for MCP entries.
			entry["enabled"] = false
		}
		if description != "" {
			entry["description"] = description
		}
		emit.MergeCustomTargetMeta(entry, &keys, h.Meta, target,
			"name", "trigger", "matcher", "action", "timeout", "enabled", "description")
		entries = append(entries, entry)
	}
	return entries, nil
}

// Zero disables Kiro's timeout, so invalid or absent values must stay
// distinguishable from a successfully parsed numeric or quoted zero.
func hookTimeout(meta map[string]any) (int, bool) {
	if value, ok := meta["timeout"].(string); ok {
		timeout, err := strconv.Atoi(strings.TrimSpace(value))
		return timeout, err == nil
	}
	return emit.IntField(meta, "timeout")
}

func hookActions(h spec.Entry, form hookCommandForm) ([]any, error) {
	if native, ok := h.Meta["x-kiro"].(map[string]any); ok {
		if raw, exists := native["action"]; exists {
			action, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("x-kiro.action must be an object")
			}
			actionType, _ := action["type"].(string)
			var field string
			switch actionType {
			case "command":
				field = "command"
			case "agent":
				field = "prompt"
			default:
				return nil, fmt.Errorf("x-kiro.action.type %q must be command or agent", actionType)
			}
			value, ok := action[field].(string)
			if !ok || strings.TrimSpace(value) == "" {
				return nil, fmt.Errorf("x-kiro.action.%s must be a non-empty string for type %q", field, actionType)
			}
			if actionType == "command" {
				copy := make(map[string]any, len(action))
				for key, value := range action {
					copy[key] = value
				}
				copy["command"] = form.native(value)
				return []any{copy}, nil
			}
			return []any{action}, nil
		}
	}
	commands := hookCommands(h.Meta["command"])
	actions := make([]any, 0, len(commands))
	for _, command := range commands {
		actions = append(actions, commandHookAction{Type: "command", Command: form.shell(command, h.Meta)})
	}
	return actions, nil
}

// hookCommands normalizes a `command:` field that may be a string or a
// list of strings into a single []string. Empty strings drop out.
func hookCommands(raw any) []string {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
