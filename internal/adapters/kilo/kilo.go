// Package kilo emits configs for Kilo Code.
//
// Rules emit as one Markdown file per rule spec under `.kilo/rules/`
// (override via outputs.kilo.rules-dir), each one also listed as its
// own entry in `kilo.jsonc`'s `instructions` array (raw doc
// packages/kilo-docs/pages/customize/custom-rules.md: "Each entry
// points to a file path or glob pattern"). This adapter lists explicit
// per-file paths rather than a `.kilo/rules/*.md` directory glob: a
// rule's scope nests its file under `.kilo/rules/<scope>/`, and the
// vendor doc's own glob example is a single, non-recursive `*.md`, so
// a lone glob entry would silently miss anything scoped. Kilo Code's
// own precedence order is agent prompt > project `instructions` >
// AGENTS.md > global (agents-md.md), so `instructions` outranks the
// project-root AGENTS.md that `sync` writes centrally as a slim
// pointer to the source specs (one body shared with every other
// target's entry-point file). AGENTS.md "cannot be individually
// disabled: it is always loaded if present" (agents-md.md), so this
// adapter keeps inlining full rule bodies there too (see
// inlineRulesTargets in internal/adapters/internal/emit/
// rules_appendix.go): `instructions` is how a rule wins a conflict
// with a user's own entry, not a reason to drop the AGENTS.md fallback.
// `.kilocode/rules/` (the pre-rename Kilo Code branding) is a separate,
// genuinely legacy tree Kilo Code still reads automatically for
// backward compatibility; this adapter intentionally never emits it,
// and it is distinct from the current `.kilo/rules/` mechanism above
// (target-audit 2026-08-01, #535).
//
// Agents emit as one Markdown file per agent spec at
// `.kilo/agents/<name>.md` (override via outputs.kilo.agents-dir).
// Kilo Code takes the agent's name from the filename, not from
// frontmatter, so `name:` is never written. Frontmatter otherwise
// carries `description` (falls back to the spec name) plus `color`,
// `mode`, and `model` when the spec sets them. All three read from the
// plain top-level field: `color` is the same generic top-level key
// augment (augment.go:224) and qoder also promote, written through
// without per-target validation; the three targets document different
// value spaces (`color` support by target, docs/user/spec-format.md), and
// `mode` shares OpenCode's `primary`/`subagent`/`all` vocabulary under
// the identical name. Kilo Code's full agent Configuration Options table
// also documents `disable`, `hidden`, `steps`, `temperature`, and `top_p`
// (target-audit 2026-08-08, #562); none has a confirmed counterpart on
// another registered target, and `temperature`/`top_p` are
// provider-scaled tuning knobs besides, so all five stay reachable only
// through `x-kilo` (e.g. `x-kilo: {temperature: 0.1, steps: 15}`) rather
// than a generic top-level key. Every other arbitrary `x-kilo`
// key passes through the same way. `tools` is never written under any
// key, including `x-kilo`: Kilo Code's full agent option table has no
// `tools` field, so a spec's `tools` allowlist would be a silent no-op
// there, and the agent would keep its default (typically full)
// permissions while looking restricted. Kilo Code's real access control
// is a per-tool `permission` map (`allow` / `ask` / `deny`), but this
// adapter has no vendor-confirmed mapping from agnostic-ai's generic
// tool names onto Kilo's own tool identifiers, so it does not guess
// one. An author who needs per-tool restriction writes
// `x-kilo: {permission: {...}}` directly; an agent spec that sets
// `tools` instead surfaces a coverage note rather than silently
// dropping the restriction.
//
// Skills emit into the shared `.agents/skills/<name>/SKILL.md` tree
// (override via outputs.kilo.skills-dir): Kilo Code documents its own
// `.kilo/skills/` path, but also lists `.agents/skills/` as a
// compatibility directory "loaded by default" (target-audit
// 2026-08-01), and that is the same tree codex, amp, zed, crush,
// openhands, windsurf, and augment already write byte-identically, so
// pointing here dedupes instead of adding a second on-disk copy.
//
// MCP servers merge into the project `kilo.jsonc` (override via
// outputs.kilo.mcp-file) under an `mcp` map, the key current Kilo Code
// reads (`mcpServers` is the deprecated MCP-spec 2025-03-26 form).
// Stdio entries combine `command` + `args` into one `command` array
// and set `"type": "local"`; HTTP / SSE / remote entries render as
// `{"type": "remote", "url": ..., "headers": {...}}`. `environment`
// (not `env`) carries a stdio server's environment variables. A spec's
// `disabled: true` writes `"enabled": false`, the key Kilo Code's own
// documented MCP example carries; `disabled` itself is never written,
// and an enabled server (the common case) gets no explicit key at all.
// kilo.jsonc also holds user-managed keys (models, providers, ...);
// the merge only touches `mcp` and `instructions` and skips whichever
// of the two has nothing to contribute, so those survive a sync. This
// adapter writes plain JSON: JSONC is a superset of JSON, so every
// JSONC parser accepts the output, and agnostic-ai never needs to emit
// (or preserve) comments of its own.
//
// Kilo's docs also read from `.kilo/kilo.jsonc` when present, a second
// project-tier config file this adapter does not write. The vendor's
// documented 8-level config precedence
// (kilo.ai/docs/getting-started/settings#config-file-precedence) places
// `.kilo/` above project-root `kilo.jsonc` and describes higher levels
// as overriding lower ones, i.e. a merge, not an exclusive first-match
// read: an untouched key on the root file still reaches Kilo Code even
// when `.kilo/kilo.jsonc` exists. A hand-authored `.kilo/kilo.jsonc`
// that redeclares `mcp` or `instructions` would still shadow this
// adapter's output for those two keys specifically (target-audit
// 2026-08-27, #644).
package kilo

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target           = "kilo"
	defaultRulesDir  = ".kilo/rules"
	defaultAgentsDir = ".kilo/agents"
	defaultMCPFile   = "kilo.jsonc"
	// defaultSkillsDir is the shared cross-tool skills tree Kilo Code
	// scans by default (alongside its own .kilo/skills/); codex, amp,
	// zed, crush, openhands, windsurf, and augment already write here,
	// so identical skill folders dedupe under sync.shared-skills.
	defaultSkillsDir = ".agents/skills"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindRule, spec.KindAgent, spec.KindMCP, spec.KindSkill},
}

