package emit

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// PluginHookHost describes one tool that loads hook specs as Bun
// TypeScript plugin modules. OpenCode and Kilo share the plugin runtime
// and the hook vocabulary; they differ in the type package and in how a
// module exports its plugin function.
type PluginHookHost struct {
	// Target is the adapter name, used for coverage notes and hook
	// script paths.
	Target string
	// Tool is the product name coverage notes print ("OpenCode").
	Tool string
	// ImportModule is the package the type-only `Plugin` import names.
	ImportModule string
	// DefaultExport renders `export default { id, server }` (Kilo)
	// instead of an exported const (OpenCode).
	DefaultExport bool
	// BusEvents is the event vocabulary a plugin reaches through the
	// single `event` hook. Start from PluginBusEvents.
	BusEvents map[string]bool
}

// PluginToolHookKeys maps a spec event to one of the two direct tool
// hook keys a plugin's returned hook object accepts. The native names
// map to themselves so a spec authored against the vendor doc, or
// imported from a plugin, is not treated as unmappable.
var PluginToolHookKeys = map[string]string{
	"PreToolUse":          "tool.execute.before",
	"PostToolUse":         "tool.execute.after",
	"tool.execute.before": "tool.execute.before",
	"tool.execute.after":  "tool.execute.after",
}

// PluginBusEvents is the event vocabulary OpenCode and Kilo both list,
// minus the tool events (PluginToolHookKeys) and the shell events
// (pluginMutationHooks) their own examples show as direct hook keys.
// A plugin subscribes to these through the `event` hook and switches on
// `event.type`. Returns a fresh map so a host can add its own groups.
func PluginBusEvents() map[string]bool {
	out := map[string]bool{}
	for _, e := range []string{
		"command.executed", "file.edited", "file.watcher.updated",
		"installation.updated", "lsp.client.diagnostics", "lsp.updated",
		"message.part.removed", "message.part.updated", "message.removed",
		"message.updated", "permission.asked", "permission.replied",
		"server.connected", "session.created", "session.compacted",
		"session.deleted", "session.diff", "session.error", "session.idle",
		"session.status", "session.updated", "todo.updated",
	} {
		out[e] = true
	}
	return out
}

// pluginMutationHooks are the shell events that exist to rewrite
// output.env, output.context, or output.prompt, which a command-running
// hook spec cannot express.
var pluginMutationHooks = map[string]bool{
	"shell.env":                       true,
	"experimental.session.compacting": true,
}

// claudeToolNames is the Claude tool vocabulary a matcher copied from a
// Claude or Codex source carries. Plugin tool names are lowercase
// (`input.tool === "bash"`), so a Claude-cased name never matches.
var claudeToolNames = map[string]bool{
	"Bash": true, "Read": true, "Write": true, "Edit": true,
	"Glob": true, "Grep": true, "WebFetch": true, "WebSearch": true,
}

// blockExitCode is the exit status that blocks a tool call, matching
// Claude's PreToolUse contract: exit 2 blocks, any other failure is a
// warning and the tool still runs.
const blockExitCode = 2

