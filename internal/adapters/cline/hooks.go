package cline

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// defaultHooksDir is where hook scripts land. Cline has two hook
// runtimes. The VS Code extension runs only `.clinerules/hooks/<Event>`,
// an executable file with no extension, through `/bin/sh` and so its
// shebang (apps/vscode/src/core/hooks/hook-factory.ts:1022-1033,
// HookProcess.ts:70-77). The SDK runtime the Cline CLI runs scans
// `.clinerules/hooks` and `.cline/hooks` for any `<Event>` file with an
// allowed extension, "" included (hook-file-config.ts:49-117), and runs
// all of them. One extensionless file in `.clinerules/hooks` is the only
// layout both runtimes run, and run once. Override with
// `outputs.cline.hooks-dir`.
const defaultHooksDir = ".clinerules/hooks"

// hookShebang is the event script's first line. The extension execs the
// file, and the SDK reads the shebang to pick bash (hook-file-hooks.ts:
// 290-354), so both run the commands under bash, as the SDK ran the
// `.sh` scripts sync wrote before. The provenance comment sits on the
// line below it, where header.Leads looks.
const hookShebang = "#!/usr/bin/env bash\n"

// legacyHooksDir and legacyHookExt are where releases before #1723 wrote
// each event script. The SDK runtime would run a leftover beside the new
// script, so a managed one is swept even when no ledger lists it, as in
// a fresh clone.
const (
	legacyHooksDir = ".cline/hooks"
	legacyHookExt  = ".sh"
)

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
// (hook-file-config.ts:45-47). A spec that writes `pretooluse` or
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
// (hook-file-config.ts:41) and `createHookCommandMap` skips every entry
// with no runtime event (hook-file-hooks.ts:404-406). The file name is
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
	for _, event := range clineHookEvents {
		if err := sess.RemoveGenerated(filepath.Join(legacyHooksDir, event+legacyHookExt), dryRun); err != nil {
			return err
		}
	}
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
			commands[canonical] = append(commands[canonical], emit.RewriteHookPath(cmd, target, h.Meta))
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
		path := filepath.Join(dir, event)
		body := hookShebang + emit.WithHeader(hookScript(commands[event]), emit.FormatShell)
		if err := sess.WriteExecutableFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return materializeHookScripts(sess, hooks, dryRun)
}

// hookScript renders the body of one event script. Cline never reads
// the exit code (hook-file-hooks.ts:415-454, and the VS Code extension
// fails open on any non-zero exit, HookProcess.ts:255-258): a hook blocks
// only by printing `{"cancel": true}` on stdout. So each command runs in
// a subshell, and an exit 2, the Claude Code convention for "block",
// becomes a last `HOOK_CONTROL\t{"cancel": true, ...}` line and exit 0,
// which both runtimes read as a block. Its stderr becomes the
// errorMessage. Any other failure exits with its own code, as `set -e`
// did, so the commands after it do not run.
//
// Commands for the same event share one file because the file name is
// the event: Cline has no second slot to put them in. They also share
// one stdout, and `parseStdout` (subprocess-runner.ts:68-97) treats a
// non-empty stdout as control JSON, preferring the last
// `HOOK_CONTROL\t<json>` line when one is present. A hook that wants to
// print anything else should write to stderr.
//
// The script owns the process every command runs in, so one export
// line hands the target to all of them and to any script they call.
func hookScript(commands []string) string {
	var b strings.Builder
	b.WriteString("set -e\n")
	b.WriteString("export " + emit.HookTargetEnv + "=" + target + "\n")
	b.WriteString(clineBlockPrelude)
	for _, cmd := range commands {
		b.WriteString("\nset +e\n(\nset -e\n")
		b.WriteString(cmd)
		b.WriteString("\n)" + clineBlockOnExit2)
	}
	return b.String()
}

// clineBlockPrelude sets up the stderr file and the JSON string escape
// every command's exit 2 check uses. The escape runs in the C locale, so
// bytes that are not UTF-8 pass through instead of stopping awk; it drops
// control bytes other than tab and newline, and writes `{` and `}` as
// \u escapes, since the VS Code extension's fallback parser counts braces
// without reading quotes (hook-factory.ts:366-466).
const clineBlockPrelude = `aai_err=$(mktemp)
trap 'rm -f "$aai_err"' EXIT
aai_json() { LC_ALL=C tr -d '\000-\010\013-\037\177' | LC_ALL=C awk 'BEGIN { ORS = "" } { gsub(/\\/, "\\\\"); gsub(/"/, "\\\""); gsub(/\t/, "\\t"); gsub(/{/, "\\u007b"); gsub(/}/, "\\u007d"); if (NR > 1) printf "\\n"; print }'; }
`

// clineBlockOnExit2 follows each command's subshell: it replays stderr,
// turns exit 2 into a cancel reply, and stops on any other failure. The
// subshell runs outside an && or || list, where bash would ignore
// `set -e` inside it.
const clineBlockOnExit2 = ` 2>"$aai_err"
aai_status=$?
set -e
cat "$aai_err" >&2
if [ "$aai_status" -eq 2 ]; then
  aai_msg=$(aai_json <"$aai_err") || aai_msg=
  printf 'HOOK_CONTROL\t{"cancel": true, "errorMessage": "%s"}\n' "${aai_msg:-blocked by a hook that exited 2}"
  exit 0
fi
[ "$aai_status" -eq 0 ] || exit "$aai_status"
`

// materializeHookScripts copies each hook's stashed script body from
// `.agnostic-ai/scripts/` into `.cline/hooks/`. The lookup keys off the
// spec's original `command:` path, so a script imported via claude still
// materializes when the same hook syncs out to cline. A command that is
// a free-form shell expression carries no stashed body and skips.
//
// The copies are invisible to Cline's own discovery:
// `toHookConfigFileName` returns undefined for any stem that is not one
// of the ten event names, so a helper script is never mistaken for an
// event.
func materializeHookScripts(sess *emit.Session, hooks []spec.Entry, dryRun bool) error {
	if err := sess.MaterializeNeutralHookScripts(hooks, target, emit.HookScriptsDir(target), dryRun); err != nil {
		return err
	}
	for _, h := range hooks {
		for _, raw := range emit.HookCommands(h.Meta["command"]) {
			raw = emit.RewriteNeutralHookPath(raw, ".")
			sourceTool, _ := emit.SourceToolFromHookCommand(raw)
			rewritten := emit.RewriteHookPath(raw, target, h.Meta)
			if err := sess.MaterializeHookScript(rewritten, target, sourceTool, dryRun); err != nil {
				return err
			}
		}
	}
	return nil
}
