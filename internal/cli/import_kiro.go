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

const (
	kiroSteeringDir = ".kiro/steering"
	kiroMCPFile     = ".kiro/settings/mcp.json"
	kiroMCPKey      = "mcpServers"
	kiroMainFile    = "AGENTS.md"
)

// importFromKiro reads an existing AWS Kiro project and writes specs into
// the configured source directories, reversing the kiro emit:
//
//   - `.kiro/steering/*.md` steering files carry a frontmatter-first
//     `inclusion:` block. The filename prefix picks the kind
//     (`agent-<name>.md` -> agent, `skill-<name>.md` -> skill, otherwise
//     a rule) and a rule's `inclusion:` maps back to scope: `fileMatch`
//     with `fileMatchPattern` becomes a `globs:` rule, `always` an
//     unscoped rule.
//   - `.kiro/settings/mcp.json` (`mcpServers` map) reconstructs MCP specs.
//   - `AGENTS.md` (the shared entry-point Kiro reads directly) mirrors to
//     `.agnostic-ai/AGNOSTIC_AI.md`.
//
// Lossy fields (Kiro's emit cannot carry them, so a round-trip drops them
// without changing Kiro's output): a rule's source-layout scope collapses
// into an equivalent `globs:`; a steering agent keeps only its body (Kiro
// agents hold no description or model); a steering skill keeps only its
// SKILL.md content (bundled sibling assets flatten away on emit).
func importFromKiro(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.MCPs); err != nil {
		return err
	}
	c, err := importKiroSteering(root, src)
	if err != nil {
		return err
	}
	mcps, err := importJSONMCPMap(filepath.Join(root, kiroMCPFile), kiroMCPKey,
		filepath.Join(root, src.MCPs))
	if err != nil {
		return err
	}
	if _, err := mirrorMainFile(root, kiroMainFile); err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d mcps\n",
		c.rules, c.agents, c.skills, mcps)
	printImportNextSteps(root, "kiro")
	return nil
}

// importKiroSteering walks the flat `.kiro/steering/` directory and
// reconstructs one spec per `*.md` steering file. A missing directory
// imports nothing.
func importKiroSteering(root string, src config.Sources) (rulesDirCounts, error) {
	var c rulesDirCounts
	dir := filepath.Join(root, kiroSteeringDir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("read %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if err := importKiroSteeringFile(root, filepath.Join(dir, e.Name()), e.Name(), src, &c); err != nil {
			return c, err
		}
	}
	return c, nil
}

// importKiroSteeringFile reconstructs one steering file into a rule,
// agent, or skill spec based on its filename prefix, parsing the
// frontmatter-first `inclusion:` block the kiro adapter writes.
func importKiroSteeringFile(root, path, filename string, src config.Sources, c *rulesDirCounts) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	meta, body := splitMdcFrontmatter([]byte(header.Strip(string(data))))
	kind, name := classifyRulesDirFile(filename)
	switch kind {
	case "agents":
		out := filepath.Join(root, src.Agents, name+".md")
		if err := writeAgentMD(out, name, "", nil, body); err != nil {
			return err
		}
		c.agents++
	case "skills":
		out := filepath.Join(root, src.Skills, name, "SKILL.md")
		if err := importMkdirAll(filepath.Dir(out), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(out), err)
		}
		desc, _ := meta["description"].(string)
		if err := writeAgentMD(out, name, desc, nil, body); err != nil {
			return err
		}
		c.skills++
	default:
		out := filepath.Join(root, src.Rules, name+".md")
		globs, _ := meta["fileMatchPattern"].(string)
		if err := writeRuleWithGlobs(out, name, globs, body); err != nil {
			return err
		}
		c.rules++
	}
	return nil
}

// writeRuleWithGlobs writes a rule spec with a `name:` and an optional
// `globs:` frontmatter followed by body. Mirrors writeRule but carries
// the glob a kiro `fileMatchPattern` reconstructs, encoded YAML-safely so
// a leading-`*` glob is quoted rather than read back as an alias.
func writeRuleWithGlobs(path, name, globs, body string) error {
	var sb strings.Builder
	sb.WriteString("---\nname: " + name + "\n")
	if globs != "" {
		sb.WriteString(yamlFrontmatterLine("globs", globs))
	}
	sb.WriteString("---\n\n")
	sb.WriteString(strings.TrimRight(body, "\n"))
	sb.WriteString("\n")
	if err := importWriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
