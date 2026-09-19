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

// clineRulesDirs lists the candidate rules directories, preferred
// first. `.clinerules` is the only one any Cline surface reads
// (GlobalFileNames.clineRules in the VS Code extension, resolved
// against the workspace root); `.cline/rules` shows up only in the
// project tree on docs.cline.bot/getting-started/config, which
// releases #534 through #853 defaulted to (target-audit 2026-09-18,
// #853). Import walks the first one that exists so a project synced by
// either release round-trips. A `.clinerules/` tree may hold
// `agent-<name>.md` files (the pre-#534 combined rules-and-agents
// convention); importRulesDirectory already reclassifies those by
// filename prefix, so they come back as agents exactly as they always
// did.
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

// clineSkillsDirs lists every documented project skill path in
// precedence order. `.cline/skills/` is the recommended location and is
// confirmed by GlobalFileNames.clineSkillsDir, followed by
// `.clinerules/skills/` and Claude-compatible skills.
var clineSkillsDirs = []string{
	filepath.Join(".cline", "skills"),
	filepath.Join(".clinerules", "skills"),
	filepath.Join(".claude", "skills"),
}

// clineImportDir returns the first existing candidate rules dir under
// root, defaulting to the preferred `.clinerules` when neither exists
// yet.
func clineImportDir(root string) string {
	for _, d := range clineRulesDirs {
		if dirExists(filepath.Join(root, d)) {
			return d
		}
	}
	return clineRulesDirs[0]
}

// importFromCline reads an existing Cline project and writes specs into
// the configured source directories, reversing the cline emit:
//
//   - `.clinerules/*.md` (or `.cline/rules/*.md`, for a project synced
//     between #534 and #853) walks via the shared rules-directory
//     importer. A `skill-<name>.md` there still imports as a skill too,
//     covering projects synced before skills moved to a native folder;
//     an `agent-<name>.md` there reclassifies as an agent, covering
//     projects synced before agents moved to their own directory
//     (#534).
//   - `.cline/agents/*.yml` (the native agents directory) reconstructs
//     agents as `<name>.md` specs, byte-for-byte minus the provenance
//     header. `.yaml` is read too, and `.md` last, so a project synced
//     before #886 still round-trips.
//   - `.cline/skills/`, `.clinerules/skills/`, and `.claude/skills/`
//     reconstruct native skill folders with bundled assets. Earlier paths
//     win same-name collisions.
func importFromCline(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills); err != nil {
		return err
	}
	rulesDir := clineImportDir(root)
	opts := rulesDirImportOpts{}
	if rulesDir == ".clinerules" {
		opts.SkipDirs = map[string]bool{"skills": true}
	}
	c, err := importRulesDirectoryWith(root, rulesDir, src, opts)
	if err != nil {
		return err
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
			if err := importWriteFile(dstPath, []byte(header.Strip(string(data))), 0o644); err != nil {
				return count, fmt.Errorf("write %s: %w", dstPath, err)
			}
			count++
		}
	}
	return count, nil
}
