package opencode

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

//go:embed hooks_v2.ts.txt
var v2HookModule string

type v2Command struct {
	Command string    `json:"command"`
	Args    *[]string `json:"args,omitempty"`
}

func validateV2Hook(hook spec.Entry) error {
	hook = emit.TargetHook(target, hook)
	event, _ := hook.Meta["event"].(string)
	if _, ok := emit.PluginToolHookKeys[event]; !ok {
		return fmt.Errorf("%s: opencode plugin-api v2 does not support event %q; use PreToolUse, PostToolUse, tool.execute.before, or tool.execute.after", hookLocation(hook), event)
	}
	meta := emit.ResolveMeta(hook.Meta, target)
	for _, command := range emit.HookCommands(meta["command"]) {
		if strings.ContainsRune(command, '\x00') {
			return fmt.Errorf("%s: opencode v2 command must not contain NUL", hookLocation(hook))
		}
	}
	if raw, exists := meta["timeout"]; exists {
		seconds := emit.HookIntMeta(meta, "timeout")
		if seconds <= 0 || seconds > 86400 || fmt.Sprint(raw) != strconv.Itoa(seconds) {
			return fmt.Errorf("%s: opencode v2 timeout %v must be between 1 and 86400 seconds", hookLocation(hook), raw)
		}
	}
	if raw, exists := meta["args"]; exists {
		switch values := raw.(type) {
		case []string:
		case []any:
			for _, arg := range values {
				if _, ok := arg.(string); !ok {
					return fmt.Errorf("%s: opencode v2 args must contain strings", hookLocation(hook))
				}
			}
		default:
			return fmt.Errorf("%s: opencode v2 args must be a list", hookLocation(hook))
		}
		for index, arg := range emit.HookArgs(target, hook.Meta) {
			if strings.ContainsRune(arg, '\x00') {
				return fmt.Errorf("%s: opencode v2 argument %d must not contain NUL", hookLocation(hook), index+1)
			}
		}
	}
	if raw, exists := meta["env"]; exists {
		values, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: opencode v2 env must be a map of strings", hookLocation(hook))
		}
		for key, value := range values {
			if key == "" || strings.ContainsRune(key, '=') {
				return fmt.Errorf("%s: opencode v2 environment name must be nonempty and must not contain =", hookLocation(hook))
			}
			text, ok := value.(string)
			if !ok {
				return fmt.Errorf("%s: opencode v2 env must contain strings", hookLocation(hook))
			}
			if strings.ContainsRune(key, '\x00') || strings.ContainsRune(text, '\x00') {
				return fmt.Errorf("%s: opencode v2 environment name and value must not contain NUL", hookLocation(hook))
			}
		}
	}
	return nil
}

func emitV2Hook(sess *emit.Session, hook spec.Entry, dir string, dryRun bool) error {
	hook = emit.TargetHook(target, hook)
	commands := emit.HookCommands(hook.Meta["command"])
	if len(commands) == 0 || hook.Name == "" || emit.HookBoolMeta(hook.Meta, "disabled") {
		return nil
	}
	meta := emit.ResolveMeta(hook.Meta, target)
	_, literal := meta["args"]
	materialized := hook
	if literal && len(emit.HookArgs(target, hook.Meta)) == 0 {
		materialized.Meta = maps.Clone(hook.Meta)
		quoted := make([]string, len(commands))
		for i, command := range commands {
			quoted[i] = emit.ShellQuote(command)
		}
		materialized.Meta["command"] = quoted
	}
	if err := sess.MaterializeNeutralHookScripts([]spec.Entry{materialized}, target, emit.HookScriptsDir(target), dryRun); err != nil {
		return err
	}
	var rendered []v2Command
	for _, command := range commands {
		entry := v2Command{Command: emit.RewriteHookPath(command, target, hook.Meta)}
		if literal {
			entry.Command = emit.RewriteHookRoot(emit.RewriteHookDirectories(command, target, true), target, hook.Meta)
			args := emit.HookArgs(target, hook.Meta)
			if args == nil {
				args = []string{}
			}
			entry.Args = &args
		}
		rendered = append(rendered, entry)
		sourceTool, _ := emit.SourceToolFromHookCommand(emit.RewriteNeutralHookPath(command, ".", literal))
		if err := sess.MaterializeHookScript(entry.Command, target, sourceTool, dryRun); err != nil {
			return err
		}
	}
	seconds := emit.HookIntMeta(meta, "timeout")
	if seconds == 0 {
		seconds = 60
	}
	env := map[string]string{}
	if values, ok := meta["env"].(map[string]any); ok {
		for name, value := range values {
			env[name] = value.(string)
		}
	}
	matcher, _ := hook.Meta["matcher"].(string)
	if matcher == "" || matcher == "*" {
		matcher = ".*"
	}
	event, _ := hook.Meta["event"].(string)
	native := strings.TrimPrefix(emit.PluginToolHookKeys[event], "tool.")
	afterFilter, block := "", ""
	if event == "PostToolUse" {
		afterFilter = `if (event.status !== "completed") return`
	}
	if native == "execute.before" {
		block = `if (result.code === 2) throw new Error(result.stderr.trim() || ` + v2JSON("blocked by hook "+hook.Name) + `)`
	}
	body := strings.NewReplacer(
		"__NAME__", v2JSON(hook.Name),
		"__ID__", v2JSON("agnostic-ai."+hook.Name),
		"__COMMANDS__", v2JSON(rendered),
		"__TIMEOUT__", strconv.Itoa(seconds*1000),
		"__ENV__", v2JSON(env),
		"__MATCHER__", v2JSON("^(?:"+matcher+")$"),
		"__EVENT__", v2JSON(native),
		"__AFTER_FILTER__", afterFilter,
		"__BLOCK__", block,
	).Replace(v2HookModule)
	return sess.WriteFile(filepath.Join(dir, hook.Name+".ts"), emit.WithHeader(body, emit.FormatJavaScript), dryRun)
}

func v2JSON(value any) string {
	body, _ := json.Marshal(value)
	return string(body)
}
