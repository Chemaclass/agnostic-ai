package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// antigravityRulesDirs lists the rules directories Antigravity reads,
// preferred first: "Antigravity defaults to `.agents/rules`, but still
// maintains backward compatibility for `.agent/rules`"
// (antigravity.google/docs/rules-workflows?tab=ide). Import walks the
// first one that exists so both pre- and post-plural-default projects
// round-trip.
var antigravityRulesDirs = []string{
	filepath.Join(".agents", "rules"),
	filepath.Join(".agent", "rules"),
}

const antigravityMainFile = ".agent/AGENTS.md"

// antigravityDefaultAgentsDir is Antigravity's native custom-subagent
// root. New output uses `<name>/agent.md`; import falls back to the
// older flat `<name>.md` form when no nested profiles exist.
const antigravityDefaultAgentsDir = ".agents/agents"

// antigravityMCPFile mirrors the antigravity adapter's own
// defaultMCPFile (internal/adapters/antigravity/antigravity.go).
// Adapter packages cannot import each other, so the literal path is
// duplicated the way every other target's importer already does.
const antigravityMCPFile = ".agents/mcp_config.json"

const antigravityMCPKey = "mcpServers"

// antigravityMCPTopLevel are the mcp_config.json server keys captured
// as first-class MCP spec fields: stdio's `command`/`args`/`env`/`cwd`,
// remote's `headers`, and the shared `disabled` flag
// (antigravity.google/docs/mcp?tab=ide). `serverUrl` is also a known field
// but renames to the spec's generic `url` before writing, since
// buildMCPServer reads `url`, not `serverUrl` (the vendor's own doc
// states "Legacy fields like `url` or `httpUrl` are not supported" for
// Antigravity itself, so the vendor name only ever exists in the raw
// JSON). Any other documented field (`authProviderType`, `oauth`,
// `disabledTools`, same page) or any field the vendor adds next is
// preserved under `x-antigravity` instead of dropped, the read side of
// the emit.MergeCustomTargetMeta passthrough mcp.go's buildMCPServer
// writes on the way out, so a sync -> import -> sync cycle converges in
// both directions (#588, #589).
var antigravityMCPTopLevel = map[string]bool{
	"command": true, "args": true, "env": true, "cwd": true,
	"headers": true, "disabled": true,
}

// antigravityImportDir returns the first existing candidate rules dir
// under root, defaulting to the preferred `.agents/rules` when neither
// exists yet.
func antigravityImportDir(root string) string {
	for _, d := range antigravityRulesDirs {
		if dirExists(filepath.Join(root, d)) {
			return d
		}
	}
	return antigravityRulesDirs[0]
}

// normalizeAntigravityRuleMeta turns Antigravity's `trigger` frontmatter
// key into the generic `alwaysApply` the spec format uses, so a synced
// rule imports to the spec it came from (#1113). The singular `glob`
// spelling the vendor also accepts folds onto the plural `globs` key
// `rulesDirFileContent` reads.
//
// `trigger` itself is never deleted: it stays in meta so the
// `NativeKeys: []string{"trigger"}` passed to importRulesDirectoryWith
// below lifts it, verbatim, into `x-antigravity.trigger`, alongside the
// generic mapping. Collapsing straight to `alwaysApply` and dropping
// the original value lost information the generic fields cannot
// recover alone: a `manual` rule that also carries a `description`
// (the vendor recommends one on every rule) re-derived as
// `model_decision` on the next sync, turning an explicit
// @-mention-only rule into an automatically-loaded one, and an
// unrecognized trigger value re-derived as `always_on`, the most
// active mode (#1117 review). rule.go's ruleTrigger reads the native
// override back and honors it first for exactly that reason.
//
// An unrecognized trigger value gets no generic `alwaysApply` mapping
// and a warning, so it round-trips inert (native override present,
// generic fields silent) rather than silently landing on some default.
func normalizeAntigravityRuleMeta(meta map[string]any) {
	if _, hasGlobs := meta["globs"]; !hasGlobs {
		if g, ok := meta["glob"]; ok {
			meta["globs"] = g
		}
	}
	delete(meta, "glob")

	trigger, ok := meta["trigger"].(string)
	if !ok {
		return
	}
	switch trigger {
	case "always_on":
		meta["alwaysApply"] = true
	case "glob", "model_decision", "manual":
		meta["alwaysApply"] = false
	default:
		summaryf("  ! antigravity rule trigger %q is not always_on/glob/model_decision/manual; kept verbatim under x-antigravity.trigger\n", trigger)
	}
}

