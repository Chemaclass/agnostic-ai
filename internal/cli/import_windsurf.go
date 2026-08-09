package cli

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// windsurfRulesDirs lists the rules directories Devin Desktop (the
// renamed Windsurf) reads, preferred first. Import walks the first one
// that exists so both pre- and post-rename projects round-trip.
var windsurfRulesDirs = []string{
	filepath.Join(".devin", "rules"),
	filepath.Join(".windsurf", "rules"),
}

// windsurfSkillsDir is the shared cross-tool skills tree Devin Desktop
// scans (docs.devin.ai/desktop/cascade/skills): a folder per skill
// holding a SKILL.md, the same tree codex, amp, zed, crush, and
// openhands emit into.
const windsurfSkillsDir = ".agents/skills"

// windsurfMCPFile is the project-scoped MCP server registry Devin
// Local reads (docs.devin.ai/cli/extensibility/mcp/configuration).
const windsurfMCPFile = ".devin/mcp_config.json"

// windsurfMCPKey is the top-level JSON object holding the server map.
const windsurfMCPKey = "mcpServers"

// windsurfImportDir returns the first existing candidate rules dir
// under root, defaulting to the preferred `.devin/rules` when neither
// exists yet.
func windsurfImportDir(root string) string {
	for _, d := range windsurfRulesDirs {
		if dirExists(filepath.Join(root, d)) {
			return d
		}
	}
	return windsurfRulesDirs[0]
}

// importFromWindsurf reads an existing Devin Desktop / Windsurf project
// and writes specs into the configured source directories, reversing
// the windsurf emit:
//
//   - `.devin/rules/*.md` (or the legacy `.windsurf/rules/*.md`)
//     reclassifies by filename prefix into rules and agents
//     (`agent-<name>.md`); a `skill-<name>.md` there still imports as a
//     skill too, covering projects synced before skills moved to a
//     native folder.
//   - `.agents/skills/<name>/SKILL.md` folders reconstruct skills
//     natively, with bundled sibling assets copied byte-for-byte.
//   - `.devin/mcp_config.json`'s `mcpServers` map writes one yaml per
//     server. See importWindsurfMCP for the `transport` -> `type`
//     rename this importer applies on the way in.
func importFromWindsurf(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.MCPs); err != nil {
		return err
	}
	c, err := importRulesDirectory(root, windsurfImportDir(root), src)
	if err != nil {
		return err
	}
	folderSkills, err := importSkillFolders(filepath.Join(root, windsurfSkillsDir), filepath.Join(root, src.Skills))
	if err != nil {
		return err
	}
	c.skills += folderSkills
	mcps, err := importWindsurfMCP(root, filepath.Join(root, src.MCPs))
	if err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d mcps (from windsurf)\n", c.rules, c.agents, c.skills, mcps)
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
