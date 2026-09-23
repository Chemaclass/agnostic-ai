// Package openhands emits configs for All Hands' OpenHands agent.
//
// OpenHands recommends skills as one folder per skill under
// `.agents/skills/<name>/SKILL.md`, the same shared cross-tool tree
// codex, amp, zed, and crush already emit into; the renderer matches
// that output byte-for-byte so the shared tree dedupes.
//
// Local conversations discover project agents from flat files at
// `.agents/agents/<name>.md`. Goose reads the same primary path, so
// both adapters share one renderer for name, description, model, and
// prompt body. Portable tool names are omitted with a coverage note
// because OpenHands uses its own file_editor/terminal vocabulary.
//
// The portable `color` field is omitted with a coverage note too, for
// a different reason. OpenHands does document it ("Rich color name
// (e.g., `"blue"`, `"green"`) used by visualizers to style this
// agent's output in terminal panels",
// docs.openhands.dev/sdk/guides/agent-file-based), but Goose's own
// frontmatter table for the same tree is `name`, `description`, and
// `model` only, and its shipping loader's AgentMetadata struct carries
// exactly those three fields
// (crates/goose/src/agents/platform_extensions/summon.rs). Writing
// `color` in the shared renderer would leave a key Goose reads into
// nothing; writing it for openhands alone would break the
// byte-identity the shared path depends on and trip collision
// detection for anyone syncing both. `x-openhands.color` reaches the
// file for an author who wants it on this target only, and suppresses
// the note (target-audit 2026-09-18, #864).
//
// The project-root AGENTS.md is written centrally by `sync` as a slim
// pointer to the source specs (one body shared with every other
// target's entry-point file). OpenHands reads that file natively for
// project instructions, and an always-on rule (no `paths`/`globs`
// value and no source-layout or frontmatter scope) reaches it
// exclusively through that shared entry-point.
//
// A rule that carries one of those instead emits natively as a
// path-triggered rule: `<skillsDir>/<name>/SKILL.md` with a `paths:`
// frontmatter list, OpenHands' own deterministic per-file mechanism
// (docs.openhands.dev/overview/skills/path): "guaranteed to load for
// the files they scope, with no reliance on the model choosing them",
// and "zero baseline cost" to the context window until a matching file
// is touched. The vendor's own example places this content in a flat
// `.md` file; this adapter writes the folder form instead, the second
// documented location, which shares `.agents/skills/` with regular
// skills rather than needing a separate `outputs.openhands.rules-dir`
// key. See path_rules.go for the glob-resolution and rendering.
//

// MCP servers merge into `./config.toml` (override via
// outputs.openhands.mcp-file) under a `[mcp]` table with three arrays:
// `stdio_servers` (`[[mcp.stdio_servers]]` tables carrying `name`,
// `command`, `args`, `env`), and `sse_servers` / `shttp_servers`
// (`shttp_servers` is OpenHands' streamable-HTTP transport, the
// cross-tool spec's `type: http`). Each sse/shttp element is a bare URL
// string, or the vendor's documented `{ url, api_key, timeout }` object
// once the entry sets a top-level `api_key` and/or (shttp only) a
// `timeout` meta field; OpenHands has no header-map field for these, so
// a spec's `headers` value never reaches it and surfaces a coverage
// note instead of vanishing silently, and `timeout` on an sse entry
// gets the same treatment since the vendor documents it for shttp only
// (#588). Read config_toml.go's serverValue for the exact mapping this
// defends. OpenHands has no `type` field of its own; transport is
// implied by which array a server lands in. The TOML rendering reuses
// the same field writers Codex's `[mcp_servers.<name>]` tables use (see
// internal/adapters/internal/emit/toml.go) rather than a second
// hand-rolled copy.
//
// An environment spec's `install` field writes `.openhands/setup.sh`
// (override via outputs.openhands.setup-file), the vendor's documented
// project bootstrap script: "You can add a `.openhands/setup.sh` file,
// which will run every time OpenHands begins working with your
// repository" (docs.openhands.dev/openhands/usage/customization/repository).
// Hooks emit to `.openhands/hooks.json`, configured "per-repository"
// and honored "across Cloud, CLI, and local GUI setups". The vendor's
// native layout uses snake_case event keys with no wrapper, and
// documents the Claude shape as equally supported: "PascalCase event
// keys (e.g., `PreToolUse`) and the `{"hooks": {...}}` wrapper are both
// supported, so you can share hook scripts between the two tools"
// (docs.openhands.dev/openhands/usage/customization/hooks). This
// adapter emits that shared shape, so one renderer serves both targets.
// Six events: PreToolUse, PostToolUse, UserPromptSubmit, Stop,
// SessionStart, SessionEnd. A matcher only applies to the two ToolUse
// events, and OpenHands names its own tools, so a Claude-style matcher
// such as `Bash` parses and then matches nothing; that case surfaces a
// coverage note rather than a guessed rename. See hooks.go and #629.
//
// `terminals` has no OpenHands surface (the script runs once,
// synchronously, not as a set of long-running processes) and surfaces
// a coverage note. See setup_script.go for the full mapping and the
// merge policy across multiple environment specs.
package openhands

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target           = "openhands"
	defaultAgentsDir = ".agents/agents"
	defaultSkillsDir = ".agents/skills"
	defaultMCPFile   = "config.toml"
	defaultSetupFile = ".openhands/setup.sh"
)

