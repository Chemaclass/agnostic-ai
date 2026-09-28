package claude

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

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
	noteOtherEnvironmentKeys(envs)
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
		if autoVerify != nil {
			emit.NoteFieldNoOp(target, spec.KindEnvironment, "autoVerify", 1,
				"launch.json is written only for dev-commands")
		}
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
	if port, ok := launchPort(m["port"]); ok {
		out["port"] = port
	}
	if cwd, _ := m["cwd"].(string); cwd != "" {
		out["cwd"] = cwd
	}
	if env := launchEnv(m["env"]); len(env) > 0 {
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

// launchPort reads a port written as a number or a numeric string.
func launchPort(v any) (int, bool) {
	if s, ok := v.(string); ok {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		return n, err == nil
	}
	return emit.IntField(map[string]any{"port": v}, "port")
}

// launchEnv reads env values as strings. YAML reads `PORT: 3000` as a
// number and `DEBUG: true` as a bool; both are still environment values.
func launchEnv(v any) map[string]string {
	m, _ := v.(map[string]any)
	out := map[string]string{}
	for k, val := range m {
		switch val.(type) {
		case string, int, int64, float64, bool:
			out[k] = fmt.Sprint(val)
		}
	}
	return out
}

// launchSpecKeys are the environment spec keys the Claude emit reads or
// notes itself, plus the spec's identity fields.
var launchSpecKeys = map[string]bool{
	"name": true, "description": true, "scope": true, "dev-commands": true, "autoVerify": true,
	"install": true, "setup": true, "setup-windows": true, "terminals": true,
}

// noteOtherEnvironmentKeys notes each other environment key, such as a
// Cursor environment.json key passed through at the top level: Claude
// Code has no file for it.
func noteOtherEnvironmentKeys(envs []spec.Entry) {
	counts := map[string]int{}
	for _, e := range envs {
		for k := range emit.ResolveMeta(e.Meta, target) {
			if !launchSpecKeys[k] {
				counts[k]++
			}
		}
	}
	for _, k := range slices.Sorted(maps.Keys(counts)) {
		emit.NoteFieldNoOp(target, spec.KindEnvironment, k, counts[k], "Claude Code has no file for it")
	}
}
