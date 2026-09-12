// Package crush emits configs for Charm's Crush CLI.
//
// The project-root AGENTS.md is written centrally by `sync` as a slim
// pointer to the source specs (one body shared with every other
// target's entry-point file). Crush reads that file natively for
// project instructions, and rules reach it exclusively through that
// shared entry-point: Crush has no per-rule directory, so this adapter
// never writes rules directly.
//
// Skills emit as one folder per skill under `.agents/skills/<name>/SKILL.md`,
// the first location Crush scans (ahead of `.crush/skills`,
// `.claude/skills`, and `.cursor/skills`). The renderer matches the
// codex/amp/zed output byte-for-byte so the shared tree dedupes. Setting
// `x-crush.user-invocable: true` on a skill adds that flag to its
// frontmatter so the skill also shows up in Crush's command palette
// (ctrl+p); the shared renderer passes any `x-crush` key through
// unchanged, so no crush-specific code path is needed.
//
// MCP servers are configured in the project `crush.json` under the
// `mcp` map: stdio entries render as `{"type": "stdio", "command":
// ..., "args": [...], "env": {...}}`; HTTP and remote entries render
// as `{"type": "http", "url": ..., ...}`; SSE entries keep their own
// `{"type": "sse", "url": ..., ...}` rather than collapsing into http.
// Crush's own config.go declares MCPSSE and MCPHttp as distinct enum
// values, and createTransport (internal/agent/tools/mcp/init.go) routes
// them to two different SDK transports (SSEClientTransport vs
// StreamableClientTransport), so an sse entry emitted as http fails to
// connect rather than merely being mislabelled. Both shapes also accept
// "headers": {...}, "oauth": ..., "oauth_client_id": ...,
// "oauth_client_secret": ..., and "oauth_callback_port": ... (oauth
// fields optional, shipped in Crush v0.87.0). Either transport also
// accepts "disabled": true, "sessionless": true, "enabled_tools":
// [...], and "disabled_tools": [...], all four read from the vendor's
// published schema.json rather than the README, which is the only place
// the MCP property set appears closed. crush.json also holds
// user-managed keys (models, providers, lsp, options); the merge only
// touches the `mcp` key so those survive a sync.
//
// Hooks merge into that same crush.json under a `hooks` map keyed by
// event name: `{"PreToolUse": [{"name": ..., "matcher": ..., "command":
// ..., "timeout": ...}, ...]}`. docs/hooks/README.md states "Crush
// currently supports just one hook, PreToolUse, with plans to support
// the full gamut" (re-verified 2026-09-10 against both that doc and
// the vendor's published schema.json, whose `$defs.HookConfig` still
// lists no other event; #629). A hook spec targeting any other event
// parses fine and would sit in the file unread, so it earns a coverage
// note instead. See hooks.go for the field mapping. mcp and hooks
// merge into crush.json in one MergeJSONFile call, not two:
// MergeJSONFile reads the on-disk file fresh on every call, and
// during sync's collision-detection capture pass writes never reach
// disk, so a second call would read the same pre-write file and
// produce a second, divergent snapshot for the same path from the
// same target (one carrying only `mcp`, the other only `hooks`),
// tripping the collision check as though two different targets
// disagreed on crush.json's content.
//
// crush.json is Crush's legacy format. The vendor's own docs call it
// deprecated and freeze it: "new configuration options will only be
// added to Bash-based config" (`crushrc`, a Bash script Crush sources
// on startup, now the documented primary format). This adapter still
// targets crush.json because crushrc needs its own design pass, not a
// drift fix: shell-quoting arbitrary header/URL values and choosing how
// a generated crushrc interacts with the merge below are open questions
// (#674). Practical effect: our MCP entries keep loading, since the
// vendor gives JSON no removal date and says it plans to keep it
// working, but a future crush-only MCP field ships Bash-only and has no
// path through this adapter. Discovery order matters too: on Unix,
// Crush reads `./.crushrc`, then `./crushrc`, then
// `$XDG_CONFIG_HOME/crush/crushrc`, merges every legacy `crush.json` /
// `.crush.json` found alongside those paths, with project settings
// overriding global ones and crushrc overriding JSON in the same
// directory, and logs a warning whenever a directory holds both. A
// project that also hand-authors a `crushrc` gets that warning against
// our crush.json on every launch; nothing on our side can suppress it.
//
// Ignore specs emit to project-root .crushignore through the shared protected
// writer (outputs.crush.ignore-file overrides the path). Crush v0.94.1
// documents gitignore syntax for this file; import crush reads it back.
package crush

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target           = "crush"
	defaultSkillsDir = ".agents/skills"
	defaultMCPFile   = "crush.json"
)

const defaultIgnoreFile = ".crushignore"

var caps = emit.Capabilities{
	Target: target,
	// KindRule is declared even though this adapter never writes rules
	// itself: they reach Crush through the shared AGENTS.md entry-point
	// sync writes centrally. KindAgent is absent; Crush has no agent
	// surface, so the unsupported warning is accurate.
	Supports: []spec.Kind{spec.KindSkill, spec.KindRule, spec.KindMCP, spec.KindHook, spec.KindIgnore},
}

// Adapter emits Crush configs.
type Adapter struct{}