// EmitPluginHooks writes one plugin module per hook spec at
// `<dir>/<name>.ts`. The hook surface is codegen, not a config key: the
// tool loads every module in its plugin directory at startup.
//
// A spec with no `event:` or no `command:` produces no file, matching
// every other hook adapter. A spec with `disabled: true` produces no
// file either: a module in the plugin directory always runs, so leaving
// it out is the only way to turn it off.
func EmitPluginHooks(sess *Session, host PluginHookHost, hooks []spec.Entry, dir string, dryRun bool) error {
	var unmapped, mutationOnly, timeouts, busMatchers, claudeMatchers, badMatchers int

	for _, h := range hooks {
		event, _ := h.Meta["event"].(string)
		commands := HookCommands(h.Meta["command"])
		if event == "" || len(commands) == 0 || h.Name == "" || HookBoolMeta(h.Meta, "disabled") {
			continue
		}
		hookKey, isToolHook := PluginToolHookKeys[event]
		switch {
		case isToolHook:
		case host.BusEvents[event]:
		case pluginMutationHooks[event]:
			mutationOnly++
			continue
		default:
			unmapped++
			continue
		}

		if HookIntMeta(h.Meta, "timeout") > 0 {
			timeouts++
		}

		matcher, _ := h.Meta["matcher"].(string)
		// Claude spells "every tool" as `*`, which is not a regex on its
		// own, so both the wildcard and the empty matcher render as no
		// guard at all.
		if matcher == "*" {
			matcher = ""
		}
		switch {
		case matcher == "":
		case !isToolHook:
			busMatchers++
			matcher = ""
		case !jsCompatibleMatcher(matcher):
			badMatchers++
			matcher = ""
		case hasClaudeToolName(matcher):
			claudeMatchers++
		}

		rewritten := make([]string, 0, len(commands))
		for _, raw := range commands {
			cmd := RewriteHookPath(raw, host.Target)
			rewritten = append(rewritten, cmd)
			sourceTool, _ := SourceToolFromHookCommand(raw)
			if err := sess.MaterializeHookScript(cmd, host.Target, sourceTool, dryRun); err != nil {
				return err
			}
		}

		body := pluginModule(host, h.Name, hookKey, event, matcher, rewritten)
		path := filepath.Join(dir, h.Name+".ts")
		if err := sess.WriteFile(path, WithHeader(body, FormatJavaScript), dryRun); err != nil {
			return err
		}
	}

	NoteCoverageGap(host.Target, spec.KindHook, unmapped,
		host.Tool+" names its own events (tool.execute.before, tool.execute.after, session.idle, file.edited, ...); an event outside that vocabulary has no plugin hook to attach to")
	NoteCoverageGap(host.Target, spec.KindHook, mutationOnly,
		"shell.env and experimental.session.compacting exist to rewrite output.env, output.context, or output.prompt, which a command-running hook spec cannot express")
	NoteFieldNoOp(host.Target, spec.KindHook, "timeout", timeouts,
		host.Tool+" plugin hooks are awaited with no deadline; wrap the command in timeout(1) to bound it")
	NoteFieldNoOp(host.Target, spec.KindHook, "matcher", busMatchers,
		host.Tool+"'s event hook payload carries no tool name, so only tool.execute.before and tool.execute.after can filter on one")
	NoteFieldNoOp(host.Target, spec.KindHook, "matcher", badMatchers,
		"the value is not a regular expression JavaScript reads the same way (RE2-only syntax such as (?i) or \\A), and new RegExp() would throw or match something else")
	NoteFieldNoOp(host.Target, spec.KindHook, "matcher", claudeMatchers,
		host.Tool+"'s own tool names are lowercase (bash, read, write, edit); a Claude-style matcher compiles but matches nothing")
	return nil
}

// hasClaudeToolName reports whether any alternative of matcher is a
// Claude tool name, so `Edit|Write` is caught as well as `Bash`.
func hasClaudeToolName(matcher string) bool {
	for _, alt := range strings.Split(matcher, "|") {
		if claudeToolNames[strings.Trim(alt, "()^$ ")] {
			return true
		}
	}
	return false
}

// jsCompatibleMatcher reports whether matcher compiles and means the
// same thing to JavaScript's RegExp as it does to Go's RE2. RE2 accepts
// inline flags (`(?i)`), `(?P<name>`, `\A`, `\z`, `\Q...\E`, `\pL` and
// POSIX classes. JavaScript either throws on those or reads them as
// literal letters, so a matcher using them is dropped rather than
// shipped as a guard that throws or never matches.
func jsCompatibleMatcher(matcher string) bool {
	if _, err := regexp.Compile(matcher); err != nil {
		return false
	}
	if strings.Contains(matcher, "[[:") {
		return false
	}
	for i := 0; i < len(matcher); i++ {
		switch matcher[i] {
		case '\\':
			if i+1 < len(matcher) && strings.IndexByte("AzQECpP", matcher[i+1]) >= 0 {
				return false
			}
			i++
		case '(':
			if !strings.HasPrefix(matcher[i+1:], "?") {
				continue
			}
			rest := matcher[i+2:]
			named := strings.HasPrefix(rest, "<") && len(rest) > 1 && unicode.IsLetter(rune(rest[1]))
			if !strings.HasPrefix(rest, ":") && !named {
				return false
			}
		}
	}
	return true
}