// Adapter emits Kilo Code configs.
type Adapter struct{}

// New returns a Kilo adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one Markdown file per rule under `.kilo/rules/`, one
// agent Markdown file per agent spec under `.kilo/agents/`, one shared
// `.agents/skills/<name>/SKILL.md` folder per skill, plus a merged
// `kilo.jsonc` carrying the `instructions` array (one entry per rule
// file) and the `mcp` map. The project-root AGENTS.md (still read, but
// lower priority than `instructions`; see the package doc) is written
// by `sync`, not here.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	rulesDir := emit.OutputRulesDir(cfg, target, defaultRulesDir)
	if err := sess.RulesDirectory(b, emit.RulesDirOpts{
		Dir:        rulesDir,
		SkipAgents: true,
		SkipSkills: true,
	}, dryRun); err != nil {
		return err
	}
	dir := emit.OutputAgentsDir(cfg, target, defaultAgentsDir)
	if err := emitAgents(sess, b.Agents, dir, dryRun); err != nil {
		return err
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	if err := sess.WriteSkillFolders(b.Skills, target, skillsDir, dryRun); err != nil {
		return err
	}
	return emitKiloJSONC(sess, b.Rules, rulesDir, b.MCPs, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// emitAgents writes one `<dir>/<name>.md` per agent spec. Agents whose
// spec declares `tools` get no restriction on Kilo Code (see
// agentMarkdown), so the whole batch surfaces one coverage note
// instead of a silent drop.
func emitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	withTools := 0
	for _, a := range agents {
		path := filepath.Join(dir, a.Name+".md")
		md, hadTools := agentMarkdown(a)
		if hadTools {
			withTools++
		}
		body := emit.WithHeader(md, emit.FormatMarkdown)
		if err := sess.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	emit.NoteCoverageGap(target, spec.KindAgent, withTools,
		"tools has no Kilo Code key; use x-kilo.permission for native per-tool access control")
	return nil
}

// agentMarkdown renders a single agent definition: `description`
// (falls back to the spec name) plus `color`, `mode`, and `model` when
// set, followed by arbitrary x-kilo passthrough and the spec body as
// the agent's system prompt. Kilo Code takes the agent name from the
// filename, so `name` is never written; `tools` is never written
// either (see the package doc). Both stay excluded from the x-kilo
// passthrough too, so an escape-hatch attempt cannot reintroduce a
// confirmed no-op key. `color` and `mode` are also excluded from the
// x-kilo passthrough below: ResolveMeta already flattens any
// `x-kilo.color` / `x-kilo.mode` onto `resolved` before this function
// runs, so the top-level loop above already carries an override
// through, and re-merging the same key from raw `e.Meta` would only be
// redundant, not additive.
// hadTools reports whether the spec declared a tools list, so the
// caller can fold it into one coverage note per sync instead of a
// silent drop.
func agentMarkdown(e spec.Entry) (body string, hadTools bool) {
	resolved := emit.ResolveMeta(e.Meta, target)
	desc, _ := resolved["description"].(string)
	if desc == "" {
		desc = e.Name
	}
	meta := map[string]any{
		"description": desc,
	}
	keys := []string{"description"}
	for _, k := range []string{"color", "mode", "model"} {
		if v, _ := resolved[k].(string); v != "" {
			meta[k] = v
			keys = append(keys, k)
		}
	}
	hadTools = len(emit.StringSlice(resolved["tools"])) > 0
	emit.MergeCustomTargetMeta(meta, &keys, e.Meta, target, "description", "color", "mode", "model", "name", "tools")
	front := emit.FrontmatterOrdered(meta, keys)
	trimmed := strings.TrimSpace(e.Body)
	if trimmed == "" {
		return front + "\n", hadTools
	}
	return front + "\n" + trimmed + "\n", hadTools
}

// emitKiloJSONC merges the `instructions` and `mcp` keys into
// kilo.jsonc in a single read-modify-write. Routes through
// emit.MergeJSONFile so any pre-existing user-managed keys (models,
// providers, ...) survive the sync. Each key is set only when its
// source list is non-empty, and no file is written at all when both
// are empty (or every MCP entry renders empty).
func emitKiloJSONC(sess *emit.Session, rules []spec.Entry, rulesDir string, mcps []spec.Entry, path string, dryRun bool) error {
	keys := map[string]any{}
	if instructions := ruleInstructions(rules, rulesDir); len(instructions) > 0 {
		keys["instructions"] = instructions
	}
	if servers := buildMCPMap(mcps); len(servers) > 0 {
		keys["mcp"] = servers
	}
	if len(keys) == 0 {
		return nil
	}
	return sess.MergeJSONFile(path, keys, dryRun)
}

// ruleInstructions returns one `instructions` entry per rule spec: the
// project-relative path RulesDirectory writes it to, scope subdirectory
// included (see the package doc for why this lists explicit paths
// rather than a `.kilo/rules/*.md` glob).
func ruleInstructions(rules []spec.Entry, rulesDir string) []string {
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		if r.Name == "" {
			continue
		}
		dir := rulesDir
		if s := r.EffectiveScope(); s != "" {
			dir = filepath.Join(rulesDir, s)
		}
		out = append(out, filepath.ToSlash(filepath.Join(dir, r.Name+".md")))
	}
	return out
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

// buildMCPEntry renders one kilo.jsonc `mcp` entry. Stdio specs
// combine `command` + `args` into a single `command` array and set
// `"type": "local"`; HTTP / SSE / remote specs set `"type": "remote"`
// with a `url`/`headers` block. `environment` (not `env`) carries a
// stdio server's environment variables. An entry missing its
// transport's required field (command for stdio, url for remote) is
// dropped: there is nothing for Kilo Code to run or connect to.
//
// A spec's `disabled: true` maps to Kilo Code's own `enabled: false`
// key (B9, target-audit 2026-08-01 follow-up): the documented MCP
// example carries `"enabled": true` alongside type/command/environment/
// timeout, so the vendor concept exists under that name. Kilo Code's
// own default (enabled) needs no explicit key, matching the codex
// adapter's identical convention for its `enabled` field.
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
		out["type"] = "local"
		out["command"] = combineCommand(cmd, e.Meta)
		if env := emit.StringMap(e.Meta["env"]); len(env) > 0 {
			out["environment"] = env
		}
	case "http", "sse", "remote":
		url, _ := e.Meta["url"].(string)
		if url == "" {
			return nil
		}
		out["type"] = "remote"
		out["url"] = url
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			out["headers"] = h
		}
	default:
		return nil
	}

	if disabled, _ := e.Meta["disabled"].(bool); disabled {
		out["enabled"] = false
	}

	return out
}

// combineCommand folds Kilo Code's expected `command: [cmd, arg1,
// ...]` array out of agnostic-ai's separate `command` + `args` fields.
func combineCommand(cmd string, meta map[string]any) []string {
	parts := []string{cmd}
	parts = append(parts, emit.StringSlice(meta["args"])...)
	return parts
}
