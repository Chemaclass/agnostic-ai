package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// windsurfRulesDirs lists the rules directories Devin Desktop (the
// renamed Windsurf) reads, preferred first. Import walks the first one
// that exists so both pre- and post-rename projects round-trip.
var windsurfRulesDirs = []string{
	filepath.Join(".devin", "rules"),
	filepath.Join(".windsurf", "rules"),
}

// windsurfSkillsDirs lists every documented project skill path in
// precedence order. The shared Agent Skills path comes first, followed by
// Devin's own current path and the Windsurf compatibility path.
var windsurfSkillsDirs = []string{
	filepath.Join(".agents", "skills"),
	filepath.Join(".devin", "skills"),
	filepath.Join(".windsurf", "skills"),
}

// windsurfAgentsDir is Devin CLI's native custom-subagent directory
// (docs.devin.ai/cli/subagents): flat `<name>.md` files, not the
// pre-#638 `agent-<name>.md` rule-form.
const windsurfAgentsDir = ".devin/agents"

// windsurfMCPFile is the project-scoped MCP server registry Devin
// Local reads (docs.devin.ai/cli/extensibility/mcp/configuration).
const windsurfMCPFile = ".devin/mcp_config.json"

// windsurfMCPKey is the top-level JSON object holding the server map.
const windsurfMCPKey = "mcpServers"

// windsurfHooksFile is Devin CLI's project-scoped hooks file
// (docs.devin.ai/cli/extensibility/hooks/overview, #629).
const windsurfHooksFile = ".devin/hooks.v1.json"

// windsurfOwnOutputSubtrees are the root-relative directories the
// windsurf adapter's own emit writes as always-unscoped output: the
// preferred and legacy rules dirs, the native agents dir, and every
// path windsurfSkillsDirs reads (windsurf.go's defaultDir/legacyDir/
// defaultAgentsDir/defaultSkillsDir, duplicated here since adapter
// constants are unexported the same way antigravityOwnOutputSubtrees
// already duplicates antigravity's). These are pruned by exact
// root-relative path so import does not misread the tool's own
// rules/agents/skills tree as a nested scope, matching
// antigravityOwnOutputSubtrees (#1123).
var windsurfOwnOutputSubtrees = map[string]bool{
	".devin/rules":     true,
	".devin/agents":    true,
	".devin/skills":    true,
	".windsurf/rules":  true,
	".windsurf/skills": true,
	".agents/skills":   true,
}

// windsurfRulesDirFromCfg returns the project-relative
// `outputs.windsurf.rules-dir` path when configured, otherwise "".
func windsurfRulesDirFromCfg(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if o, ok := cfg.Outputs["windsurf"]; ok {
		return o.RulesDir
	}
	return ""
}

// windsurfImportDir returns `outputs.windsurf.rules-dir` verbatim when
// configured, the same path emission itself resolves to
// (emit.OutputRulesDir), so a synced custom directory round-trips
// instead of import falling back to disk detection and finding nothing
// at either conventional default (#1123). Unconfigured, it returns the
// first existing candidate among windsurfRulesDirs, defaulting to the
// preferred `.devin/rules` when neither exists yet.
func windsurfImportDir(root string, cfg *config.Config) string {
	if dir := windsurfRulesDirFromCfg(cfg); dir != "" {
		return dir
	}
	for _, d := range windsurfRulesDirs {
		if dirExists(filepath.Join(root, d)) {
			return d
		}
	}
	return windsurfRulesDirs[0]
}

