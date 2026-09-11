// Package kiro emits steering files, native agent profiles, native
// skill folders, hook definitions, and MCP config for AWS Kiro.
//
// Kiro loads Markdown steering documents from `.kiro/steering/`. Every
// file starts with a YAML frontmatter block (it must be the first
// content in the file, no blank line before it) whose `inclusion` key
// picks one of three loading modes this adapter uses:
//
//   - `always`: loaded on every interaction. Used for rules with no
//     glob or scope to target.
//   - `fileMatch` (+ `fileMatchPattern`): loaded when the active file
//     matches the pattern. Used for rules that carry `globs` or a
//     source-layout scope.
//
// (Kiro also documents `auto` and `manual` steering modes; this
// adapter has no rule shape that needs either.)
//
// Skills are a native Kiro surface too, not a steering-file convention:
// one folder per skill at `.kiro/skills/<name>/SKILL.md`
// (kiro.dev/docs/skills/: "Workspace skills (`.kiro/skills/`)", loaded
// by glob at `skill://.kiro/skills/*/SKILL.md`), the standard Agent
// Skills layout (`name` + `description` frontmatter) this adapter
// shares byte-for-byte with the `.agents/skills/` render ten other
// targets already produce (emit.WriteSkillFolders). Bundled sibling
// assets (`scripts/`, `references/`, `assets/`) copy alongside
// SKILL.md, so a skill package keeps progressive disclosure and
// slash-command invocation instead of losing them. A prior version of
// this adapter flattened skills into `.kiro/steering/skill-<name>.md`
// with `inclusion: auto`, which dropped bundled assets entirely and
// never reached Kiro's own skill picker (#642); sync sweeps a stale
// file of that shape left behind for a current skill name, the same
// convention agents already use below.
//
// Agents are a native Kiro surface, not a steering-file convention: one
// YAML-frontmatter Markdown file per agent at `.kiro/agents/<name>.md`,
// the tree Kiro's own agent picker reads (kiro.dev/docs/custom-agents/:
// "Workspace-level: `.kiro/agents/` ... Configuration lives in YAML
// frontmatter, your system prompt is the document body"). `description`
// (falls back to the agent's name) and `model` pass through; the full
// documented field set also includes `tools`, `mcpServers`,
// `permissions`, `hooks`, `keyboardShortcut`, and `welcomeMessage`
// (kiro.dev/docs/custom-agents/configuration-reference/), of which only
// `tools` has an agnostic-ai spec equivalent.
//
// That page documents Kiro's own `tools` vocabulary in full: category
// tags (`read`, `write`, `shell`, `web`, `subagent`, `knowledge`,
// `todo_list`), `@server_name` / `@server_name/tool_name` for one or
// all tools from a specific MCP server, `@mcp` for every MCP tool
// across servers, `@builtin` for every built-in tool, and `*` for
// everything. A second page (kiro.dev/docs/tools/, updated 2026-08-21,
// seventeen days after configuration-reference's own 2026-08-04 date)
// tables the same field differently: `read`, `write`, `shell`, `web`,
// `subagent`, `spec`, `context`, where `context` bundles
// `disclose_context`, `introspect`, and `knowledge`; `todo_list` is
// gone and `knowledge` no longer stands alone. The two pages disagree
// and neither states which one the shipping product follows, so this
// adapter keeps citing configuration-reference rather than guessing;
// it does not matter functionally, since both pages agree on the four
// categories this adapter actually emits. This adapter translates
// agnostic-ai's Claude-style names onto that vocabulary
// (kiroToolCategory): `Read`, `Grep`, and `Glob` collapse onto `read`;
// `Write` and `Edit` onto `write`; `Bash` onto `shell`; `WebFetch` and
// `WebSearch` onto `web`, deduplicated so several Claude-style names
// sharing a category emit that tag once.
// Kiro's built-in-tools catalog (kiro.dev/docs/tools/) documents each
// category as a bundle, not a single tool: `write` covers `fs_write`,
// `fs_append`, `str_replace`, and `delete_file`, so an agent declaring
// only `Edit` also gains delete capability on Kiro; `web` covers both
// `web_fetch` and `web_search`, so `WebFetch` alone also grants search.
// No finer Kiro category avoids this short of emitting Kiro's internal
// per-tool identifiers instead of its documented category vocabulary,
// which would also give up the vendor's stated guarantee that a
// category picks up new tools shipped under it automatically. A name
// with no table entry (anything outside agnostic-ai's
// Read/Write/Edit/Bash/Grep/Glob/WebFetch/WebSearch set) is never
// guessed at or written verbatim; it drops from the emitted list and
// folds into one coverage note per sync, while any name in the same
// list that does translate still emits.
//
// `name` is never written: it is absent from Kiro's own field list, so
// identity comes from the filename, the same convention every
// per-agent surface with no documented `name` key uses. Arbitrary
// `x-kiro` keys pass through verbatim, and `x-kiro.tools` always wins
// outright over the translated form (never merged alongside it), so an
// author who already knows Kiro's own vocabulary can bypass the table
// entirely or set `mcpServers`, `permissions`, `hooks`,
// `keyboardShortcut`, or `welcomeMessage` directly. A prior version of
// this adapter flattened agents into `.kiro/steering/agent-<name>.md`
// with `inclusion: manual`; that path never reached Kiro's agent picker
// and dropped every field steering has no key for, so this adapter now
// sweeps any such file a prior sync left behind for a current agent
// name.
//
// Hooks are also native: one JSON file per hook spec at
// `.kiro/hooks/<name>.json` (kiro.dev/docs/hooks/: "Hooks are JSON
// files stored in `.kiro/hooks/` at the workspace level"), each holding
// `{"version": "v1", "hooks": [...]}`. A hook entry carries `name`,
// `trigger` (the spec's `event`, passed through verbatim like every
// other adapter's hook event), an optional `matcher`, an `action`
// object, an optional `timeout`, and the spec's generic `description`
// field (docs/user/spec-format.md: "Free-form documentation"; the
// vendor field reference lists the matching `hooks[].description` as
// "Documentation only"). A spec's `command:` (string or list) always
// renders `action: {"type": "command", "command": ...}`; a list
// produces one entry per command in the same file, `name` suffixed
// `-2`, `-3`, ... to stay unique. `disabled: true` on the spec writes
// `"enabled": false` (the vendor default, enabled, needs no explicit
// key), mirroring the `disabled`/`enabled` convention already used for
// MCP entries. Every entry marshals from a `map[string]any`, not a
// fixed struct, so arbitrary `x-kiro` keys pass through verbatim
// (emit.MergeCustomTargetMeta): `confirm` (the vendor's Stop-hook
// confirmation block: "Ask for confirmation before a Stop command hook
// runs", taking `question`, `options` (`id`/`label`/`run` each), and an
// optional `confirmCommand`) has no agnostic-ai spec equivalent and so
// is only reachable this way, and `x-kiro.action` can set the
// documented `{"type": "agent", "prompt": ...}` shape this adapter
// never emits by hand (agnostic-ai's hook spec has no generic prompt
// field). Before #642, `hookEntry` was a fixed Go struct: `description`
// and `confirm` were unreachable at any layer, including x-kiro,
// because a struct cannot marshal a key it does not declare. Unlike
// Claude Code, Codex, Gemini, and Cursor, this adapter does not
// materialize stashed hook scripts from `.agnostic-ai/scripts/` into
// `.kiro/hooks/`: that directory is where Kiro looks for hook
// definitions, and there is no vendor confirmation that a plain script
// file living alongside them is safe.
//
// MCP servers write to `.kiro/settings/mcp.json` as a `mcpServers` map.
// A local server carries `command` plus optional `args` and `env`; a
// remote server carries `url` plus optional `headers` and `env`, and
// a `type` discriminant so the transport is never guessed. Both
// transports also carry `description` and `roots` when the spec sets
// them, plus `disabled` (Kiro's own key, honored as written, unlike
// Claude Code and Cursor which have no file-based equivalent) and
// `autoApprove` / `disabledTools` tool lists. kiro.dev's own local- and
// remote-server field tables document `disabled`, `autoApprove`, and
// `disabledTools`, but list neither `description` nor `roots`, so this
// adapter writes both unconfirmed, the same as every other target
// sharing this builder. A remote server additionally carries
// `oauth` (`clientId`, `clientSecret`, `redirectUri`,
// `clientMetadataUrl`, `oauthScopes`) and the top-level `oauthScopes`
// fallback; an explicitly empty `oauthScopes: []` emits as written,
// since kiro.dev/docs/mcp/configuration/ makes that the documented
// remedy for OAuth scope errors (target-audit 2026-08-27, #634).
//
// Ignore specs emit as `.kiroignore` (override via
// outputs.kiro.ignore-file), gitignore syntax under a `#` provenance
// header: "To exclude files in a specific project, create a
// `.kiroignore` file in your project root (or any subdirectory) and add
// patterns for files you want to exclude" and "`.kiroignore` uses
// standard gitignore syntax" (kiro.dev/docs/kiroignore/, target-audit
// 2026-09-11). Two vendor caveats gate how far the file reaches, and
// neither is something an emitter can set: the IDE reads ignore
// filenames from its own `kiroAgent.agentIgnoreFiles` setting, so
// `.kiroignore` has to be in that array before the IDE honors it, and
// CLI V3 applies it to content- and filename-search results only rather
// than across every agent tool.
//
// The root `AGENTS.md` entry-point (which Kiro reads directly and
// always includes) is written centrally by `sync`, not by this
// adapter.
package kiro

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target             = "kiro"
	defaultSteeringDir = ".kiro/steering"
	defaultAgentsDir   = ".kiro/agents"
	defaultSkillsDir   = ".kiro/skills"
	defaultHooksDir    = ".kiro/hooks"
	defaultMCPFile     = ".kiro/settings/mcp.json"
	defaultIgnoreFile  = ".kiroignore"
	// legacyAgentPrefix names the flattened steering file this adapter
	// used to write per agent before agents moved to their native
	// `.kiro/agents/` surface (see the package doc). Kept only so
	// emitAgents can sweep away a stale file of this shape left behind
	// by an older sync.
	legacyAgentPrefix = "agent-"
	// legacySkillPrefix names the flattened steering file this adapter
	// used to write per skill before skills moved to their native
	// `.kiro/skills/` surface (see the package doc). Kept only so
	// emitSkills can sweep away a stale file of this shape left behind
	// by an older sync.
	legacySkillPrefix = "skill-"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindMCP, spec.KindHook, spec.KindIgnore},
}

