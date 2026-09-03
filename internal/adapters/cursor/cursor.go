// Package cursor emits Cursor editor configs.
//
// Rules emit as .cursor/rules/*.mdc with alwaysApply=true (frontmatter
// override honored). Agents emit natively as Cursor subagents at
// .cursor/agents/<name>.md (Cursor 2.4+). Skills emit natively as one
// folder per skill under .cursor/skills/<name>/SKILL.md (the Agent
// Skills layout Cursor 2.4+ discovers), including bundled asset files.
// Commands emit to .cursor/commands/<name>.md, Cursor's standard
// project commands location. Hooks land in .cursor/hooks.json, MCP
// servers in .cursor/mcp.json, Bugbot review guidance in
// .cursor/BUGBOT.md (root and per scope), background agent bootstrap in
// .cursor/environment.json, and ignore lists in .cursorignore.
//
// An MCP spec's `disabled: true` has no file-based equivalent here:
// cursor.com/docs/mcp documents no `disabled` (or `enabled`) key
// anywhere in its server schema, only a sidebar UI toggle. The emitter
// drops the field rather than write one Cursor ignores, and buffers a
// coverage note so the drop is loud, not silent.
//
// A stdio MCP server also accepts `envFile` (a path to an env file to
// load additional variables); a remote server (`url`) accepts a static
// `auth` object (`CLIENT_ID`, `CLIENT_SECRET`, `scopes`) for providers
// without OAuth Dynamic Client Registration. Neither is documented for
// the other targets sharing this adapter's MCP builder, so both are
// scoped to cursor.com/docs/mcp.md's own emission path (target-audit
// 2026-09-03, #661).
package cursor

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target             = "cursor"
	defaultDir         = ".cursor/rules"
	defaultExt         = ".mdc"
	defaultAgentsDir   = ".cursor/agents"
	defaultSkillsDir   = ".cursor/skills"
	defaultCommandsDir = ".cursor/commands"
	defaultMCPFile     = ".cursor/mcp.json"
	// defaultReviewFile is the Bugbot basename; each file lands inside a
	// `.cursor/` dir (root or per scope), the location Bugbot reads.
	defaultReviewFile  = "BUGBOT.md"
	defaultEnvironFile = ".cursor/environment.json"
	defaultIgnoreFile  = ".cursorignore"
	defaultHooksFile   = ".cursor/hooks.json"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindHook, spec.KindMCP, spec.KindCommand, spec.KindReview, spec.KindEnvironment, spec.KindIgnore},
}

// environRoutingKeys are the agnostic-ai spec fields stripped after
// emit.ResolveMeta before the remaining keys pass through to
// `.cursor/environment.json`. ResolveMeta already removes the target
// allow/deny keys and the x-<target> namespace; these three are the spec
// identity/scoping fields it leaves in place that Cursor has no use for.
var environRoutingKeys = map[string]struct{}{
	"name": {}, "scope": {}, "description": {},
}

// Adapter emits Cursor configs.
type Adapter struct{}

