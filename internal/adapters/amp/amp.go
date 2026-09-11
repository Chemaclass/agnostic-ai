// Package amp emits Sourcegraph Amp configs.
//
// The project-root `AGENTS.md` is written centrally by `sync` as a
// slim pointer to the source specs (one body shared with every other
// target's entry-point file). When `outputs.amp.rules-file` is set,
// this adapter instead writes the legacy concatenated layout at that
// path so users on older workflows keep their behavior.
//
// Skills emit as a folder per skill under `.agents/skills/<name>/SKILL.md`
// (Amp's native skills layout).
//
// Agents emit no file of their own. They used to land in
// `.agents/commands/<name>.md`, but Amp removed custom commands on
// 2026-01-29 (https://ampcode.com/news/slashing-custom-commands) and its
// migration steps end with "Delete the original command file", so that
// directory is one the vendor tells users to delete. Their bodies reach
// Amp through the merged document when `outputs.amp.rules-file` is set,
// and otherwise only through the entry-point pointer to the source
// specs, which is what emit.NoteCoverageGap reports (target-audit
// 2026-09-11, #727). A prior sync's generated files under
// `.agents/commands/` are swept on the next run.
//
// The migration's replacement path, `.agents/skills/<name>/SKILL.md`,
// is deliberately not reused for agents. That tree is shared: codex,
// goose, crush, factory, augment, antigravity and others scan the same
// directory, and most of them already emit the same Agent spec to their
// own native agent surface. Writing it there as a skill too would
// duplicate one spec inside those tools, and would silently overwrite a
// Skill spec of the same name. A native agent surface does exist on
// Amp's side (ampcode.com/docs/customize/plugins:
// `amp.createAgent(...)` and `amp.registerAgentMode(...)`), but it is
// programmatic TypeScript, out of reach of a declarative emitter.
//
// The Command spec kind is not declared in caps.Supports: Amp's docs
// (ampcode.com/docs/customize/skills) document `.agents/skills/`, but a
// full sweep of every page in ampcode.com/llms.txt finds no file-based
// command surface anywhere, confirming #553's conclusion still holds
// (re-swept over all 50 pages, target-audit 2026-09-11, #727).
// `ampcode.com/manual`, this repo's former citation for that page, now
// redirects to the docs index rather than serving its own content, so
// it is cited by topic instead.
// `.agents/checks/` was genuinely documented as a code-review surface
// in the 2026-08-20 snapshot; it is gone from every page in the current
// sweep, so that surface retires rather than ships. Commands register
// programmatically via `amp.registerCommand(...)` in plugin TypeScript,
// and the migration post above tells users to delete the old command
// file rather than pointing at a replacement path, so there is no path
// left to redirect a Command spec to. A Command spec targeting amp is
// skipped with a warning (see #553).
//
// Previous releases of this adapter wrote `AGENT.md` (singular).
// `AGENTS.md` is Amp's primary file and wins when both exist in a
// directory; `AGENT.md` remains a documented fallback Amp reads only
// when no `AGENTS.md` is present there, which never happens once sync
// writes `AGENTS.md` (target-audit 2026-09-03, #663). The first sync
// after upgrading detects an old agnostic-generated `AGENT.md` and
// renames it to `AGENT.md.bak` so users can verify the new layout
// before deleting the backup.
package amp

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target           = "amp"
	defaultOutFile   = "AGENTS.md"
	defaultSkillsDir = ".agents/skills"
	defaultMCPFile   = ".amp/settings.json"
	legacyOutFile    = "AGENT.md"
	ampMCPKey        = "amp.mcpServers"
	// retiredCommandsDir is where agents landed before Amp removed
	// custom commands. Swept, never written.
	retiredCommandsDir = ".agents/commands"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindMCP},
}

// Adapter emits Amp configs.
type Adapter struct{}