// windsurfScopedRulesDirs returns every project sub-directory holding
// its own copy of rulesDir, sorted, root excluded. Devin reads
// "`.devin/rules` or `.windsurf/rules` in any sub-directory of your
// workspace" (docs.devin.ai/desktop/cascade/memories), which is where
// sync writes a scoped rule, so import has to look there too.
// `CheckScopePath` rejects nothing about a name like `.github`,
// `vendor`, or `node_modules`, so emission accepts a scope there and
// import must be able to round-trip it: pruning every hidden directory
// and a hardcoded `node_modules`/`vendor` list, the way an earlier
// draft of this function did, silently orphaned
// `.github/.devin/rules/release.md` on the next full sync (#1123,
// mirroring antigravityScopedRulesDirs's own earlier draft, #1114).
// Pruning matches the exact root-relative path, never a bare directory
// name at any depth either: an earlier draft skipped every directory
// named after a source root's first segment, so `sources.rules:
// config/rules` pruned `packages/api/config` too, and a legitimate
// `packages/api/config/.devin/rules/auth.md` scope never imported
// (#1123). Only `.git`, agnostic-ai's own configured source
// directories, and windsurfOwnOutputSubtrees are pruned now; see
// scopedRulesDirs, the walker this and antigravityScopedRulesDirs
// share.
func windsurfScopedRulesDirs(root, rulesDir string, src config.Sources) ([]string, error) {
	return scopedRulesDirs(root, rulesDir, windsurfOwnOutputSubtrees, src)
}

// normalizeWindsurfRuleMeta turns Devin's `trigger` activation key back
// into the generic `alwaysApply` the spec format uses, so a synced rule
// imports to the spec it came from. `always_on` is the mode a bare file
// already has; the other three all mean "not always on", and the
// `globs` / `description` keys sitting next to the trigger carry which
// one it was (docs.devin.ai/cli/extensibility/rules). An unrecognized
// value is left alone rather than guessed at.
func normalizeWindsurfRuleMeta(meta map[string]any) {
	trigger, ok := meta["trigger"].(string)
	if !ok {
		return
	}
	switch trigger {
	case "always_on":
		delete(meta, "trigger")
		meta["alwaysApply"] = true
	case "glob", "model_decision", "manual":
		delete(meta, "trigger")
		meta["alwaysApply"] = false
	}
}

// normalizeWindsurfSkill moves Devin-only trigger policy under the target
// namespace. The shared skill renderer resolves it back to top-level
// `triggers` for Windsurf without leaking that native key to other targets.
func normalizeWindsurfSkill(data []byte) ([]byte, error) {
	meta, body := splitMdcFrontmatter(data)
	triggers, ok := meta["triggers"]
	if !ok {
		return data, nil
	}
	delete(meta, "triggers")
	targetMeta, _ := meta["x-windsurf"].(map[string]any)
	if targetMeta == nil {
		targetMeta = map[string]any{}
	}
	targetMeta["triggers"] = triggers
	meta["x-windsurf"] = targetMeta
	front, err := yaml.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("marshal frontmatter: %w", err)
	}
	out := "---\n" + string(front) + "---\n"
	if body != "" {
		out += "\n" + body
		if !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
	}
	return []byte(out), nil
}

