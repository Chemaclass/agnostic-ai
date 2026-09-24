package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

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

// antigravityMainFiles lists the project-instructions entry-point file
// Antigravity discovers in a subdirectory, preferred first:
// "<dir>/.agents/AGENTS.md or <dir>/.agents/GEMINI.md"
// (antigravity.google/docs/rules). Import walks the first one that
// exists so both post-#1114 and pre-#1114 projects round-trip; the
// singular `.agent/AGENTS.md` predates the documented path.
var antigravityMainFiles = []string{
	filepath.Join(".agents", "AGENTS.md"),
	filepath.Join(".agent", "AGENTS.md"),
}

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

// antigravityImportDir returns `outputs.antigravity.rules-dir` verbatim
// when configured, the same path emission itself resolves to
// (emit.OutputRulesDir), so a synced custom directory round-trips
// instead of import falling back to disk detection and finding nothing
// at either conventional default. Unconfigured, it returns the first
// existing candidate among the conventional default and legacy paths,
// defaulting to the preferred `.agents/rules` when neither exists yet.
func antigravityImportDir(root string, cfg *config.Config) string {
	if dir := antigravityRulesDirFromCfg(cfg); dir != "" {
		return dir
	}
	for _, d := range antigravityRulesDirs {
		if dirExists(filepath.Join(root, d)) {
			return d
		}
	}
	return antigravityRulesDirs[0]
}

// antigravityRulesDirFromCfg returns the project-relative
// `outputs.antigravity.rules-dir` path when configured, otherwise "".
func antigravityRulesDirFromCfg(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if o, ok := cfg.Outputs["antigravity"]; ok {
		return o.RulesDir
	}
	return ""
}

// antigravityImportMainFile returns the first existing candidate
// entry-point file under root, defaulting to the preferred
// `.agents/AGENTS.md` when neither exists yet.
func antigravityImportMainFile(root string) string {
	for _, f := range antigravityMainFiles {
		if fileExists(filepath.Join(root, f)) {
			return f
		}
	}
	return antigravityMainFiles[0]
}

// antigravityScopedRulesDirs returns every project sub-directory
// holding its own copy of rulesDir, sorted, root excluded. Antigravity
// reads "a `.agents/rules/` directory ... in any subdirectory of your
// project" (antigravity.google/docs/rules), which is where sync writes
// a scoped rule, so import has to look there too. `CheckScopePath`
// rejects nothing about a name like `.github`, `vendor`, or
// `node_modules`, so emission accepts a scope there and import must be
// able to round-trip it: pruning by a hidden-dir prefix or a
// hardcoded name list, the way an earlier draft of this function did
// (matching windsurfScopedRulesDirs), silently orphaned
// `.github/.agents/rules/release.md` on the next full sync (#1114).
// Only `.git` (never a legitimate scope, and large enough that walking
// it is wasted work) and agnostic-ai's own source and output roots are
// pruned: the configured source directories, plus both the plural and
// legacy singular Antigravity output roots, since import's own
// non-scoped call already reads whichever of those two is active and
// a `.agents/rules` or `.agent/rules` nested inside the other would
// only be that same output tree, not a user scope.
//
// Pruning matches the exact root-relative path, never a bare directory
// name at any depth: an earlier draft skipped every directory named
// after a source root's first segment, so `sources.rules: config/rules`
// pruned `packages/api/config` too, and a legitimate
// `packages/api/config/.agents/rules/auth.md` scope never imported
// (#1114 review). `.agents` and `.agent` are excluded the same way, at
// the root only, so a scope whose own name happens to be `.agents` (or
// one further down the tree) is never mistaken for the tool's own
// output root.
func antigravityScopedRulesDirs(root, rulesDir string, src config.Sources) ([]string, error) {
	skipDirs := map[string]bool{".git": true, ".agents": true, ".agent": true}
	for _, p := range []string{src.Agents, src.Skills, src.Rules, src.Hooks, src.MCPs} {
		if p != "" {
			skipDirs[filepath.ToSlash(filepath.Clean(p))] = true
		}
	}
	var scopes []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			return nil
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if skipDirs[rel] {
			return fs.SkipDir
		}
		if !dirExists(filepath.Join(path, rulesDir)) {
			return nil
		}
		scopes = append(scopes, rel)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s for scoped rules dirs: %w", root, err)
	}
	sort.Strings(scopes)
	return scopes, nil
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
//     `# <heading>\n` block are stripped from each body). The rules
//     directory itself is `outputs.antigravity.rules-dir` when
//     configured, the same path emission writes to, and only falls back
//     to disk detection between the two conventional defaults when
//     unconfigured (#1114 review). The
//     `agent-<name>.md` form covers projects synced before agents moved
//     to their own directory (#638). The mandatory `trigger` frontmatter
//     translates back to `alwaysApply` / `globs` / `description` via
//     normalizeAntigravityRuleMeta (#1113) and survives verbatim under
//     `x-antigravity.trigger` too (#1117 review); a pre-#1113 bare rule
//     file carries no frontmatter at all and imports unchanged.
//   - `<scope>/<rules-dir>/*.md` in any project sub-directory imports
//     back to a scoped spec at `rules/<scope>/<name>.md`, the emit side
//     of #1114, with the same `trigger` round-trip the root rules dir
//     gets (#1114 review).
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
//   - `.agents/AGENTS.md` (or the legacy `.agent/AGENTS.md`) mirrors into
//     `.agnostic-ai/AGNOSTIC_AI.md` when present so a hand-edit propagates
//     back into the source body.
func importFromAntigravity(root string, src config.Sources, cfg *config.Config) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.MCPs); err != nil {
		return err
	}
	rulesDir := antigravityImportDir(root, cfg)
	c, err := importRulesDirectoryWith(root, rulesDir, src, rulesDirImportOpts{
		NormalizeMeta: normalizeAntigravityRuleMeta,
		NativeTarget:  "antigravity",
		NativeKeys:    []string{"trigger"},
	})
	if err != nil {
		return err
	}
	scopes, err := antigravityScopedRulesDirs(root, rulesDir, src)
	if err != nil {
		return err
	}
	for _, scope := range scopes {
		scoped, err := importRulesDirectoryWith(root, filepath.Join(scope, rulesDir), src, rulesDirImportOpts{
			ScopePrefix:   scope,
			NormalizeMeta: normalizeAntigravityRuleMeta,
			NativeTarget:  "antigravity",
			NativeKeys:    []string{"trigger"},
		})
		if err != nil {
			return err
		}
		c.add(scoped)
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

	if _, err := mirrorMainFile(root, antigravityImportMainFile(root)); err != nil {
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