// New returns an Amp adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes a folder per skill under `.agents/skills/<name>/SKILL.md`,
// `.amp/settings.json` for MCP servers, and—when opted in via
// outputs.amp.rules-file—a legacy concatenated rules document that also
// carries the agent bodies. Agents get no file of their own; see the
// package doc. The project-root AGENTS.md is written by `sync`, not
// here.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}

	sess.MigrateLegacyFile(cfg, target, legacyOutFile, defaultOutFile, dryRun)

	warnCommandsDirRemoved(sess, cfg)
	if err := sess.RemoveGeneratedTree(retiredCommandsDir, dryRun); err != nil {
		return err
	}
	if emit.OutputRulesFile(cfg, target, "") == "" {
		emit.NoteCoverageGap(target, spec.KindAgent, len(b.Agents), "outputs.amp.rules-file")
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	if err := sess.WriteSkillFolders(b.Skills, target, skillsDir, dryRun); err != nil {
		return err
	}
	if err := sess.EmitLegacyRulesFile(b, cfg, target, emit.MergedOpts{Title: "AGENTS.md"}, dryRun); err != nil {
		return err
	}
	return emitMCPSettings(sess, b.MCPs, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// warnCommandsDirRemoved fires once per real sync when
// outputs.amp.commands-dir is still configured. This adapter used to
// write one command file per agent there; it no longer does, so a user
// who moved the directory would otherwise see an empty path and no
// explanation. Files a previous sync left at a custom path carry the
// provenance marker and go on the next full sync's ledger orphan
// sweep, the same way the retired default is swept here.
func warnCommandsDirRemoved(sess *emit.Session, cfg *config.Config) {
	if sess.IsCapturing() {
		return
	}
	dir := emit.OutputCommandsDir(cfg, target, "")
	if dir == "" {
		return
	}
	_, _ = fmt.Fprintf(emit.Warner,
		"%s: outputs.amp.commands-dir (%s) is set, but Amp removed custom commands on 2026-01-29 and its migration steps end with \"Delete the original command file\". Nothing is written there anymore. Remove outputs.amp.commands-dir to silence this.\n",
		target, dir)
}

// emitMCPSettings writes (or merges into) `.amp/settings.json` with the
// `amp.mcpServers` map. Routes through emit.MergeJSONFile so any
// pre-existing user-managed keys (theme, editor settings, ...) survive
// the sync; only `amp.mcpServers` is overwritten.
func emitMCPSettings(sess *emit.Session, mcps []spec.Entry, path string, dryRun bool) error {
	if len(mcps) == 0 {
		return nil
	}
	return sess.MergeJSONFile(path, map[string]any{
		ampMCPKey: buildMCPMap(mcps),
	}, dryRun)
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

// buildMCPEntry renders a single MCP server in Amp's settings shape.
// Amp accepts the standard `command`/`args`/`env` for stdio and
// `url`/`headers` for HTTP transports (ampcode.com/docs/customize/mcp).
//
// Any field beyond that set reaches the entry through `x-amp`
// (emit.MergeCustomTargetMeta), the same passthrough the skill
// renderer already gives its own surface. The field this
// unblocks today is `includeTools`, which ampcode.com/docs/customize/skills
// lists under "Common fields": "includeTools (string[], optional but
// recommended) contains tool names or glob patterns used to choose
// which tools are exposed". It stays namespaced rather than mapped
// top-level because ampcode.com/docs/customize/mcp says MCP servers
// "use the same configuration fields as MCP servers in skills" and
// then enumerates without naming it: the clause implies the field, the
// enumeration does not, and a namespaced key is correct either way
// (target-audit 2026-08-27, #634).
func buildMCPEntry(e spec.Entry) map[string]any {
	transport, _ := e.Meta["type"].(string)
	if transport == "" {
		transport = "stdio"
	}
	entry := map[string]any{}
	switch transport {
	case "stdio":
		if cmd, _ := e.Meta["command"].(string); cmd != "" {
			entry["command"] = cmd
		}
		if args := emit.StringSlice(e.Meta["args"]); len(args) > 0 {
			entry["args"] = args
		}
	case "http", "sse":
		if url, _ := e.Meta["url"].(string); url != "" {
			entry["url"] = url
		}
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			entry["headers"] = h
		}
	}
	if env := emit.StringMap(e.Meta["env"]); len(env) > 0 {
		entry["env"] = env
	}
	var keys []string
	emit.MergeCustomTargetMeta(entry, &keys, e.Meta, target,
		"command", "args", "url", "headers", "env")
	return entry
}
