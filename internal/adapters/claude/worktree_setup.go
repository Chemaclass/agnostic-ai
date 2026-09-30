package claude

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/claudehooks"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// worktreeSetupMarker is the file the setup script leaves in a linked
// worktree's own git directory once setup succeeds. Git removes it with
// the worktree, and it never shows as an untracked file, which would make
// Claude Code ask before removing a clean worktree.
const worktreeSetupMarker = "agnostic-ai-worktree-setup"

// worktreeSetupEvents are the hooks that run the setup script. Claude
// Code has no setup step for a worktree it creates with git, and a
// WorktreeCreate hook replaces the creation itself
// (code.claude.com/docs/en/hooks#worktreecreate). SessionStart `startup`
// covers `--worktree` and Desktop sessions; SessionStart does not fire
// for a subagent, so SubagentStart covers `isolation: worktree`, whose
// payload `cwd` is the worktree root
// (code.claude.com/docs/en/hooks#subagentstart).
var worktreeSetupEvents = []struct{ event, matcher string }{
	{"SessionStart", "startup"},
	{"SubagentStart", ""},
}

// environmentSetup returns the setup commands the last environment spec
// that sets `setup` for claude declares, one per line. `x-claude.setup:
// false` resolves to no commands.
func environmentSetup(envs []spec.Entry) string {
	setup := ""
	for _, e := range envs {
		v, ok := emit.ResolveMeta(e.Meta, target)["setup"]
		if !ok {
			continue
		}
		if s, isString := v.(string); isString {
			setup = strings.TrimRight(s, "\n")
			continue
		}
		setup = strings.Join(emit.StringSlice(v), "\n")
	}
	return setup
}

// emitWorktreeSetup writes the script that runs the environment specs'
// setup and returns the hooks that start it, or nil when no spec sets
// setup for claude.
func emitWorktreeSetup(sess *emit.Session, envs []spec.Entry, dir string, dryRun bool) ([]spec.Entry, error) {
	setup := environmentSetup(envs)
	if setup == "" {
		return nil, nil
	}
	path := filepath.Join(dir, "hooks", claudehooks.WorktreeSetupScript)
	if err := sess.WriteExecutableFile(path, worktreeSetupScript(setup), dryRun); err != nil {
		return nil, err
	}
	command := `sh "$CLAUDE_PROJECT_DIR/` + filepath.ToSlash(path) + `"`
	if filepath.IsAbs(path) {
		command = "sh " + emit.ShellQuote(path)
	}
	hooks := make([]spec.Entry, 0, len(worktreeSetupEvents))
	for _, h := range worktreeSetupEvents {
		hooks = append(hooks, spec.Entry{
			Kind: spec.KindHook,
			Name: "worktree-setup",
			Meta: map[string]any{"event": h.event, "matcher": h.matcher, "command": command},
		})
	}
	return hooks, nil
}

// worktreeSetupScript renders the hook script. It reads the worktree from
// the payload's `cwd`, since `$CLAUDE_PROJECT_DIR` stays at the directory
// the session started in. It runs setup only in a linked worktree, once,
// and sends setup output to stderr because Claude Code adds a
// SessionStart hook's stdout to the context. Setup runs in a subshell as
// a plain command: `set -e` has no effect inside a `( ) &&` list, and an
// `exit` in setup ends only the subshell. A failed setup leaves no
// marker, so the next session retries it. The target check skips tools
// that also read `.claude/settings.json` hooks, such as Cursor, which run
// setup from their own file.
func worktreeSetupScript(setup string) string {
	var sb strings.Builder
	sb.WriteString("#!/bin/sh\n")
	sb.WriteString(emit.HeaderBlock(emit.FormatShell))
	sb.WriteString(`[ "${` + emit.HookTargetEnv + `:-}" = ` + target + ` ] || exit 0
dir=$(sed -n 's/.*"cwd"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | sed 's/\\\\/\\/g' | head -n 1)
cd "${dir:-.}" 2>/dev/null || exit 0
root=$(git rev-parse --show-toplevel 2>/dev/null) || exit 0
git_dir=$(cd "$(git rev-parse --git-dir)" && pwd -P) || exit 0
common_dir=$(cd "$(git rev-parse --git-common-dir)" && pwd -P) || exit 0
[ "$git_dir" != "$common_dir" ] || exit 0
marker="$git_dir/` + worktreeSetupMarker + `"
[ ! -e "$marker" ] || exit 0
cd "$root" || exit 1
exec >&2
(
set -e
`)
	sb.WriteString(setup)
	sb.WriteString("\n)\nstatus=$?\n[ \"$status\" -eq 0 ] || exit \"$status\"\n: > \"$marker\"\n")
	return sb.String()
}
