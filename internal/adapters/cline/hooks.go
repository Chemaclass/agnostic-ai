package cline

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// defaultHooksDir is where hook scripts land. `resolveHooksConfigSearchPaths`
// (sdk/packages/shared/src/storage/paths.ts:487-500) pushes both
// `<workspace>/.clinerules/hooks` and `<workspace>/.cline/hooks`, so
// either works. `.cline/hooks` is the default: it is the one
// `docs.cline.bot/getting-started/config` shows in the project tree, it
// sits beside `.cline/agents` and `.cline/skills`, and it is where
// `emit.RewriteHookPath` and `emit.MaterializeHookScript` already put a
// hook body carried over from a sibling tool. Override with
// `outputs.cline.hooks-dir`.
const defaultHooksDir = ".cline/hooks"

// hookFileExt is the extension every emitted hook script carries.
// `SUPPORTED_HOOK_FILE_EXTENSIONS` (hook-file-config.ts:49) accepts
// "", .sh, .bash, .zsh, .js, .mjs, .cjs, .ts, .mts, .cts, .py and .ps1.
// `.sh` is the one where `inferHookCommand` runs the file as
// `["bash", path]` without needing a shebang, which matters because the
// provenance comment has to be the first line for sync to recognize the
// file as managed. A shebang above it would push the marker down; a
// shebang below it would not be a shebang.
const hookFileExt = ".sh"

// clineHookEvents lists the ten file names Cline discovers, in the order
// `HookConfigFileName` declares them (hook-file-config.ts:17-28).
// Discovery is by file name alone: `toHookConfigFileName` strips the
// extension, lowercases the stem, and looks it up, so the event is the
// file name and there is nowhere else to put one.
var clineHookEvents = []string{
	"TaskStart",
	"TaskResume",
	"TaskCancel",
	"TaskComplete",
	"TaskError",
	"PreToolUse",
	"PostToolUse",
	"UserPromptSubmit",
	"PreCompact",
	"SessionShutdown",
}

// clineHookEventByKey folds an event name the way Cline's own lookup
// does: `HOOK_CONFIG_FILE_LOOKUP` is keyed by the lowercased enum value
// (hook-file-config.ts:54-56). A spec that writes `pretooluse` or
// `PRETOOLUSE` lands on the same file as `PreToolUse`.
var clineHookEventByKey = func() map[string]string {
	m := make(map[string]string, len(clineHookEvents))
	for _, event := range clineHookEvents {
		m[strings.ToLower(event)] = event
	}
	return m
}()

// clineInertHookEvents names the file names Cline lists but never runs.
// `HOOK_CONFIG_FILE_EVENT_MAP` maps `PreCompact` to `undefined`
// (hook-file-config.ts:40) and `createHookCommandMap` skips every entry
// with no runtime event (hook-file-hooks.ts:375-377). The file name is
// still the vendor's own, so the script is written and the gap is
// reported rather than dropped: it starts firing the day Cline wires
// the event up.
var clineInertHookEvents = map[string]bool{"PreCompact": true}

// emitHooks writes one executable script per hook event under the hooks
// directory. Every spec whose `event:` folds onto one of Cline's ten
// file names contributes its commands to that event's script, in spec
// order; specs naming anything else earn a coverage note instead of a
// file no loader looks for.
//
// Hook bodies stashed under `.agnostic-ai/scripts/` materialize into
// `.cline/hooks/` as well, so a hook imported from claude or codex still
// has its script on disk when it syncs out to cline.
func emitHooks(sess *emit.Session, hooks []spec.Entry, cfg *config.Config, dryRun bool) error {
	dir := emit.OutputHooksDir(cfg, target, defaultHooksDir)
	commands := map[string][]string{}
	var order []string
	var unmapped, matchers, timeouts, inert int

	for _, h := range hooks {
		event, _ := h.Meta["event"].(string)
		canonical, known := clineHookEventByKey[strings.ToLower(strings.TrimSpace(event))]
		if !known {
			if event != "" {
				unmapped++
			}
			continue
		}
		cmds := emit.HookCommands(h.Meta["command"])
		if len(cmds) == 0 {
			continue
		}
		if matcher, _ := h.Meta["matcher"].(string); matcher != "" {
			matchers++
		}
		if emit.HookIntMeta(h.Meta, "timeout") > 0 {
			timeouts++
		}
		if clineInertHookEvents[canonical] {
			inert++
		}
		if _, seen := commands[canonical]; !seen {
			order = append(order, canonical)
		}
		for _, cmd := range cmds {
			commands[canonical] = append(commands[canonical], emit.RewriteHookPath(cmd, target))
		}
	}

	emit.NoteCoverageGap(target, spec.KindHook, unmapped,
		"Cline reads a hook by file name, and only "+strings.Join(clineHookEvents, ", ")+" are names it looks for")
	emit.NoteFieldNoOp(target, spec.KindHook, "matcher", matchers,
		"a Cline hook file is per event with no matcher, so the script runs on every occurrence and has to filter itself")
	emit.NoteFieldNoOp(target, spec.KindHook, "timeout", timeouts,
		"Cline times hook subprocesses out on its own runtime setting, with nothing per hook file to set")
	emit.NoteSurfaceGap(target, spec.KindHook, inert, "the Cline hook runtime",
		"PreCompact is a file name Cline lists but maps to no runtime event today, so the script is discovered and never run")

	for _, event := range order {
		path := filepath.Join(dir, event+hookFileExt)
		body := emit.WithHeader(hookScript(commands[event]), emit.FormatShell)
		if err := sess.WriteExecutableFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return materializeHookScripts(sess, hooks, dryRun)
}

// hookScript renders the body of one event script. `set -e` makes the
// first failing command the script's exit status, which is what a
// blocking event such as PreToolUse reads to stop the tool call.
//
// Commands for the same event share one file because the file name is
// the event: Cline has no second slot to put them in. They also share
// one stdout, and `parseStdout` (subprocess-runner.ts:29-57) treats a
// non-empty stdout as control JSON, preferring the last
// `HOOK_CONTROL\t<json>` line when one is present. A hook that wants to
// print anything else should write to stderr.
func hookScript(commands []string) string {
	var b strings.Builder
	b.WriteString("set -e\n")
	for _, cmd := range commands {
		b.WriteString("\n")
		b.WriteString(cmd)
		b.WriteString("\n")
	}
	return b.String()
}

// materializeHookScripts copies each hook's stashed script body from
// `.agnostic-ai/scripts/` into `.cline/hooks/`. The lookup keys off the
// spec's original `command:` path, so a script imported via claude still
// materializes when the same hook syncs out to cline. A command that is
// a free-form shell expression carries no stashed body and skips.
//
// The copies sit beside the event scripts and are invisible to Cline's
// own discovery: `toHookConfigFileName` returns undefined for any stem
// that is not one of the ten event names, so a helper script is never
// mistaken for an event.
func materializeHookScripts(sess *emit.Session, hooks []spec.Entry, dryRun bool) error {
	for _, h := range hooks {
		for _, raw := range emit.HookCommands(h.Meta["command"]) {
			sourceTool, _ := emit.SourceToolFromHookCommand(raw)
			rewritten := emit.RewriteHookPath(raw, target)
			if err := sess.MaterializeHookScript(rewritten, target, sourceTool, dryRun); err != nil {
				return err
			}
		}
	}
	return nil
}
