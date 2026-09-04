// Package qoder emits .qoder/rules/*.md and .qoder/agents/*.md for
// Alibaba's Qoder IDE.
//
// Qoder reads project rules from `.qoder/rules/*.md`: one file per
// rule, each carrying a name and description, and Qoder's own docs
// state this native rules content takes precedence over `AGENTS.md`
// when both are present. Qoder also reads the cross-tool root
// `AGENTS.md`, which is written centrally by `sync` as a slim pointer
// to the source specs, not by this adapter.
//
// Agents emit as one Markdown file per agent spec at
// `.qoder/agents/<name>.md` (override via outputs.qoder.agents-dir):
// docs.qoder.com/extensions/subagent documents `name` and
// `description` as required frontmatter, plus optional `model`,
// `tools`, `skills`, and `mcpServers`. docs.qoder.com/cli/subagent's
// field reference additionally documents `color` (one of eight named
// values, e.g. `red`, `cyan`, shown while the Subagent runs in the
// TUI); the smaller `/extensions/subagent` page omits it but defers to
// the CLI page as "the complete guide" for the identical
// `.qoder/agents/<name>.md` path, so this adapter emits `color` too
// when a spec sets it, the same shared portable field augment and kilo
// already promote (#588). `tools` renders as a
// comma-separated string (`tools: Read, Grep, Bash`). docs.qoder.com/
// cli/subagent documents two other forms too, a YAML inline array
// (`tools: [Read, Grep, Bash]`) and a YAML block list, so the
// comma-separated form is this adapter's choice among three valid
// options, not the only one the vendor documents (target-audit
// 2026-08-08, #563). Qoder's built-in tool vocabulary is Claude-style
// (`Bash`, `Edit`, `Write`, `Glob`, `Grep`, `Read`, `WebFetch`,
// `WebSearch`), so agnostic-ai's generic `tools` list passes through
// safely here: joined into the comma form on emit and split back into a
// list on `import qoder`. That is the opposite of kilo and augment,
// whose vendor tool vocabularies differ from agnostic-ai's and which
// drop a generic `tools` list with a coverage note instead of guessing
// a translation; qoder is the one target where passthrough is confirmed
// safe.
//
// Skills emit into their own native folder tree at
// `.qoder/skills/<name>/SKILL.md` (override via
// outputs.qoder.skills-dir): docs.qoder.com/extensions/skills documents
// exactly this path for project scope, plus a user-level
// `~/.qoder/skills/{skill-name}/SKILL.md` this adapter has no reach
// into, states "Each Skill contains a `SKILL.md` file", and gives plain
// `name` + `description` frontmatter (target-audit 2026-08-08, #558).
// That doc does not list `.agents/skills/` as a compatible path, unlike
// kilo, augment, and openhands, so this is Qoder's own tree, not a
// dedupe target for the shared one.
//
// MCP servers merge into `.qoder/settings.json` (override via
// outputs.qoder.mcp-file) under a root `mcpServers` map. See mcp.go for
// the emitted field set.
//
// That file replaced the project-root `.mcp.json` in #641. Both are
// documented project-level locations on
// docs.qoder.com/cli/mcp-reference's own scope table, but `.mcp.json`
// is the identical literal path Claude Code writes, and the two
// adapters only shared it while their emitted bytes stayed identical.
// They no longer can: Qoder documents nine per-server fields Claude
// Code does not (`trust`, `includeTools`, `excludeTools`,
// `alwaysAllow`, a working `disabled`, and more), Claude Code documents
// four Qoder does not (`headersHelper`, `alwaysLoad`, and its own
// `oauth` and `timeout` shapes), and a spec using any of them would
// have made two adapters write different bytes to one path. That trips
// sync's collision check as a hard error, on a pair of targets both in
// the default set. Moving to Qoder's own file removes the shared path
// instead of arbitrating it.
//
// One migration note: a `.mcp.json` left behind by an older
// agnostic-ai still loads, and the vendor's precedence order puts it
// ahead of `.qoder/settings.json` for a same-named server. Sync cannot
// sweep it, since a JSON file carries no provenance header and the file
// may well belong to Claude Code. A Qoder-only project should delete it
// by hand once.
package qoder

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target           = "qoder"
	defaultDir       = ".qoder/rules"
	defaultAgentsDir = ".qoder/agents"
	defaultSkillsDir = ".qoder/skills"
	defaultMCPFile   = ".qoder/settings.json"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindRule, spec.KindAgent, spec.KindSkill, spec.KindMCP},
}

