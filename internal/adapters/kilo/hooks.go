package kilo

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// defaultPluginsDir is the project-level plugin directory Kilo Code
// loads at startup: "Drop TypeScript or JavaScript files into a
// `plugin/` or `plugins/` folder inside any config directory ...
// Project: `.kilo/plugin/` or legacy `.kilocode/plugin/`" and "Every
// `.ts` or `.js` file in those directories is auto-registered at
// startup" (kilo.ai/docs/automate/extending/plugins, raw
// packages/kilo-docs/pages/automate/extending/plugins.md, read
// 2026-09-23). This adapter writes only the current `.kilo/plugin/`
// path, not the legacy `.kilocode/plugin/` one, the same choice this
// package already makes for rules and skills (see the package doc).
const defaultPluginsDir = ".kilo/plugin"

// toolHookKeys maps a hook spec's `event:` onto one of the two tool
// hook keys a Kilo plugin's returned hook object accepts. The vendor's
// own `.env` guard example returns `{"tool.execute.before": async
// (input, output) => {...}}`, reading `input.tool` to decide whether to
// act, so these are direct keys on the returned object rather than
// event-bus subscriptions.
//
// PreToolUse and PostToolUse are the portable spellings every other
// hook adapter in this repo already carries; the native spellings map
// to themselves so a spec authored straight from the vendor doc, or
// imported from a Kilo or OpenCode plugin, is not treated as
// unmappable. Kilo's own reference page says its behavior "is identical
// to OpenCode" here, and internal/adapters/opencode/hooks.go already
// carries the same vocabulary for `.opencode/plugins/`.
var toolHookKeys = map[string]string{
	"PreToolUse":          "tool.execute.before",
	"PostToolUse":         "tool.execute.after",
	"tool.execute.before": "tool.execute.before",
	"tool.execute.after":  "tool.execute.after",
}

// busEvents is the event vocabulary the vendor's Events section lists,
// minus the two groups its own examples show as direct hook keys
// instead: Tool events (see toolHookKeys) and Shell events (see
// shellMutationHooks). A plugin subscribes to these through the single
// `event` hook and switches on `event.type`, exactly as the vendor's
// notification example does for `session.idle`.
var busEvents = map[string]bool{
	"command.executed":       true,
	"file.edited":            true,
	"file.watcher.updated":   true,
	"installation.updated":   true,
	"lsp.client.diagnostics": true,
	"lsp.updated":            true,
	"message.part.removed":   true,
	"message.part.updated":   true,
	"message.removed":        true,
	"message.updated":        true,
	"permission.asked":       true,
	"permission.replied":     true,
	"server.connected":       true,
	"session.created":        true,
	"session.compacted":      true,
	"session.deleted":        true,
	"session.diff":           true,
	"session.error":          true,
	"session.idle":           true,
	"session.status":         true,
	"session.updated":        true,
	"todo.updated":           true,
}

// shellMutationHooks names the documented hook keys whose entire
// purpose is rewriting the `output` object the handler receives:
// `shell.env` sets `output.env.<KEY>`, and
// `experimental.session.compacting` pushes onto `output.context` or
// replaces `output.prompt`. agnostic-ai's portable hook spec carries a
// command to run, not a value to assign, so there is nothing to
// generate for either. They earn a coverage note rather than a plugin
// that runs a command and throws the result away.
var shellMutationHooks = map[string]bool{
	"shell.env":                       true,
	"experimental.session.compacting": true,
}

// claudeToolNames flags the capitalized matcher values a hook spec
// copied from a Claude or Codex source carries. Kilo's own tool names
// are lowercase in its automate/tools group listing ("read", "glob",
// "grep", "edit", "write", "apply_patch", "bash", "webfetch",
// "websearch"; see the package doc), so a Claude-cased matcher compiles
// into a valid regex that then matches nothing.
var claudeToolNames = map[string]bool{
	"Bash": true, "Read": true, "Write": true, "Edit": true,
	"Glob": true, "Grep": true, "WebFetch": true, "WebSearch": true,
}