// importFromWindsurf reads an existing Devin Desktop / Windsurf project
// and writes specs into the configured source directories, reversing
// the windsurf emit:
//
//   - `.devin/rules/*.md` (or the legacy `.windsurf/rules/*.md`)
//     reclassifies by filename prefix into rules and agents
//     (`agent-<name>.md`); a `skill-<name>.md` there still imports as a
//     skill too, covering projects synced before skills moved to a
//     native folder. The `trigger` activation key translates back into
//     `alwaysApply` / `globs` / `description` via
//     normalizeWindsurfRuleMeta. The rules directory itself is
//     `outputs.windsurf.rules-dir` when configured, the same path
//     emission writes to, and only falls back to disk detection between
//     the two conventional defaults when unconfigured (#1123).
//   - `<scope>/.devin/rules/*.md` in any project sub-directory imports
//     back to a scoped spec at `rules/<scope>/<name>.md`, the emit side
//     of #628, scanning the whole project tree rather than a fixed list
//     of candidate directories so a scope named `.github`, `vendor`, or
//     anything else `CheckScopePath` accepts still round-trips (#1123).
//   - `.devin/agents/*.md` (the native subagent directory) reconstructs
//     agents, byte-for-byte minus the provenance header, so `model`,
//     `max-nesting`, and any `x-windsurf` key round-trip untouched. One
//     field is lossy: `allowed-tools` re-imports as whatever Devin name
//     is on disk (e.g. `write`, `edit`), not the Claude-style name it
//     was translated from. `Write` and `Edit` map onto distinct Devin
//     names since #1022, so the round trip no longer conflates the
//     two; it simply keeps Devin's own spelling rather than the
//     portable one.
//   - `.agents/skills/`, `.devin/skills/`, and `.windsurf/skills/`
//     reconstruct native skill folders with bundled assets. Earlier paths
//     win same-name collisions. `triggers` moves under `x-windsurf` so its
//     manual/model invocation boundary survives sync without leaking.
//   - `.devin/mcp_config.json`'s `mcpServers` map writes one yaml per
//     server. See importWindsurfMCP for the `transport` -> `type`
//     rename this importer applies on the way in.
//   - `.devin/hooks.v1.json` writes one yaml per matcher group, same
//     collapsing rule as importClaudeHooks. Unlike that file, there is
//     no `"hooks"` wrapper key to unwrap, and a `type: prompt` entry
//     imports with `prompt:` in place of `command:` (#629).
//   - a hand-authored `.devinignore` reconstructs an ignore spec (#754),
//     falling back to `.windsurfignore`, the second file sync writes
//     from the same spec, when `.devinignore` is absent (#863).
func importFromWindsurf(root string, src config.Sources, cfg *config.Config) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.Hooks, src.MCPs, src.Settings); err != nil {
		return err
	}
	rulesDir := windsurfImportDir(root, cfg)
	c, err := importRulesDirectoryWith(root, rulesDir, src, rulesDirImportOpts{
		NormalizeMeta: normalizeWindsurfRuleMeta,
	})
	if err != nil {
		return err
	}
	scopes, err := windsurfScopedRulesDirs(root, rulesDir, src)
	if err != nil {
		return err
	}
	for _, scope := range scopes {
		scoped, err := importRulesDirectoryWith(root, filepath.Join(scope, rulesDir), src, rulesDirImportOpts{
			ScopePrefix:   scope,
			NormalizeMeta: normalizeWindsurfRuleMeta,
		})
		if err != nil {
			return err
		}
		c.add(scoped)
	}
	nativeAgents, err := importFlatMarkdownFiles(filepath.Join(root, windsurfAgentsDir), filepath.Join(root, src.Agents), windsurfAgentFields)
	if err != nil {
		return err
	}
	c.agents += nativeAgents
	seenSkills := map[string]bool{}
	for _, skillsDir := range windsurfSkillsDirs {
		folderSkills, err := importSkillFoldersWith(filepath.Join(root, skillsDir), filepath.Join(root, src.Skills), skillFolderImportOpts{
			SkipNames:      seenSkills,
			TransformSkill: normalizeWindsurfSkill,
		})
		if err != nil {
			return err
		}
		c.skills += folderSkills
	}
	mcps, err := importWindsurfMCP(root, filepath.Join(root, src.MCPs))
	if err != nil {
		return err
	}
	hooks, err := importWindsurfHooks(root, filepath.Join(root, src.Hooks))
	if err != nil {
		return err
	}
	ignores, err := importIgnoreFile(root, "windsurf", src)
	if err != nil {
		return err
	}
	settings, err := importWindsurfPermissions(root, filepath.Join(root, src.Settings))
	if err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d hooks, %d mcps, %d ignores, %d settings (from windsurf)\n", c.rules, c.agents, c.skills, hooks, mcps, ignores, settings)
	printImportNextSteps(root, "windsurf")
	return nil
}

// importWindsurfMCP reads `.devin/mcp_config.json` and writes one yaml
// per `mcpServers.<name>` entry into dstDir. Devin's own file spells
// the transport discriminant `transport`, not the `type` key
// agnostic-ai's spec meta uses everywhere else (see the windsurf
// adapter's mcp.go), so this renames it on the way in. A url-only
// entry with no explicit `transport` (Devin's own default) still
// infers `type: http` via writeMCPYAMLs, the same way every other
// JSON-map importer does. No-op when the file is absent.
func importWindsurfMCP(root, dstDir string) (int, error) {
	servers, err := readJSONMapAt(filepath.Join(root, windsurfMCPFile), windsurfMCPKey)
	if err != nil || len(servers) == 0 {
		return 0, err
	}
	for _, raw := range servers {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if transport, ok := entry["transport"]; ok {
			entry["type"] = transport
			delete(entry, "transport")
		}
	}
	return writeMCPYAMLs(servers, dstDir)
}
