// Package gemini emits Gemini CLI configs.
//
// The project-root `GEMINI.md` is written centrally by `sync` as a
// slim pointer to the source specs (one body shared with every other
// target's entry-point file). When `outputs.gemini.rules-file` is set,
// this adapter instead writes the legacy concatenated layout at that
// path so users on older workflows keep their behavior.
//
// Agents emit as one TOML per agent under `.gemini/commands/`. Skills
// emit natively as one folder per skill under `.gemini/skills/<name>/`
// (SKILL.md + bundled assets), the workspace tier Gemini CLI scans.
// Gemini CLI also scans the cross-tool `.agents/skills/` alias at the
// same tier, and within a tier that alias takes precedence over
// `.gemini/skills/` for a same-named skill
// (geminicli.com/docs/cli/skills/, target-audit 2026-08-08, #563).
// Gemini CLI resolves that itself at discovery time, so this adapter
// adds no conflict detection of its own. Setting
// `outputs.gemini.emit-skills-as-commands: true` additionally writes a
// TOML per skill with a `skill-` filename prefix.
//
// MCP stdio servers accept an optional `cwd` (working directory for the
// server process), the same field Codex documents; it is part of the
// cross-tool MCP spec, not a gemini-only extension.
package gemini

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target              = "gemini"
	defaultCommandsDir  = ".gemini/commands"
	defaultSkillsDir    = ".gemini/skills"
	defaultSettingsFile = ".gemini/settings.json"
	// .aiexclude is Gemini Code Assist's file, not Gemini CLI's; the CLI
	// reads .geminiignore (geminicli.com/docs/cli/gemini-ignore/).
	defaultIgnoreFile   = ".geminiignore"
	skillFilenamePrefix = "skill-"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindHook, spec.KindMCP, spec.KindCommand, spec.KindIgnore},
}

// Adapter emits Gemini CLI configs.
type Adapter struct{}

// New returns a Gemini adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one TOML per agent under `.gemini/commands/`, one native
// skill folder per skill under `.gemini/skills/` (plus a TOML per skill
// when opted in), `.gemini/settings.json`, and—when opted in via
// outputs.gemini.rules-file—a legacy concatenated rules document. The
// project-root GEMINI.md is written by `sync`, not here.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}

	commandsDir := emit.OutputCommandsDir(cfg, target, defaultCommandsDir)

	if err := emitAgentCommands(sess, b.Agents, commandsDir, dryRun); err != nil {
		return err
	}
	if err := emitCommands(sess, b.Commands, commandsDir, dryRun); err != nil {
		return err
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	if err := sess.WriteSkillFolders(b.Skills, target, skillsDir, dryRun); err != nil {
		return err
	}
	if emit.EmitSkillsAsCommands(cfg, target) {
		if err := emitSkillCommands(sess, b.Skills, commandsDir, dryRun); err != nil {
			return err
		}
	}
	if err := sess.EmitLegacyRulesFile(b, cfg, target, emit.MergedOpts{Title: "GEMINI.md"}, dryRun); err != nil {
		return err
	}
	if err := emitSettings(sess, b, emit.OutputMCPFile(cfg, target, defaultSettingsFile), dryRun); err != nil {
		return err
	}
	if err := sess.WriteIgnoreFile(b.Ignores, emit.OutputIgnoreFile(cfg, target, defaultIgnoreFile), dryRun); err != nil {
		return err
	}
	return materializeHookScripts(b.HooksFor(target), dryRun)
}

// materializeHookScripts copies each hook's stashed script body from
// `.agnostic-ai/scripts/` into `.gemini/hooks/` so the emitted
// settings.json has the actual script alongside the path it references.
func materializeHookScripts(hooks []spec.Entry, dryRun bool) error {
	for _, h := range hooks {
		cmds := hookCommands(h.Meta["command"])
		for _, raw := range cmds {
			sourceTool, _ := emit.SourceToolFromHookCommand(raw)
			rewritten := emit.RewriteHookPath(raw, target)
			if err := emit.MaterializeHookScript(rewritten, target, sourceTool, dryRun); err != nil {
				return err
			}
		}
	}
	return nil
}

// emitSettings writes (or merges into) .gemini/settings.json with the
// `mcpServers` and `hooks` keys. Routes through emit.MergeJSONFile so
// any user-managed Gemini settings survive the sync.
func emitSettings(sess *emit.Session, b spec.Bundle, path string, dryRun bool) error {
	keys := map[string]any{}
	if servers := buildMCPServers(b.MCPs); len(servers) > 0 {
		keys["mcpServers"] = servers
	}
	if hooks := buildHooks(b.HooksFor(target)); len(hooks) > 0 {
		keys["hooks"] = hooks
	}
	if len(keys) == 0 {
		return nil
	}
	return sess.MergeJSONFile(path, keys, dryRun)
}