var caps = emit.Capabilities{
	Target: target,
	// KindRule covers two paths: an always-on rule reaches OpenHands
	// only through the shared AGENTS.md entry-point sync writes
	// centrally, while a path-triggered rule (paths/globs/scope) writes
	// its own skill folder directly (see path_rules.go).
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindHook, spec.KindMCP, spec.KindEnvironment},
	AgentFieldReasons: map[string]string{
		"mcpServers": "OpenHands takes inline server definitions only; set x-openhands.mcp_servers",
	},
}

// Adapter emits OpenHands configs.
type Adapter struct{}

// New returns an OpenHands adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

func (Adapter) Capabilities() []spec.Kind { return caps.Supports }

// Emit writes one native skill folder per skill under .agents/skills/,
// one path-triggered-rule skill folder per scoped rule under the same
// directory, plus a merged `./config.toml` for MCP servers and
// `.openhands/setup.sh` for environment specs. The project-root
// AGENTS.md (with always-on rule bodies inlined) is written by `sync`,
// not here.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	agentsDir := emit.OutputAgentsDir(cfg, target, defaultAgentsDir)
	if err := (Adapter{}).EmitAgents(sess, b.Agents, agentsDir, dryRun); err != nil {
		return err
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	if err := sess.WriteSkillFolders(b.Skills, target, skillsDir, dryRun); err != nil {
		return err
	}
	if err := emitPathTriggeredRules(sess, b.Rules, skillsDir, dryRun); err != nil {
		return err
	}
	if err := emitSetupScript(sess, b.Environments, cfg, dryRun); err != nil {
		return err
	}
	if err := emitHooks(sess, b.Hooks, cfg, dryRun); err != nil {
		return err
	}
	return emitMCPConfig(sess, b.MCPs, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// EmitAgents writes native agent profiles without other project outputs.
func (Adapter) EmitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	if err := sess.WriteSharedAgentFiles(agents, target, dir, dryRun); err != nil {
		return err
	}
	noteDroppedAgentTools(agents)
	noteDroppedAgentColor(agents)
	return nil
}

func noteDroppedAgentTools(agents []spec.Entry) {
	dropped := 0
	for _, agent := range agents {
		if emit.SharedAgentToolsDropped(agent, target) {
			dropped++
		}
	}
	emit.NoteFieldNoOp(target, spec.KindAgent, "tools", dropped,
		"OpenHands project agents use the file_editor and terminal tool vocabulary")
}

// noteDroppedAgentColor reports the portable `color` field OpenHands
// does document for a file-based agent but this adapter does not write.
// `.agents/agents/<name>.md` is shared with Goose byte-for-byte, and
// Goose's own frontmatter is `name`, `description`, and `model` only,
// so a `color` key there would be inert in every Goose agent file and
// would split the two renderers apart. x-openhands.color reaches the
// file for an author who wants it on this target alone.
func noteDroppedAgentColor(agents []spec.Entry) {
	dropped := 0
	for _, agent := range agents {
		if emit.SharedAgentFieldDropped(agent, target, "color") {
			dropped++
		}
	}
	emit.NoteFieldNoOp(target, spec.KindAgent, "color", dropped,
		"`.agents/agents/<name>.md` is shared byte-for-byte with Goose, whose frontmatter has no color key; set x-openhands.color to emit it for openhands only")
}

// emitMCPConfig sorts mcps into OpenHands' three [mcp] arrays, surfaces
// a coverage note for any transport that maps to none of them (rather
// than guessing a bucket), for any remote entry whose `headers` cannot
// reach OpenHands' single-`api_key` credential field, and for any sse
// entry whose `timeout` has no effect there (shttp-only), and writes
// the merged TOML when at least one server rendered.
func emitMCPConfig(sess *emit.Session, mcps []spec.Entry, path string, dryRun bool) error {
	stdio, sse, shttp, unmapped, headersNoOp, timeoutNoOp := mcpTransportBuckets(mcps)
	emit.NoteCoverageGap(target, spec.KindMCP, unmapped,
		"no OpenHands [mcp] array for this transport")
	emit.NoteFieldNoOp(target, spec.KindMCP, "headers", headersNoOp,
		"OpenHands sse/shttp servers take a single api_key string, not a headers map; set meta.api_key directly")
	emit.NoteFieldNoOp(target, spec.KindMCP, "timeout", timeoutNoOp,
		"OpenHands documents timeout for shttp_servers only, not sse_servers")
	doc := renderConfigTOML(stdio, sse, shttp)
	if doc == "" {
		return nil
	}
	return sess.WriteFile(path, doc, dryRun)
}