// importFromAntigravity reads an existing Antigravity project under
// root and writes specs into the configured source directories.
//
//   - `.agents/rules/*.md` (or the legacy `.agent/rules/*.md`) walks via
//     the shared rules-directory importer (agent-<name>.md routes to
//     agents, the rest to rules; the provenance header and the leading
//     `# <heading>\n` block are stripped from each body). The
//     `agent-<name>.md` form covers projects synced before agents moved
//     to their own directory (#638). The mandatory `trigger` frontmatter
//     translates back to `alwaysApply` / `globs` / `description` via
//     normalizeAntigravityRuleMeta (#1113) and survives verbatim under
//     `x-antigravity.trigger` too (#1117 review); a pre-#1113 bare rule
//     file carries no frontmatter at all and imports unchanged.
//   - `.agents/agents/<name>/agent.md` (the preferred native subagent form)
//     reconstructs agents, byte-for-byte minus the provenance header,
//     so `model` and any `x-antigravity` key round-trip untouched. A
//     generic `tools` list never reaches the file on emit, so it never
//     comes back from one either. Import falls back to the older flat
//     `.agents/agents/*.md` form only when no nested profile exists, so
//     co-located Goose/OpenHands profiles are not mistaken for
//     Antigravity agents.
//   - When `outputs.antigravity.rules-file` is set in agnostic-ai.yaml,
//     the legacy concatenated file is sliced by H2 sections.
//   - `.agents/mcp_config.json`'s `mcpServers` map walks via
//     importAntigravityMCP.
//   - `.agent/AGENTS.md` mirrors into `.agnostic-ai/AGNOSTIC_AI.md`
//     when present so a hand-edit propagates back into the source body.
func importFromAntigravity(root string, src config.Sources, cfg *config.Config) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.MCPs); err != nil {
		return err
	}
	c, err := importRulesDirectoryWith(root, antigravityImportDir(root), src, rulesDirImportOpts{
		NormalizeMeta: normalizeAntigravityRuleMeta,
		NativeTarget:  "antigravity",
		NativeKeys:    []string{"trigger"},
	})
	if err != nil {
		return err
	}

	agentsDir := filepath.Join(root, antigravityAgentsDirFromCfg(cfg))
	nativeAgents, err := importAntigravityAgents(agentsDir, filepath.Join(root, src.Agents))
	if err != nil {
		return err
	}
	c.agents += nativeAgents
	skillsDir := filepath.Join(root, ".agents", "skills")
	if !dirExists(skillsDir) {
		skillsDir = filepath.Join(root, ".agent", "skills")
	}
	nativeSkills, err := importSkillFolders(skillsDir, filepath.Join(root, src.Skills))
	if err != nil {
		return err
	}
	c.skills += nativeSkills

	rulesFileCount := 0
	if rulesFile := antigravityRulesFileFromCfg(cfg); rulesFile != "" {
		n, err := sliceMainFileByH2(root, rulesFile, filepath.Join(root, src.Rules))
		if err != nil {
			return err
		}
		rulesFileCount = n
	}

	mcps, err := importAntigravityMCP(root, filepath.Join(root, src.MCPs))
	if err != nil {
		return err
	}

	if _, err := mirrorMainFile(root, antigravityMainFile); err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d mcps\n",
		c.rules+rulesFileCount, c.agents, c.skills, mcps)
	printImportNextSteps(root, "antigravity")
	return nil
}

// antigravityAgentsDirFromCfg returns the configured native agents root.
func antigravityAgentsDirFromCfg(cfg *config.Config) string {
	if cfg != nil {
		if output, ok := cfg.Outputs["antigravity"]; ok && output.AgentsDir != "" {
			return output.AgentsDir
		}
	}
	return antigravityDefaultAgentsDir
}

// importAntigravityAgents imports the nested profile form first. When
// none exist it accepts the vendor's older flat form for compatibility.
func importAntigravityAgents(srcDir, dstDir string) (int, error) {
	entries, err := os.ReadDir(srcDir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", srcDir, err)
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		srcPath := filepath.Join(srcDir, entry.Name(), "agent.md")
		data, err := os.ReadFile(srcPath)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return count, fmt.Errorf("read %s: %w", srcPath, err)
		}
		dstPath := filepath.Join(dstDir, entry.Name()+".md")
		if err := importWriteSpecMarkdown(dstPath, []byte(header.Strip(string(data))), 0o644, antigravityAgentFields); err != nil {
			return count, fmt.Errorf("write %s: %w", dstPath, err)
		}
		count++
	}
	if count > 0 {
		return count, nil
	}
	return importFlatMarkdownFiles(srcDir, dstDir, antigravityAgentFields)
}

// antigravityRulesFileFromCfg returns the project-relative
// `outputs.antigravity.rules-file` path when configured, otherwise "".
func antigravityRulesFileFromCfg(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if o, ok := cfg.Outputs["antigravity"]; ok {
		return o.RulesFile
	}
	return ""
}

// importAntigravityMCP reads `.agents/mcp_config.json` and writes one
// yaml per `mcpServers.<name>` entry into dstDir. No-op when the file
// is absent (#589).
func importAntigravityMCP(root, dstDir string) (int, error) {
	servers, err := readJSONMapAt(filepath.Join(root, antigravityMCPFile), antigravityMCPKey)
	if err != nil || len(servers) == 0 {
		return 0, err
	}
	normalized := map[string]any{}
	for name, raw := range servers {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		normalized[name] = normalizeAntigravityMCPEntry(entry)
	}
	return writeMCPYAMLs(normalized, dstDir)
}

// normalizeAntigravityMCPEntry maps one mcp_config.json server object
// onto the agnostic MCP spec shape (see antigravityMCPTopLevel).
func normalizeAntigravityMCPEntry(entry map[string]any) map[string]any {
	out := map[string]any{}
	xAntigravity := map[string]any{}
	for k, v := range entry {
		switch {
		case k == "serverUrl":
			out["url"] = v
		case antigravityMCPTopLevel[k]:
			out[k] = v
		default:
			xAntigravity[k] = v
		}
	}
	if len(xAntigravity) > 0 {
		out["x-antigravity"] = xAntigravity
	}
	return out
}
