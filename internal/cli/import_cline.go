package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// clineRulesDirs lists both project directories Cline reads. Identical
// imported destinations deduplicate; distinct content returns a conflict.
var clineRulesDirs = []string{
	".clinerules",
	filepath.Join(".cline", "rules"),
}

const (
	// clineAgentsDir is Cline's native per-agent directory, confirmed
	// by resolveAgentConfigSearchPaths in cline/cline
	// (sdk/packages/shared/src/storage/paths.ts:476). Files there are
	// flat `<name>.yml`, not the pre-migration `agent-<name>.md`
	// rule-form.
	clineAgentsDir = ".cline/agents"
)

// clineAgentExts lists the agent-file extensions import accepts, in
// precedence order. `.yml` and `.yaml` are the two the loader reads
// (isYamlFile, configured-agent-config.ts L113-115); `.md` is what
// releases #534 through #886 wrote there and is read last so a project
// synced by one of those still round-trips.
var clineAgentExts = []string{".yml", ".yaml", ".md"}

// clineSkillsDirs lists every project skill path Cline scans, in
// precedence order. `.cline/skills/` is the recommended location and is
// confirmed by GlobalFileNames.clineSkillsDir, followed by
// `.clinerules/skills/`, Claude-compatible skills, and `.agents/skills`.
//
// `.agents/skills` is source-confirmed and doc-unconfirmed:
// `getWorkspaceSkillDirectories` (sdk/packages/shared/src/storage/
// paths.ts:461) maps `.clinerules`, `.cline` and `.agents` onto
// `<dir>/skills`, and the VS Code extension agrees independently
// (`getSkillsDirectoriesForScan` in apps/vscode/src/core/storage/
// skill-directories.ts returns `agentsSkillsDir: ".agents/skills"`).
// docs.cline.bot/customization/skills still lists only the first three.
// Without it, importing a project whose skills live in the shared
// `.agents/skills` tree (amp, codex, windsurf, zed) returned nothing
// for cline (target-audit 2026-09-19, #889).
var clineSkillsDirs = []string{
	filepath.Join(".cline", "skills"),
	filepath.Join(".clinerules", "skills"),
	filepath.Join(".claude", "skills"),
	filepath.Join(".agents", "skills"),
}

// importFromCline reads an existing Cline project and writes specs into
// the configured source directories, reversing the cline emit:
//
//   - `.clinerules/*.md` and `.cline/rules/*.md` use the shared rule
//     importer. Native paths conditions retain exact arrays in x-cline.
//     Workflows, hooks, and skills under .clinerules are not rules.
//     A `skill-<name>.md` there still imports as a skill too,
//     covering projects synced before skills moved to a native folder;
//     an `agent-<name>.md` there reclassifies as an agent, covering
//     projects synced before agents moved to their own directory
//     (#534).
//   - `.cline/agents/*.yml` (the native agents directory) reconstructs
//     agents as `<name>.md` specs, byte-for-byte minus the provenance
//     header. `.yaml` is read too, and `.md` last, so a project synced
//     before #886 still round-trips.
//   - `.cline/skills/`, `.clinerules/skills/`, `.claude/skills/`, and `.agents/skills/`
//     reconstruct native skill folders with bundled assets. Earlier paths
//     win same-name collisions.
func importFromCline(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills); err != nil {
		return err
	}
	var c rulesDirCounts
	seen := map[string]importedRuleContent{}
	for _, rulesDir := range clineRulesDirs {
		opts := rulesDirImportOpts{NativeTarget: "cline", NativeKeys: []string{"paths"}, Seen: seen}
		if rulesDir == ".clinerules" {
			opts.SkipDirs = map[string]bool{"skills": true, "workflows": true, "hooks": true}
		}
		imported, err := importRulesDirectoryWith(root, rulesDir, src, opts)
		if err != nil {
			return err
		}
		c.add(imported)
	}
	nativeAgents, err := importClineAgents(filepath.Join(root, clineAgentsDir), filepath.Join(root, src.Agents))
	if err != nil {
		return err
	}
	c.agents += nativeAgents
	seenSkills := map[string]bool{}
	for _, skillsDir := range clineSkillsDirs {
		folderSkills, err := importSkillFoldersWith(filepath.Join(root, skillsDir), filepath.Join(root, src.Skills), skillFolderImportOpts{SkipNames: seenSkills})
		if err != nil {
			return err
		}
		c.skills += folderSkills
	}
	summaryf("imported %d rules, %d agents, %d skills (from cline)\n", c.rules, c.agents, c.skills)
	printImportNextSteps(root, "cline")
	return nil
}

// importClineAgents copies every top-level agent file in src into
// dstDir as `<name>.md`, stripping the provenance header. Cline's agent
// files are frontmatter over a Markdown system prompt, so the body
// carries across unchanged and only the extension moves: source specs
// are always `.md`. Extensions are walked in clineAgentExts order and
// the first file to claim a name wins, so a `.yml` written by the
// current release beats a `.md` left by an older one. A missing
// directory is a no-op.
func importClineAgents(src, dstDir string) (int, error) {
	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	count := 0
	seen := map[string]bool{}
	for _, ext := range clineAgentExts {
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ext {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ext)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			srcPath := filepath.Join(src, e.Name())
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return count, fmt.Errorf("read %s: %w", srcPath, err)
			}
			dstPath := filepath.Join(dstDir, name+".md")
			if err := importWriteSpecMarkdown(dstPath, []byte(header.Strip(string(data))), 0o644, clineAgentFields); err != nil {
				return count, fmt.Errorf("write %s: %w", dstPath, err)
			}
			count++
		}
	}
	return count, nil
}