// Adapter emits AWS Kiro configs.
type Adapter struct{}

// New returns a Kiro adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one steering file per rule, one native agent profile per
// agent, one native skill folder per skill, one hook definition file
// per hook, `.kiroignore` when ignore entries exist, plus
// `.kiro/settings/mcp.json` when MCP entries exist.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	dir := emit.OutputRulesDir(cfg, target, defaultSteeringDir)
	if err := emitRules(sess, b.Rules, dir, dryRun); err != nil {
		return err
	}
	agentsDir := emit.OutputAgentsDir(cfg, target, defaultAgentsDir)
	if err := emitAgents(sess, b.Agents, agentsDir, dir, dryRun); err != nil {
		return err
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	if err := emitSkills(sess, b.Skills, skillsDir, dir, dryRun); err != nil {
		return err
	}
	hooksDir := emit.OutputHooksDir(cfg, target, defaultHooksDir)
	if err := emitHooks(sess, b.HooksFor(target), hooksDir, dryRun); err != nil {
		return err
	}
	if err := sess.WriteIgnoreFile(b.Ignores, emit.OutputIgnoreFile(cfg, target, defaultIgnoreFile), dryRun); err != nil {
		return err
	}
	return sess.WriteMCPFile(b.MCPs, emit.MCPSchemaServersMap,
		emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun, emit.WithKiroMCPExtras())
}