// emitHooks writes one Kilo plugin module per hook spec at
// `<dir>/<name>.ts`. Kilo's hook surface is codegen, not a config key:
// the tool loads JavaScript or TypeScript modules from the plugin
// directory and calls the exported `server` function, which returns
// the hook object. So this emitter renders a module rather than
// merging into `kilo.jsonc`.
//
// The shell command rides Bun's injected `$` helper, the same context
// field the vendor's own dependency example destructures. A spec with
// no `event:` or no `command:` produces no file, matching every other
// hook adapter.
func emitHooks(sess *emit.Session, hooks []spec.Entry, dir string, dryRun bool) error {
	var unmapped, mutationOnly, timeouts, busMatchers, claudeMatchers, badMatchers int

	for _, h := range hooks {
		event, _ := h.Meta["event"].(string)
		commands := emit.HookCommands(h.Meta["command"])
		if event == "" || len(commands) == 0 || h.Name == "" {
			continue
		}
		hookKey, isToolHook := toolHookKeys[event]
		switch {
		case isToolHook:
		case busEvents[event]:
		case shellMutationHooks[event]:
			mutationOnly++
			continue
		default:
			unmapped++
			continue
		}

		if emit.HookIntMeta(h.Meta, "timeout") > 0 {
			timeouts++
		}

		matcher, _ := h.Meta["matcher"].(string)
		// Claude spells "every tool" as `*`, which is not a valid regex
		// on its own. Emitting `new RegExp("*")` would throw at plugin
		// load and take the whole module down, so both the wildcard and
		// the empty matcher render as no guard at all.
		if matcher == "*" {
			matcher = ""
		}
		switch {
		case matcher == "":
		case !isToolHook:
			busMatchers++
			matcher = ""
		case !validMatcher(matcher):
			badMatchers++
			matcher = ""
		case claudeToolNames[matcher]:
			claudeMatchers++
		}

		rewritten := make([]string, 0, len(commands))
		for _, raw := range commands {
			cmd := emit.RewriteHookPath(raw, target)
			rewritten = append(rewritten, cmd)
			sourceTool, _ := emit.SourceToolFromHookCommand(raw)
			if err := sess.MaterializeHookScript(cmd, target, sourceTool, dryRun); err != nil {
				return err
			}
		}

		body := pluginModule(h.Name, hookKey, event, isToolHook, matcher, rewritten)
		path := filepath.Join(dir, h.Name+".ts")
		if err := sess.WriteFile(path, emit.WithHeader(body, emit.FormatJavaScript), dryRun); err != nil {
			return err
		}
	}

	emit.NoteCoverageGap(target, spec.KindHook, unmapped,
		"Kilo names its own events (tool.execute.before, tool.execute.after, session.idle, file.edited, ...); an event outside that vocabulary has no plugin hook to attach to")
	emit.NoteCoverageGap(target, spec.KindHook, mutationOnly,
		"shell.env and experimental.session.compacting exist to rewrite output.env, output.context, or output.prompt, which a command-running hook spec cannot express")
	emit.NoteFieldNoOp(target, spec.KindHook, "timeout", timeouts,
		"Kilo plugin hooks are awaited with no deadline; wrap the command in timeout(1) to bound it")
	emit.NoteFieldNoOp(target, spec.KindHook, "matcher", busMatchers,
		"Kilo's event hook payload carries no tool name, so only tool.execute.before and tool.execute.after can filter on one")
	emit.NoteFieldNoOp(target, spec.KindHook, "matcher", badMatchers,
		"the value is not a valid regular expression, and new RegExp() would throw at plugin load")
	emit.NoteFieldNoOp(target, spec.KindHook, "matcher", claudeMatchers,
		"Kilo's own tool names are lowercase (bash, read, write, edit); a Claude-style matcher compiles but matches nothing")
	return nil
}

// validMatcher reports whether matcher compiles as a regular
// expression. Go's RE2 is not JavaScript's engine, but it rejects the
// unbalanced and dangling-quantifier shapes that make `new RegExp()`
// throw, which is the failure this guard exists to prevent: a throwing
// constructor at module scope stops Kilo loading the plugin at all.
func validMatcher(matcher string) bool {
	_, err := regexp.Compile(matcher)
	return err == nil
}