// New returns a Cursor adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one .mdc per rule, one native subagent per agent under
// `.cursor/agents/`, one native skill folder per skill under
// `.cursor/skills/`, one command per command spec under
// `.cursor/commands/`, plus an `.cursor/mcp.json` when MCP entries
// exist.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := sess.RulesDirectory(b, emit.RulesDirOpts{
		Dir: emit.OutputRulesDir(cfg, target, defaultDir),
		Ext: defaultExt,
		// Agents and skills emit natively under .cursor/agents/ and
		// .cursor/skills/; flattened .mdc copies would double-expose
		// each of them.
		SkipAgents: true,
		SkipSkills: true,
		FormatRule: func(e spec.Entry) string { return emit.WithHeader(mdc(e), emit.FormatMarkdown) },
	}, dryRun); err != nil {
		return err
	}
	agentsDir := emit.OutputAgentsDir(cfg, target, defaultAgentsDir)
	droppedTools := 0
	for _, a := range b.Agents {
		hadTools, err := emitAgent(sess, a, agentsDir, dryRun)
		if err != nil {
			return err
		}
		if hadTools {
			droppedTools++
		}
	}
	emit.NoteFieldNoOp(target, spec.KindAgent, "tools", droppedTools,
		"Cursor subagents have no tools field (name, description, model, readonly, is_background); use readonly: true for a coarse restriction")
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	for _, s := range b.Skills {
		if err := emitSkill(sess, s, skillsDir, dryRun); err != nil {
			return err
		}
	}
	if err := emitReviews(sess, b, cfg, dryRun); err != nil {
		return err
	}
	if err := emitEnvironment(sess, b, cfg, dryRun); err != nil {
		return err
	}
	if err := sess.WriteIgnoreFile(b.Ignores, emit.OutputIgnoreFile(cfg, target, defaultIgnoreFile), dryRun); err != nil {
		return err
	}
	commandsDir := emit.OutputCommandsDir(cfg, target, defaultCommandsDir)
	for _, c := range b.Commands {
		path := commandsDir + "/" + c.Name + ".md"
		if err := sess.WriteFile(path, emit.WithHeader(command(c), emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	if err := emitHooks(sess, b.HooksFor(target), cfg, dryRun); err != nil {
		return err
	}
	mcps := emit.StripMCPDisabled(target, b.MCPs, mcpDisabledNoOpReason)
	return sess.WriteMCPFile(mcps, emit.MCPSchemaServersMap, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun, emit.WithCursorMCPExtras())
}

// mcpDisabledNoOpReason explains, in the flushed coverage note, why
// `disabled: true` on an MCP spec never reaches `.cursor/mcp.json`:
// Cursor has no per-server disable key there. See the package doc
// comment for the vendor source.
const mcpDisabledNoOpReason = "no file-based way to pre-disable a project-scoped MCP server; use Cursor's own sidebar toggle instead"

// hooksDoc is the `.cursor/hooks.json` shape per the Cursor hooks docs:
// a numeric `version` plus a `hooks` map keyed by lifecycle event name,
// each event holding an array of `{command, matcher?}` entries.
type hooksDoc struct {
	Version int            `json:"version"`
	Hooks   map[string]any `json:"hooks"`
}

// emitHooks writes the managed `.cursor/hooks.json` from the hook specs
// scoped to cursor. The file is overwritten each sync; a no-op when no
// hooks resolve to output. The path is overridable via
// `outputs.cursor.hooks-file`. Hook scripts stashed under
// `.agnostic-ai/scripts/` materialize into `.cursor/hooks/` so the
// emitted `command:` paths resolve.
func emitHooks(sess *emit.Session, hooks []spec.Entry, cfg *config.Config, dryRun bool) error {
	byEvent := buildHooks(hooks)
	if len(byEvent) == 0 {
		return nil
	}
	raw, err := emit.MarshalJSONIndent(hooksDoc{Version: 1, Hooks: byEvent})
	if err != nil {
		return fmt.Errorf("cursor hooks: %w", err)
	}
	path := emit.OutputHooksFile(cfg, target, defaultHooksFile)
	if err := sess.WriteFile(path, string(raw)+"\n", dryRun); err != nil {
		return err
	}
	return materializeHookScripts(hooks, dryRun)
}

// buildHooks groups hook specs by their `event` frontmatter into Cursor's
// `hooks.<event> = [{command, matcher?}, ...]` shape. Cursor passes the
// event name through verbatim (no cross-tool translation), so a cursor
// hook spec sets `event:` to a Cursor lifecycle name (e.g.
// `beforeShellExecution`, `afterFileEdit`). `matcher` is omitted when
// absent. A `command:` list yields one entry per element. The optional
// Cursor entry fields `timeout` (seconds), `loop_limit`, and
// `failClosed` pass through from the same-named spec Meta keys.
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
			for _, k := range []string{"timeout", "loop_limit", "failClosed"} {
				if v, ok := h.Meta[k]; ok {
					entry[k] = v
				}
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

// materializeHookScripts copies each hook's stashed script body from
// `.agnostic-ai/scripts/` into `.cursor/hooks/` so the emitted
// hooks.json references a script that exists. Hooks whose command is a
// free-form shell expression carry no stashed body and skip silently.
func materializeHookScripts(hooks []spec.Entry, dryRun bool) error {
	for _, h := range hooks {
		for _, raw := range hookCommands(h.Meta["command"]) {
			sourceTool, _ := emit.SourceToolFromHookCommand(raw)
			rewritten := emit.RewriteHookPath(raw, target)
			if err := emit.MaterializeHookScript(rewritten, target, sourceTool, dryRun); err != nil {
				return err
			}
		}
	}
	return nil
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

// command renders a Cursor command file: Markdown whose body is the
// prompt the IDE sends when the user invokes `/name`. The docs describe
// commands as plain markdown; the optional `description`/`model`
// frontmatter is tolerated and kept for continuity.
func command(e spec.Entry) string {
	m := emit.ResolveMeta(e.Meta, target)
	desc, _ := m["description"].(string)
	model, _ := m["model"].(string)
	var b strings.Builder
	b.WriteString("---\n")
	if desc != "" {
		b.WriteString("description: " + desc + "\n")
	}
	if model != "" {
		b.WriteString("model: " + model + "\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(e.Body)
	return b.String()
}

// emitReviews writes Cursor Bugbot review guidance as a `BUGBOT.md` per
// scope, inside a `.cursor/` directory: Bugbot always includes the root
// `.cursor/BUGBOT.md` and any `<dir>/.cursor/BUGBOT.md` found while
// traversing upward from changed files. Review specs honor
// `EffectiveScope` the same way rules do: an unscoped spec lands at
// `.cursor/BUGBOT.md`, a spec under `reviews/backend/` lands at
// `backend/.cursor/BUGBOT.md`. Specs sharing a scope concatenate into
// that scope's single file. The basename is overridable via
// `outputs.cursor.review-file`.
func emitReviews(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if len(b.Reviews) == 0 {
		return nil
	}
	base := emit.OutputReviewFile(cfg, target, defaultReviewFile)
	byScope := map[string][]spec.Entry{}
	var scopeOrder []string
	for _, r := range b.Reviews {
		scope := r.EffectiveScope()
		if _, ok := byScope[scope]; !ok {
			scopeOrder = append(scopeOrder, scope)
		}
		byScope[scope] = append(byScope[scope], r)
	}
	for _, scope := range scopeOrder {
		if emit.ScopeEscapesRoot(scope) {
			// A frontmatter `scope: ../x` would anchor BUGBOT.md outside the
			// repo (review files sit at the project root, not under a tool
			// dir). Skip it rather than write beyond the project.
			continue
		}
		var sb strings.Builder
		for i, r := range byScope[scope] {
			if i > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(strings.TrimRight(r.Body, "\n"))
		}
		path := filepath.Join(scope, ".cursor", base)
		if err := sess.WriteFile(path, emit.WithHeader(sb.String()+"\n", emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	return nil
}

// emitEnvironment writes Cursor's background-agent bootstrap config to
// `.cursor/environment.json`. The spec body is the environment.json content:
// every key except the agnostic-ai routing fields passes through verbatim,
// so the author controls Cursor's schema (install, terminals, ...) while
// agnostic-ai single-sources it. Multiple environment specs merge by
// top-level key, last spec winning. The path is overridable via
// `outputs.cursor.environment-file`.
func emitEnvironment(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if len(b.Environments) == 0 {
		return nil
	}
	merged := map[string]any{}
	for _, e := range b.Environments {
		// Resolve the x-<target> namespace first so a cursor-specific
		// override wins and other targets' blocks are dropped, exactly like
		// every other adapter. Then strip the spec identity fields Cursor
		// has no schema for.
		for k, v := range emit.ResolveMeta(e.Meta, target) {
			if _, skip := environRoutingKeys[k]; skip {
				continue
			}
			merged[k] = v
		}
	}
	if len(merged) == 0 {
		return nil
	}
	raw, err := emit.MarshalJSONIndent(merged)
	if err != nil {
		return fmt.Errorf("cursor environment: %w", err)
	}
	path := emit.OutputEnvironmentFile(cfg, target, defaultEnvironFile)
	return sess.WriteFile(path, string(raw)+"\n", dryRun)
}

// mdc renders one rule as a `.mdc` file. Rules default to
// `alwaysApply: true`; the spec frontmatter overrides.
func mdc(e spec.Entry) string {
	m := emit.ResolveMeta(e.Meta, target)
	desc, _ := m["description"].(string)
	globs, _ := m["globs"].(string)
	always := true
	if v, ok := m["alwaysApply"].(bool); ok {
		always = v
	}
	// An alwaysApply:true rule ignores globs entirely, so synthesizing
	// one there is pure round-trip noise against a hand-authored source;
	// omit it (#443). An alwaysApply:false rule without globs falls back
	// to the Claude spelling (`paths`, a list, comma-joined), mirroring
	// the globs->paths translation on the claude side.
	if globs == "" && !always {
		globs = strings.Join(pathsToGlobs(m["paths"]), ",")
	}
	// cursor.com/docs/rules defines an explicit alwaysApply/description/
	// globs matrix: alwaysApply:false with a description and no globs is
	// "Agent reads the description and pulls the rule in when relevant"
	// (relevance-based selection), and alwaysApply:false with neither is
	// "Included only when you @-mention the rule in chat" (manual only).
	// Neither means "match every file". Synthesizing `globs: "**/*"` for
	// either turned it into an unconditional auto-attach, functionally
	// alwaysApply:true (#536); leave globs empty instead so Cursor picks
	// the mode that actually matches what the rule declares.
	var b strings.Builder
	b.WriteString("---\n")
	// Emit an empty description as a bare `description:` key (no trailing
	// space) so it byte-matches a hand-authored rule on round-trip (#443).
	if desc != "" {
		b.WriteString("description: " + desc + "\n")
	} else {
		b.WriteString("description:\n")
	}
	// Quote the globs value only when YAML needs it (a leading `*`, etc.)
	// rather than always double-quoting, which rewrote `apps/foo/**` to
	// `"apps/foo/**"` and broke byte round-trips (#443).
	if globs != "" {
		b.WriteString("globs: " + emit.YAMLScalar(globs) + "\n")
	}
	fmt.Fprintf(&b, "alwaysApply: %t\n", always)
	b.WriteString("---\n\n")
	b.WriteString(e.Body)
	return b.String()
}

// pathsToGlobs normalizes a `paths` value (the Claude spelling: a
// scalar string or a list) into a slice of glob strings. Returns nil
// when the key is absent or carries no usable value.
func pathsToGlobs(paths any) []string {
	switch v := paths.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