// emitRules writes one `<dir>/<name>.md` per rule. Rules that target a
// glob or a source-layout scope render `inclusion: fileMatch`;
// everything else renders `inclusion: always`.
func emitRules(sess *emit.Session, rules []spec.Entry, dir string, dryRun bool) error {
	for _, r := range rules {
		path := filepath.Join(dir, r.Name+".md")
		body := emit.WithHeader(renderRule(r), emit.FormatMarkdown)
		if err := sess.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// emitAgents writes one native `<agentsDir>/<name>.md` per agent (see
// agentMarkdown) and sweeps the legacy flattened steering file at
// `<steeringDir>/agent-<name>.md` a prior sync may have left behind for
// the same name. A generic `tools` value translates onto Kiro's own
// category vocabulary (see the package doc and translateTools); any
// name with no table entry is dropped from the emitted list and folded
// into one coverage note per sync instead of vanishing silently.
func emitAgents(sess *emit.Session, agents []spec.Entry, agentsDir, steeringDir string, dryRun bool) error {
	unmappedTools := 0
	for _, a := range agents {
		path := filepath.Join(agentsDir, a.Name+".md")
		md, hasUnmapped := agentMarkdown(a)
		if hasUnmapped {
			unmappedTools++
		}
		if err := sess.WriteFile(path, emit.WithHeader(md, emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
		legacy := filepath.Join(steeringDir, legacyAgentPrefix+a.Name+".md")
		if err := sess.RemoveGenerated(legacy, dryRun); err != nil {
			return err
		}
	}
	emit.NoteFieldNoOp(target, spec.KindAgent, "tools", unmappedTools,
		"value(s) outside agnostic-ai's Read/Write/Edit/Bash/Grep/Glob/WebFetch/WebSearch set have no confirmed Kiro category; set x-kiro.tools directly for those")
	return nil
}

// emitSkills writes the standard Agent Skills folder layout for every
// skill into skillsDir (see the package doc and emit.WriteSkillFolders):
// one `<skillsDir>/<name>/SKILL.md` plus every sibling asset propagated
// byte-for-byte. It then sweeps the legacy flattened steering file at
// `<steeringDir>/skill-<name>.md` a prior sync may have left behind for
// the same name, mirroring emitAgents' sweep of its own legacy path.
func emitSkills(sess *emit.Session, skills []spec.Entry, skillsDir, steeringDir string, dryRun bool) error {
	if err := sess.WriteSkillFolders(skills, target, skillsDir, dryRun); err != nil {
		return err
	}
	for _, s := range skills {
		legacy := filepath.Join(steeringDir, legacySkillPrefix+s.Name+".md")
		if err := sess.RemoveGenerated(legacy, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// renderRule renders a rule's steering-file body: frontmatter first,
// then a blank line, then the spec body.
func renderRule(e spec.Entry) string {
	front, keys := ruleFrontmatter(e)
	return withFrontmatter(front, keys, e.Body)
}

// ruleFrontmatter picks `inclusion: fileMatch` (with `fileMatchPattern`)
// for a rule that targets a glob or a source-layout scope, otherwise
// `inclusion: always`.
func ruleFrontmatter(e spec.Entry) (map[string]any, []string) {
	if pattern := fileMatchPatternFor(e); pattern != "" {
		return map[string]any{
			"inclusion":        "fileMatch",
			"fileMatchPattern": pattern,
		}, []string{"inclusion", "fileMatchPattern"}
	}
	return map[string]any{"inclusion": "always"}, []string{"inclusion"}
}

// fileMatchPatternFor returns the fileMatchPattern glob for a rule.
// Explicit `globs` (resolved for target-specific overrides) wins;
// otherwise the source-layout scope (e.g. `rules/backend/auth.md` ->
// "backend/**"); otherwise "" (the rule has nothing to scope to and
// loads always).
func fileMatchPatternFor(e spec.Entry) string {
	m := emit.ResolveMeta(e.Meta, target)
	if g, _ := m["globs"].(string); g != "" {
		return g
	}
	if s := e.EffectiveScope(); s != "" {
		return s + "/**"
	}
	return ""
}

// kiroToolCategory maps agnostic-ai's Claude-style tool identifiers onto
// Kiro's own `tools` category tags (kiro.dev/docs/custom-agents/configuration-reference/,
// kiro.dev/docs/tools/). Several Claude-style names collapse onto the
// same Kiro category because Kiro's category granularity is coarser
// than agnostic-ai's; see the package doc for what each category
// bundles and which of these mappings widen access beyond what a single
// Claude-style name implies on its own.
var kiroToolCategory = map[string]string{
	"Read":      "read",
	"Grep":      "read",
	"Glob":      "read",
	"Write":     "write",
	"Edit":      "write",
	"Bash":      "shell",
	"WebFetch":  "web",
	"WebSearch": "web",
}

// translateTools maps a spec's generic Claude-style tools list onto
// Kiro's own category vocabulary (kiroToolCategory), deduplicated in
// first-seen order since several Claude-style names collapse onto the
// same category. A name with no table entry is left out of mapped and
// reported via hasUnmapped instead of being written verbatim or dropped
// with no trace, so the caller can surface it.
func translateTools(names []string) (mapped []string, hasUnmapped bool) {
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		cat, ok := kiroToolCategory[n]
		if !ok {
			hasUnmapped = true
			continue
		}
		if seen[cat] {
			continue
		}
		seen[cat] = true
		mapped = append(mapped, cat)
	}
	return mapped, hasUnmapped
}

// agentMarkdown renders a single `.kiro/agents/<name>.md` file:
// `description` (falls back to the spec name), optional `model`, and
// `tools` translated onto Kiro's own category vocabulary (see
// translateTools and the package doc), plus arbitrary x-kiro passthrough
// (mcpServers, permissions, hooks, keyboardShortcut, welcomeMessage, or
// an explicit tools override already in Kiro's own vocabulary), followed
// by the spec body as the agent's system prompt. Kiro's agent schema has
// no `name` key, so identity comes from the filename; `name` and `model`
// are excluded from the x-kiro passthrough merge below only because they
// are already handled by hand above (excluding them here just prevents
// emitting the same key twice, not a ban on x-kiro overriding model:
// ResolveMeta already flattens x-kiro.model onto the value this function
// reads). `tools` is deliberately read from the raw, unresolved meta
// rather than the resolved map: ResolveMeta would already have flattened
// an x-kiro.tools override onto it, and running that value back through
// the Claude-style translation table would misread Kiro's own vocabulary
// as unmapped. xKiroSetsTools guards the same case explicitly: when it
// is set, the generic `tools` field is left untranslated entirely and
// x-kiro.tools reaches the frontmatter only through the passthrough
// merge below, so the override always wins outright instead of merging
// alongside a translated value. hasUnmappedTools reports whether
// translateTools left at least one declared name unmapped, so the
// caller can fold every such agent into one coverage note per sync;
// names that do translate still emit even when others in the same list
// do not.
func agentMarkdown(a spec.Entry) (body string, hasUnmappedTools bool) {
	resolved := emit.ResolveMeta(a.Meta, target)
	desc, _ := resolved["description"].(string)
	if desc == "" {
		desc = a.Name
	}
	meta := map[string]any{"description": desc}
	keys := []string{"description"}
	if model, _ := resolved["model"].(string); model != "" {
		meta["model"] = model
		keys = append(keys, "model")
	}
	if !xKiroSetsTools(a.Meta) {
		if raw := emit.StringSlice(a.Meta["tools"]); len(raw) > 0 {
			mapped, unmapped := translateTools(raw)
			if len(mapped) > 0 {
				meta["tools"] = mapped
				keys = append(keys, "tools")
			}
			hasUnmappedTools = unmapped
		}
	}
	emit.MergeCustomTargetMeta(meta, &keys, a.Meta, target, "name", "description", "model")
	front := emit.FrontmatterOrdered(meta, keys)
	trimmed := strings.TrimSpace(a.Body)
	if trimmed == "" {
		return front + "\n", hasUnmappedTools
	}
	return front + "\n" + trimmed + "\n", hasUnmappedTools
}

// xKiroSetsTools reports whether the spec already carries an explicit
// x-kiro.tools override: the one channel this adapter trusts to already
// be Kiro's own tool vocabulary rather than agnostic-ai's generic
// Claude-style names.
func xKiroSetsTools(meta map[string]any) bool {
	x, _ := emit.CustomTargetMeta(meta, target)
	if x == nil {
		return false
	}
	_, tools := x["tools"]
	return tools
}

// withFrontmatter joins a rendered frontmatter block with body,
// separated by a blank line, in the shape Kiro requires: frontmatter
// as the very first bytes of the file.
func withFrontmatter(front map[string]any, keys []string, body string) string {
	var b strings.Builder
	b.WriteString(emit.FrontmatterOrdered(front, keys))
	b.WriteString("\n")
	b.WriteString(body)
	return b.String()
}