// New returns a Crush adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one native skill folder per skill under .agents/skills/
// and a merged crush.json for MCP servers and PreToolUse hooks. The
// project-root AGENTS.md (with rule bodies inlined) is written by
// `sync`, not here.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := sess.WriteIgnoreFile(b.Ignores, target, emit.OutputIgnoreFile(cfg, target, defaultIgnoreFile), dryRun); err != nil {
		return err
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	if err := sess.WriteSkillFolders(b.Skills, target, skillsDir, dryRun); err != nil {
		return err
	}
	return emitCrushJSON(sess, b.MCPs, b.Hooks, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// emitCrushJSON writes (or merges into) crush.json with the `mcp` and
// `hooks` keys, in one MergeJSONFile call (see the package doc for why
// that must stay one call, not two). Routes through
// emit.MergeJSONFile so any pre-existing user-managed keys (models,
// providers, lsp, options, ...) survive the sync; only `mcp` and
// `hooks` are overwritten. No-op when both mcps and hooks render
// empty.
func emitCrushJSON(sess *emit.Session, mcps, hooks []spec.Entry, path string, dryRun bool) error {
	keys := map[string]any{}
	if servers := buildMCPMap(mcps); len(servers) > 0 {
		keys["mcp"] = servers
	}
	if hooksBlock := buildHooksBlock(hooks); hooksBlock != nil {
		keys["hooks"] = hooksBlock
	}
	if len(keys) == 0 {
		return nil
	}
	return sess.MergeJSONFile(path, keys, dryRun)
}

func buildMCPMap(mcps []spec.Entry) map[string]any {
	out := map[string]any{}
	for _, e := range mcps {
		if e.Name == "" {
			continue
		}
		entry := buildMCPEntry(e)
		if len(entry) == 0 {
			continue
		}
		out[e.Name] = entry
	}
	return out
}

// buildMCPEntry renders one Crush mcp entry. Stdio specs produce a
// command/args/env block tagged `type: "stdio"`. HTTP / SSE / remote
// specs produce a url/headers block; sse keeps its own `type: "sse"`
// tag since Crush routes it to a different transport, while http and
// remote (Crush has no "remote" MCPType of its own) both default to
// `type: "http"`.
//
// Every field here is mapped explicitly rather than merged from a
// generic `x-crush` block. Crush's schema.json sets
// `"additionalProperties": false` on `$defs.MCPConfig`, so a typo in a
// namespaced passthrough would produce a config Crush rejects outright
// rather than one it ignores. Skill frontmatter has no such constraint,
// which is why the shared skill renderer still takes the generic merge.
func buildMCPEntry(e spec.Entry) map[string]any {
	transport, _ := e.Meta["type"].(string)
	if transport == "" {
		transport = "stdio"
	}
	out := map[string]any{}

	switch transport {
	case "stdio":
		cmd, _ := e.Meta["command"].(string)
		if cmd == "" {
			return nil
		}
		out["type"] = "stdio"
		out["command"] = cmd
		if args := emit.StringSlice(e.Meta["args"]); len(args) > 0 {
			out["args"] = args
		}
		if env := emit.StringMap(e.Meta["env"]); len(env) > 0 {
			out["env"] = env
		}
	case "http", "sse", "remote":
		url, _ := e.Meta["url"].(string)
		if url == "" {
			return nil
		}
		// sse keeps its own type: Crush's config.go declares MCPSSE and
		// MCPHttp as distinct enum values, and createTransport routes
		// them to two different SDK transports (SSEClientTransport vs
		// StreamableClientTransport). remote has no native Crush
		// MCPType, so it defaults to http like the untyped case does.
		out["type"] = "http"
		if transport == "sse" {
			out["type"] = "sse"
		}
		out["url"] = url
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			out["headers"] = h
		}
		// OAuth fields, shipped in Crush v0.87.0 ("MCP OAuth
		// implementation"). Passed through verbatim; oauth is a plain
		// bool toggle, callback port keeps whatever numeric type the
		// spec loader decoded (JSON marshaling renders either the same).
		if v, ok := e.Meta["oauth"].(bool); ok {
			out["oauth"] = v
		}
		if v, _ := e.Meta["oauth_client_id"].(string); v != "" {
			out["oauth_client_id"] = v
		}
		if v, _ := e.Meta["oauth_client_secret"].(string); v != "" {
			out["oauth_client_secret"] = v
		}
		if v, ok := e.Meta["oauth_callback_port"]; ok && v != nil {
			out["oauth_callback_port"] = v
		}
	default:
		return nil
	}

	// disabled is Crush's own key, "Whether this MCP server is disabled"
	// (default false), and it was dropped silently until #641: a spec
	// carrying `disabled: true` synced to crush and trae printed a note
	// for trae only, so a user reasonably read the silence as crush
	// honoring the field. It did not.
	if disabled, _ := e.Meta["disabled"].(bool); disabled {
		out["disabled"] = true
	}
	// sessionless, enabled_tools and disabled_tools round out the
	// documented property set. schema.json: sessionless is "Mark a
	// sessionless MCP server (no Mcp-Session-Id) so Crush skips the
	// subscriptions/listen stream it would otherwise reject" (shipped
	// v0.91.2), enabled_tools is an "Allow list of tools from this MCP
	// server", disabled_tools a "List of tools from this MCP server to
	// disable". All three are properties of MCPConfig itself, not of a
	// transport variant, so none is gated on `type` (#634).
	if sessionless, _ := e.Meta["sessionless"].(bool); sessionless {
		out["sessionless"] = true
	}
	if enabled := emit.StringSlice(e.Meta["enabled_tools"]); len(enabled) > 0 {
		out["enabled_tools"] = enabled
	}
	if disabledTools := emit.StringSlice(e.Meta["disabled_tools"]); len(disabledTools) > 0 {
		out["disabled_tools"] = disabledTools
	}

	return out
}
