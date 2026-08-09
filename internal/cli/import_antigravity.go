package cli

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// antigravityRulesDirs lists the rules directories Antigravity reads,
// preferred first: "Antigravity now defaults to `.agents/rules`, but
// still maintains backward support for `.agent/rules`"
// (antigravity.google/docs/rules-workflows). Import walks the first one
// that exists so both pre- and post-plural-default projects round-trip.
var antigravityRulesDirs = []string{
	filepath.Join(".agents", "rules"),
	filepath.Join(".agent", "rules"),
}

const antigravityMainFile = ".agent/AGENTS.md"

// antigravityMCPFile mirrors the antigravity adapter's own
// defaultMCPFile (internal/adapters/antigravity/antigravity.go).
// Adapter packages cannot import each other, so the literal path is
// duplicated the way every other target's importer already does.
const antigravityMCPFile = ".agents/mcp_config.json"

const antigravityMCPKey = "mcpServers"

// antigravityMCPTopLevel are the mcp_config.json server keys captured
// as first-class MCP spec fields: stdio's `command`/`args`/`env`/`cwd`,
// remote's `headers`, and the shared `disabled` flag
// (antigravity.google/docs/ide/mcp). `serverUrl` is also a known field
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

// importFromAntigravity reads an existing Antigravity project under
// root and writes specs into the configured source directories.
//
//   - `.agents/rules/*.md` (or the legacy `.agent/rules/*.md`) walks via
//     the shared rules-directory importer (agent-<name>.md routes to
//     agents, the rest to rules; the provenance header and the leading
//     `# <heading>\n` block are stripped from each body).
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
	c, err := importRulesDirectory(root, antigravityImportDir(root), src)
	if err != nil {
		return err
	}

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
