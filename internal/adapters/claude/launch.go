package claude

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// defaultLaunchFile is where Claude Code Desktop reads its preview servers
// (code.claude.com/docs/en/desktop#configure-preview-servers).
const defaultLaunchFile = ".claude/launch.json"

// launchFileVersion is the `version` Claude Code writes in launch.json.
const launchFileVersion = "0.0.1"

// environmentFieldsWithoutEffect are the environment spec fields Claude
// Code has no project file for, with the reason each note gives.
var environmentFieldsWithoutEffect = []struct{ field, reason string }{
	{"install", "Claude Code has no install step; run it from a SessionStart hook spec"},
	{"setup", "Claude Code runs worktree setup from a WorktreeCreate or SessionStart hook spec"},
	{"terminals", "Claude Code has no terminal list; use dev-commands for preview servers"},
}

// emitLaunch writes the environment specs' dev-commands to
// .claude/launch.json as preview server configurations. As with every
// environment field, the last spec that sets dev-commands wins.
func emitLaunch(sess *emit.Session, envs []spec.Entry, dryRun bool) error {
	for _, f := range environmentFieldsWithoutEffect {
		count := emit.EnvironmentsWithField(target, envs, f.field)
		if f.field == "setup" {
			count = emit.EnvironmentsWithSetup(target, envs)
		}
		emit.NoteFieldNoOp(target, spec.KindEnvironment, f.field, count, f.reason)
	}
	var commands []any
	var autoVerify any
	for _, e := range envs {
		m := emit.ResolveMeta(e.Meta, target)
		if v, ok := m["dev-commands"]; ok {
			commands, _ = v.([]any)
		}
		if v, ok := m["autoVerify"].(bool); ok {
			autoVerify = v
		}
	}
	var configurations []any
	for _, c := range commands {
		if conf := launchConfiguration(c); conf != nil {
			configurations = append(configurations, conf)
		}
	}
	if len(configurations) == 0 {
		return nil
	}
	doc := map[string]any{"version": launchFileVersion, "configurations": configurations}
	// `x-claude.autoVerify` sets launch.json's own switch for Claude's
	// automatic checks after an edit.
	if autoVerify != nil {
		doc["autoVerify"] = autoVerify
	}
	raw, err := emit.MarshalJSONIndent(doc)
	if err != nil {
		return fmt.Errorf("claude launch: %w", err)
	}
	return sess.WriteFile(defaultLaunchFile, string(raw)+"\n", dryRun)
}

// launchConfiguration renders one dev command as a launch.json
// configuration, or nil when it has no name or command.
func launchConfiguration(v any) map[string]any {
	m, _ := v.(map[string]any)
	name, _ := m["name"].(string)
	argv := emit.CommandArgv(m["command"])
	if name == "" || len(argv) == 0 {
		return nil
	}
	out := map[string]any{"name": name, "runtimeExecutable": argv[0]}
	if len(argv) > 1 {
		out["runtimeArgs"] = argv[1:]
	}
	if port, ok := emit.IntField(m, "port"); ok {
		out["port"] = port
	}
	if cwd, _ := m["cwd"].(string); cwd != "" {
		out["cwd"] = cwd
	}
	if env := emit.StringMap(m["env"]); len(env) > 0 {
		out["env"] = env
	}
	if auto, ok := m["auto-port"].(bool); ok {
		out["autoPort"] = auto
	}
	if url, _ := m["url"].(string); url != "" {
		out["url"] = url
	}
	return out
}