// pluginModule renders one local-file plugin module. The body follows
// the vendor's own "Basic structure" example verbatim: a type-only
// import of `Plugin` from `@kilocode/plugin`, then a named async
// function returning the hook object. Kilo's module shape then differs
// from OpenCode's exported-function form: "Plugins must default-export
// a module descriptor. `id` is required for local-file plugins", shown
// as `export default { id: "my-plugin", server }`. The Troubleshooting
// section's most common failure is exactly this: "Plugin loaded but
// hooks never fire — make sure the default export includes `server`",
// so that export is never optional here.
//
// Only `$` is destructured out of the context object, the same partial
// destructure OpenCode's counterpart uses: the vendor's own
// dependency-injection example takes the whole context, so binding only
// what the generated body reads is equally valid and avoids an unused
// binding under a strict tsconfig.
func pluginModule(name, hookKey, event string, isToolHook bool, matcher string, commands []string) string {
	ident := pluginIdentifier(name)
	var sb strings.Builder
	sb.WriteString("import type { Plugin } from \"@kilocode/plugin\"\n\n")
	sb.WriteString("const " + ident + ": Plugin = async ({ $ }) => {\n")
	sb.WriteString("  return {\n")
	if isToolHook {
		sb.WriteString("    " + jsString(hookKey) + ": async (" + toolHandlerParams(matcher) + ") => {\n")
		if matcher != "" {
			sb.WriteString("      if (!new RegExp(" + jsString(matcher) + ").test(input.tool)) return\n")
		}
	} else {
		sb.WriteString("    event: async ({ event }) => {\n")
		sb.WriteString("      if (event.type !== " + jsString(event) + ") return\n")
	}
	for _, cmd := range commands {
		sb.WriteString("      await $`" + escapeTemplateLiteral(cmd) + "`\n")
	}
	sb.WriteString("    },\n")
	sb.WriteString("  }\n")
	sb.WriteString("}\n\n")
	sb.WriteString("export default { id: " + jsString(name) + ", server: " + ident + " }\n")
	return sb.String()
}

// toolHandlerParams returns the parameter list for a tool hook handler.
// The vendor signature is `(input, output)`; we bind only what the
// generated body reads, so a matcher-less hook declares no parameters
// at all rather than two unused ones.
func toolHandlerParams(matcher string) string {
	if matcher == "" {
		return ""
	}
	return "input"
}

// pluginIdentifier turns a hook spec name into a legal JavaScript
// identifier, PascalCased and suffixed to read as a plugin function
// name (`GuardBashPlugin`, `NotifyIdlePlugin`). This is the local
// variable Kilo's default-export `server` key points at; the vendor's
// own examples name it freely (`hello`, `server`, `ProviderPlugin`), so
// there is no naming convention to match beyond producing a valid
// identifier. Anything that is not a letter or digit separates words; a
// name that would start with a digit gets a `Hook` prefix so the result
// is always legal.
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

// escapeTemplateLiteral prepares a shell command for verbatim insertion
// into a Bun `$` tagged template. Bun's `$` is a cross-platform shell,
// so the command goes in as written rather than through `sh -c`, which
// would not run on Windows. Backslashes, backticks, and `${` are the
// three sequences JavaScript reads specially inside a template literal;
// escaping them makes the runtime hand the shell the author's exact
// string.
func escapeTemplateLiteral(cmd string) string {
	cmd = strings.ReplaceAll(cmd, `\`, `\\`)
	cmd = strings.ReplaceAll(cmd, "`", "\\`")
	cmd = strings.ReplaceAll(cmd, "${", "\\${")
	return cmd
}

// jsString renders s as a double-quoted JavaScript string literal. JSON
// string syntax is a subset of JavaScript's, so the encoder's escaping
// is correct here and covers the quotes, backslashes, and control
// characters a matcher, event name, or plugin id could carry.
func jsString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}