// pluginModule renders one plugin module: a type-only import of
// `Plugin`, then an async function that destructures only Bun's `$`
// (binding context fields the body never reads would trip a strict
// tsconfig) and returns the hook object.
//
// The matcher is anchored, because Claude matches a plain tool name
// exactly (`write` must not fire on `todowrite`), and compiled once at
// module scope rather than on every tool call.
func pluginModule(host PluginHookHost, name, hookKey, event, matcher string, commands []string) string {
	ident := pluginIdentifier(name)
	isToolHook := hookKey != ""
	var sb strings.Builder
	sb.WriteString("import type { Plugin } from " + jsString(host.ImportModule) + "\n\n")
	if matcher != "" {
		sb.WriteString("const MATCHER = new RegExp(" + jsString("^(?:"+matcher+")$") + ")\n\n")
	}
	if !host.DefaultExport {
		sb.WriteString("export ")
	}
	sb.WriteString("const " + ident + ": Plugin = async ({ $ }) => {\n")
	sb.WriteString("  return {\n")
	if isToolHook {
		params := ""
		if matcher != "" {
			params = "input"
		}
		sb.WriteString("    " + jsString(hookKey) + ": async (" + params + ") => {\n")
		if matcher != "" {
			sb.WriteString("      if (!MATCHER.test(input.tool)) return\n")
		}
	} else {
		sb.WriteString("    event: async ({ event }) => {\n")
		sb.WriteString("      if (event.type !== " + jsString(event) + ") return\n")
	}
	blocking := hookKey == "tool.execute.before"
	for i, cmd := range commands {
		run := shellCall(cmd)
		if !blocking {
			sb.WriteString("      await " + run + "\n")
			continue
		}
		v := "r" + strconv.Itoa(i+1)
		sb.WriteString("      const " + v + " = await " + run + "\n")
		sb.WriteString("      if (" + v + ".exitCode === " + strconv.Itoa(blockExitCode) + ") throw new Error(" +
			v + ".stderr.toString() || " + jsString("blocked by hook "+name) + ")\n")
	}
	sb.WriteString("    },\n")
	sb.WriteString("  }\n")
	sb.WriteString("}\n")
	if host.DefaultExport {
		sb.WriteString("\nexport default { id: " + jsString(name) + ", server: " + ident + " }\n")
	}
	return sb.String()
}

// shellCall renders one command as a Bun `$` call. The command goes in
// as a `{ raw }` value, which Bun passes to its shell untouched, so no
// template-literal escaping can change what the author wrote. Bun's `$`
// throws on a non-zero exit, which would abort the tool call or skip the
// next command, so `.nothrow()` hands the exit status back instead.
func shellCall(cmd string) string {
	return "$`${{ raw: " + jsString(cmd) + " }}`.nothrow()"
}

// pluginIdentifier turns a hook spec name into a legal JavaScript
// identifier, PascalCased and suffixed to read as a plugin function
// name (`GuardBashPlugin`). Anything that is not a letter or digit
// separates words; a name that would start with a digit gets a `Hook`
// prefix so the result is always legal.
func pluginIdentifier(name string) string {
	var sb strings.Builder
	upperNext := true
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if upperNext {
				sb.WriteRune(unicode.ToUpper(r))
				upperNext = false
				continue
			}
			sb.WriteRune(r)
		default:
			upperNext = true
		}
	}
	out := sb.String()
	if out == "" || unicode.IsDigit(rune(out[0])) {
		out = "Hook" + out
	}
	return out + "Plugin"
}

// jsString renders s as a double-quoted JavaScript string literal. JSON
// string syntax is a subset of JavaScript's, so the encoder's escaping
// covers the quotes, backslashes, and control characters a command,
// matcher, event name, or plugin id could carry. HTML escaping is off
// so a command such as `make 2>&1` stays readable in the module.
func jsString(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return `""`
	}
	return strings.TrimSuffix(buf.String(), "\n")
}