// Adapter emits Qoder configs.
type Adapter struct{}

// New returns a Qoder adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one .md per rule into the rules directory (default
// `.qoder/rules`), one .md per agent into the agents directory (default
// `.qoder/agents`), one folder per skill into the skills directory
// (default `.qoder/skills`, Qoder's native Agent Skills layout; a flat
// file there never loads as a skill), plus a merged
// `.qoder/settings.json` for MCP servers.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := sess.RulesDirectory(b, emit.RulesDirOpts{
		Dir:        emit.OutputRulesDir(cfg, target, defaultDir),
		SkipAgents: true,
		SkipSkills: true,
	}, dryRun); err != nil {
		return err
	}
	agentsDir := emit.OutputAgentsDir(cfg, target, defaultAgentsDir)
	if err := emitAgents(sess, b.Agents, agentsDir, dryRun); err != nil {
		return err
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	if err := sess.WriteSkillFolders(b.Skills, target, skillsDir, dryRun); err != nil {
		return err
	}
	return emitMCP(sess, b.MCPs, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// emitAgents writes one `<dir>/<name>.md` per agent spec.
func emitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	for _, a := range agents {
		path := filepath.Join(dir, a.Name+".md")
		body := emit.WithHeader(agentMarkdown(a), emit.FormatMarkdown)
		if err := sess.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// agentMarkdown renders one `.qoder/agents/<name>.md` file. `name`
// (from the spec name) and `description` (falls back to the spec name)
// are Qoder's required frontmatter keys; `model`, `tools`, `color`,
// `skills`, and `mcpServers` are its documented optional ones (see the
// package doc). `tools` joins agnostic-ai's generic list into Qoder's
// comma-separated string form; a spec that already wrote a plain
// string (e.g. via x-qoder) passes through unchanged. `color` is a
// shared portable concept augment.go and kilo.go already promote the
// same way (#588). `skills` and `mcpServers` pass through whatever
// shape the spec declares, since Qoder defines no agnostic-ai-native
// format for either.
func agentMarkdown(a spec.Entry) string {
	resolved := emit.ResolveMeta(a.Meta, target)
	desc, _ := resolved["description"].(string)
	if desc == "" {
		desc = a.Name
	}
	meta := map[string]any{
		"name":        a.Name,
		"description": desc,
	}
	keys := []string{"name", "description"}
	if model, _ := resolved["model"].(string); model != "" {
		meta["model"] = model
		keys = append(keys, "model")
	}
	if tools := qoderToolsString(resolved["tools"]); tools != "" {
		meta["tools"] = tools
		keys = append(keys, "tools")
	}
	for _, k := range []string{"color", "skills", "mcpServers"} {
		if v, ok := resolved[k]; ok {
			meta[k] = v
			keys = append(keys, k)
		}
	}
	emit.MergeCustomTargetMeta(meta, &keys, a.Meta, target,
		"name", "description", "model", "tools", "color", "skills", "mcpServers")
	front := emit.FrontmatterOrdered(meta, keys)
	trimmed := strings.TrimSpace(a.Body)
	if trimmed == "" {
		return front + "\n"
	}
	return front + "\n" + trimmed + "\n"
}

// qoderToolsString renders the generic `tools` field as Qoder's
// documented comma-separated string: `tools: Read, Grep, Bash`, no
// list syntax. A list joins with ", "; a spec that already wrote a
// plain string (typically via x-qoder.tools) passes through unchanged.
// Returns "" when tools is absent or empty, so the caller omits the key
// entirely rather than writing an empty one.
func qoderToolsString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return strings.Join(emit.StringSlice(v), ", ")
}
