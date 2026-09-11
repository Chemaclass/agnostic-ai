// Package gemini emits Gemini CLI configs.
//
// The project-root `GEMINI.md` is written centrally by `sync` as a
// slim pointer to the source specs (one body shared with every other
// target's entry-point file). When `outputs.gemini.rules-file` is set,
// this adapter instead writes the legacy concatenated layout at that
// path so users on older workflows keep their behavior.
//
// Agents emit as one native subagent Markdown file per agent under
// `.gemini/agents/<name>.md`: "Custom agents are defined as Markdown
// files (`.md`) with YAML frontmatter ... Project-level:
// `.gemini/agents/*.md` (Shared with your team)"
// (geminicli.com/docs/core/subagents, cross-confirmed by `/agents
// reload`, which "Rescans agent directories (`~/.gemini/agents` and
// `.gemini/agents`)"; target-audit 2026-09-11, #733). That surface is
// what buys automatic delegation, an isolated context window, `@name`
// invocation, and the `/agents` listing. Up to this release agents
// emitted as a slash-command TOML instead, which made an agent a prompt
// the user had to type rather than a subagent Gemini could delegate to,
// and shared both a directory and a `<name>.toml` filename with
// commands, so a same-named agent and command overwrote each other.
// Set `outputs.gemini.emit-agents-as-commands: true` to keep writing
// that TOML alongside the native file, the same opt-in shape
// `emit-skills-as-commands` already has; off by default so the two
// surfaces do not carry the same agent twice and the filename collision
// stays closed. A managed TOML a prior sync left at the old path is
// swept for every current agent name when the key is off.
//
// Frontmatter carries the two required fields, `name` and `description`
// (the latter falling back to the spec name), plus `kind`, `model`,
// `temperature`, `max_turns`, and `timeout_mins` when declared. The body
// is the system prompt. `mcpServers` (inline per-agent MCP servers) is
// documented too and has no agnostic-ai spec equivalent, so it reaches
// the file through `x-gemini` like any other arbitrary key.
//
// `tools` is the one field that needs translating. Gemini names its own
// tools (`read_file`, `write_file`, `replace`, `glob`, `grep_search`,
// `run_shell_command`, `web_fetch`, `google_web_search`;
// geminicli.com/docs/reference/tools), which shares no spelling with
// agnostic-ai's Claude-style set, so this adapter maps the eight generic
// names onto them (geminiToolName in agents.go). A name outside that set
// is dropped rather than written unconfirmed and folds into one coverage
// note per sync: an unknown entry here would restrict the subagent to a
// tool that does not exist, while an absent `tools` key inherits every
// tool from the parent session, which is the safer of the two. Set
// `x-gemini.tools` to write Gemini's own vocabulary directly, including
// the documented `*`, `mcp_*`, and `mcp_<server>_*` wildcards; that
// override always wins outright over the translated form.
//
// Skills emit natively as one folder per skill under `.gemini/skills/<name>/`
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
// cross-tool MCP spec, not a gemini-only extension. Every mcpServers.<name>
// entry, regardless of transport, also accepts `timeout` (milliseconds),
// `trust` (bypass tool-call confirmations), `description`, `includeTools`,
// and `excludeTools`; all five pass through verbatim
// (geminicli.com/docs/reference/configuration.md, target-audit
// 2026-09-03, #661).
//
// Ignore specs emit as `.geminiignore` (override via
// outputs.gemini.ignore-file), gitignore syntax under a `#` provenance
// header: "Create a file named `.geminiignore` in the root of your
// project directory" (geminicli.com/docs/cli/gemini-ignore/,
// target-audit 2026-08-27, #625). `.aiexclude` belongs to Gemini Code
// Assist, a separate product, and appears nowhere in Gemini CLI's
// configuration reference, so the adapter sweeps the managed copy it
// wrote there up to v0.49.
package gemini

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target             = "gemini"
	defaultCommandsDir = ".gemini/commands"
	// defaultAgentsDir is Gemini CLI's project-level subagent
	// directory: "Project-level: `.gemini/agents/*.md` (Shared with
	// your team)" (geminicli.com/docs/core/subagents).
	defaultAgentsDir    = ".gemini/agents"
	defaultSkillsDir    = ".gemini/skills"
	defaultSettingsFile = ".gemini/settings.json"
	defaultIgnoreFile   = ".geminiignore"
	// legacyIgnoreFile is the ignore default up to v0.49. `.aiexclude` is
	// Gemini Code Assist's file, not Gemini CLI's, so every pattern we
	// wrote there went unread. Sync sweeps the managed copy after
	// emitting to the current default: an ignore file that looks
	// applied but is not hides secrets less well than none at all. The
	// sweep is skipped when the active path matches, i.e. the user
	// opted back in via `outputs.gemini.ignore-file`.
	legacyIgnoreFile    = ".aiexclude"
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

