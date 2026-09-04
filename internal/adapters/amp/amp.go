// Package amp emits Sourcegraph Amp configs.
//
// The project-root `AGENTS.md` is written centrally by `sync` as a
// slim pointer to the source specs (one body shared with every other
// target's entry-point file). When `outputs.amp.rules-file` is set,
// this adapter instead writes the legacy concatenated layout at that
// path so users on older workflows keep their behavior.
//
// Agents emit as custom slash commands under `.agents/commands/<name>.md`.
// Skills emit as a folder per skill under `.agents/skills/<name>/SKILL.md`
// (Amp's native skills layout); Amp removed custom commands in favor of
// skills (https://ampcode.com/news/slashing-custom-commands).
//
// The Command spec kind is not declared in caps.Supports: Amp's owner's
// manual (https://ampcode.com/manual) documents `.agents/skills/` and
// `.agents/checks/` but no file-based command surface. Commands register
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
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target             = "amp"
	defaultOutFile     = "AGENTS.md"
	defaultCommandsDir = ".agents/commands"
	defaultSkillsDir   = ".agents/skills"
	defaultMCPFile     = ".amp/settings.json"
	legacyOutFile      = "AGENT.md"
	ampMCPKey          = "amp.mcpServers"
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

// Emit writes one command file per agent, a folder per skill under
// `.agents/skills/<name>/SKILL.md`, `.amp/settings.json` for MCP
// servers, and—when opted in via outputs.amp.rules-file—a legacy
// concatenated rules document. The project-root AGENTS.md is written by
// `sync`, not here.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}

	sess.MigrateLegacyFile(cfg, target, legacyOutFile, defaultOutFile, dryRun)

	commandsDir := emit.OutputCommandsDir(cfg, target, defaultCommandsDir)
	if err := emitAgentCommands(sess, b.Agents, commandsDir, dryRun); err != nil {
		return err
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

func emitAgentCommands(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	for _, a := range agents {
		body := emit.WithHeader(commandFile(a), emit.FormatMarkdown)
		if err := sess.WriteFile(filepath.Join(dir, a.Name+".md"), body, dryRun); err != nil {
			return err
		}
	}
	return nil
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
// `url`/`headers` for HTTP transports (see Amp owner's manual MCP
// guide).
//
// Any field beyond that set reaches the entry through `x-amp`
// (emit.MergeCustomTargetMeta), the same passthrough commandFile and
// the skill renderer already give their own surfaces. The field this
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

// commandFile renders one Amp slash command markdown file: description
// frontmatter (when present) followed by the spec body.
func commandFile(e spec.Entry) string {
	meta := emit.ResolveMeta(e.Meta, target)
	front := map[string]any{}
	keys := []string{}
	if d, _ := meta["description"].(string); d != "" {
		front["description"] = d
		keys = append(keys, "description")
	}
	// Pass through arbitrary x-amp keys beyond description so a target
	// scoped custom key reaches the command frontmatter. See #367.
	emit.MergeCustomTargetMeta(front, &keys, e.Meta, target, "description")
	var sb strings.Builder
	sb.WriteString(emit.FrontmatterOrdered(front, keys))
	sb.WriteString("\n")
	sb.WriteString(e.Body)
	return sb.String()
}