// buildMCPServers renders Gemini-shaped MCP servers. Stdio specs emit
// {command, args, env}; HTTP / SSE specs emit {httpUrl, headers} -
// Gemini uses `httpUrl`, not the standard `url`.
func buildMCPServers(mcps []spec.Entry) map[string]any {
	out := map[string]any{}
	for _, e := range mcps {
		if e.Name == "" {
			continue
		}
		entry := buildMCPServer(e)
		if len(entry) == 0 {
			continue
		}
		out[e.Name] = entry
	}
	return out
}

func buildMCPServer(e spec.Entry) map[string]any {
	transport, _ := e.Meta["type"].(string)
	if transport == "" {
		transport = "stdio"
	}
	out := map[string]any{}
	switch transport {
	case "stdio":
		if cmd, _ := e.Meta["command"].(string); cmd != "" {
			out["command"] = cmd
		}
		if args := emit.StringSlice(e.Meta["args"]); len(args) > 0 {
			out["args"] = args
		}
		if cwd, _ := e.Meta["cwd"].(string); cwd != "" {
			out["cwd"] = cwd
		}
	case "http":
		// Gemini CLI uses `httpUrl` for the streamable-HTTP endpoint.
		if url, _ := e.Meta["url"].(string); url != "" {
			out["httpUrl"] = url
		}
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			out["headers"] = h
		}
	case "sse":
		// Gemini CLI uses `url` for the SSE endpoint (not `httpUrl`).
		if url, _ := e.Meta["url"].(string); url != "" {
			out["url"] = url
		}
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			out["headers"] = h
		}
	}
	if env := emit.StringMap(e.Meta["env"]); len(env) > 0 {
		out["env"] = env
	}
	return out
}

// buildHooks groups hook specs by their `event` frontmatter into the
// Gemini hooks shape: `hooks.<event> = [{matcher, command}, ...]`.
// `matcher` is omitted when absent so the hook fires unconditionally.
// A spec's `command:` field accepts a string or a list of strings; each
// list entry becomes one `{matcher, command}` pair under the same event.
func buildHooks(hooks []spec.Entry) map[string]any {
	byEvent := map[string][]map[string]any{}
	for _, h := range hooks {
		event, _ := h.Meta["event"].(string)
		if event == "" {
			continue
		}
		matcher, _ := h.Meta["matcher"].(string)
		cmds := hookCommands(h.Meta["command"])
		if len(cmds) == 0 {
			continue
		}
		for _, cmd := range cmds {
			entry := map[string]any{"command": emit.RewriteHookPath(cmd, target)}
			if matcher != "" {
				entry["matcher"] = matcher
			}
			byEvent[event] = append(byEvent[event], entry)
		}
	}
	out := map[string]any{}
	for k, v := range byEvent {
		out[k] = v
	}
	return out
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

func emitAgentCommands(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	for _, a := range agents {
		path := filepath.Join(dir, a.Name+".toml")
		body := emit.HeaderBlock(emit.FormatTOML) + commandTOML(a)
		if err := sess.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// emitCommands writes one TOML per command spec under
// `.gemini/commands/`. Commands are the native slash-prompt surface, so
// they emit with their bare name (no `skill-` prefix).
func emitCommands(sess *emit.Session, commands []spec.Entry, dir string, dryRun bool) error {
	for _, c := range commands {
		path := filepath.Join(dir, c.Name+".toml")
		body := emit.HeaderBlock(emit.FormatTOML) + commandTOML(c)
		if err := sess.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

func emitSkillCommands(sess *emit.Session, skills []spec.Entry, dir string, dryRun bool) error {
	for _, s := range skills {
		path := filepath.Join(dir, skillFilenamePrefix+s.Name+".toml")
		body := emit.HeaderBlock(emit.FormatTOML) + commandTOML(s)
		if err := sess.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// commandTOML renders one slash-command TOML. Schema:
//
//	description = "<spec description>"
//	<custom x-gemini scalar/array keys>
//	prompt = """
//	<spec body>
//	"""
//
// Gemini commands have no other native frontmatter surface, so arbitrary
// keys declared under `x-gemini` are emitted verbatim between description
// and prompt (sorted, target-scoped). See #367.
func commandTOML(e spec.Entry) string {
	// Resolve through the per-target meta so an x-gemini.description wins
	// over the (claude-side) top-level value, matching codex skillMarkdown.
	// Without this the override is silently dropped (CustomTargetMeta below
	// excludes description from the pass-through keys).
	resolved := emit.ResolveMeta(e.Meta, target)
	desc, _ := resolved["description"].(string)
	var sb strings.Builder
	if desc != "" {
		emit.WriteTOMLString(&sb, "description", desc)
	}
	if cm, keys := emit.CustomTargetMeta(e.Meta, target, "description", "prompt"); cm != nil {
		for _, k := range keys {
			emit.WriteTOMLValue(&sb, k, cm[k])
		}
	}
	body := strings.TrimSpace(e.Body)
	if body == "" {
		body = desc
	}
	emit.WriteTOMLMultiline(&sb, "prompt", body)
	return sb.String()
}