// Emit writes one native subagent per agent under `.gemini/agents/`
// (plus a command TOML per agent when opted in), one TOML per command
// under `.gemini/commands/`, one native skill folder per skill under
// `.gemini/skills/` (plus a TOML per skill when opted in),
// `.gemini/settings.json`, `.geminiignore`, and—when opted in via
// outputs.gemini.rules-file—a legacy concatenated rules document. The
// project-root GEMINI.md is written by `sync`, not here.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}

	commandsDir := emit.OutputCommandsDir(cfg, target, defaultCommandsDir)

	agentsDir := emit.OutputAgentsDir(cfg, target, defaultAgentsDir)
	if err := emitAgents(sess, b.Agents, agentsDir, dryRun); err != nil {
		return err
	}
	if err := emitAgentCommands(sess, b.Agents, commandsDir, cfg, dryRun); err != nil {
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
	ignoreFile := emit.OutputIgnoreFile(cfg, target, defaultIgnoreFile)
	if err := sess.WriteIgnoreFile(b.Ignores, target, ignoreFile, dryRun); err != nil {
		return err
	}
	if ignoreFile != legacyIgnoreFile {
		if err := sess.RemoveGenerated(legacyIgnoreFile, dryRun); err != nil {
			return err
		}
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
	// timeout, trust, description, includeTools, excludeTools are
	// documented on every mcpServers.<name> entry regardless of
	// transport (geminicli.com/docs/reference/configuration.md), so
	// they land here rather than in the transport-specific branches
	// above. See #661.
	if timeout, ok := emit.IntField(e.Meta, "timeout"); ok {
		out["timeout"] = timeout
	}
	if trust, _ := e.Meta["trust"].(bool); trust {
		out["trust"] = true
	}
	if desc, _ := e.Meta["description"].(string); desc != "" {
		out["description"] = desc
	}
	if include := emit.StringSlice(e.Meta["includeTools"]); len(include) > 0 {
		out["includeTools"] = include
	}
	if exclude := emit.StringSlice(e.Meta["excludeTools"]); len(exclude) > 0 {
		out["excludeTools"] = exclude
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

// emitAgentCommands writes the legacy `<dir>/<name>.toml` slash command
// per agent, the only agent surface this adapter had before agents moved
// to `.gemini/agents/` (#733). It is opt-in via
// `outputs.gemini.emit-agents-as-commands`, for a project that wants to
// keep typing `/name`; with the key off, a managed TOML a prior sync
// left at that path is swept instead. The sweep runs before
// emitCommands, so a command spec sharing the agent name still gets its
// own file written in the same run.
func emitAgentCommands(sess *emit.Session, agents []spec.Entry, dir string, cfg *config.Config, dryRun bool) error {
	asCommands := emit.EmitAgentsAsCommands(cfg, target)
	for _, a := range agents {
		path := filepath.Join(dir, a.Name+".toml")
		if !asCommands {
			if err := sess.RemoveGenerated(path, dryRun); err != nil {
				return err
			}
			continue
		}
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
