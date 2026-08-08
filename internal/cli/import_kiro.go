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
	kiroAgentsDir   = ".kiro/agents"
	kiroMCPFile     = ".kiro/settings/mcp.json"
	kiroMCPKey      = "mcpServers"
	kiroMainFile    = "AGENTS.md"
)

// importFromKiro reads an existing AWS Kiro project and writes specs into
// the configured source directories, reversing the kiro emit:
//
//   - `.kiro/agents/*.md` native agent profiles carry a frontmatter-first
//     `description`/`model` block (no `name:`; identity comes from the
//     filename, same as spec loading's own fallback). Copied verbatim
//     minus the provenance header, so `model` and any `x-kiro` keys
//     round-trip untouched.
//   - `.kiro/steering/*.md` steering files carry a frontmatter-first
//     `inclusion:` block. The filename prefix picks the kind
//     (`agent-<name>.md` -> agent, `skill-<name>.md` -> skill, otherwise
//     a rule) and a rule's `inclusion:` maps back to scope: `fileMatch`
//     with `fileMatchPattern` becomes a `globs:` rule, `always` an
//     unscoped rule. The `agent-<name>.md` form is the flattened surface
//     this adapter wrote before agents moved to `.kiro/agents/`; still
//     read here for projects synced before that change, and merged with
//     `.kiro/agents/` by name (the native file wins on a collision,
//     since `importKiroAgents` runs second).
//   - `.kiro/settings/mcp.json` (`mcpServers` map) reconstructs MCP specs.
//   - `AGENTS.md` (the shared entry-point Kiro reads directly) mirrors to
//     `.agnostic-ai/AGNOSTIC_AI.md`.
//
// Lossy fields (Kiro's emit cannot carry them, so a round-trip drops them
// without changing Kiro's output): a rule's source-layout scope collapses
// into an equivalent `globs:`; a legacy steering agent keeps only its body
// (the flattened form held no description or model); a steering skill
// keeps only its SKILL.md content (bundled sibling assets flatten away on
// emit); an agent's generic `tools` list re-imports as whatever Kiro
// category name is actually on disk (e.g. `read`), not the Claude-style
// names it collapsed from (`Read`, `Grep`, and `Glob` all emit as `read`
// and are indistinguishable once written), since that many-to-one
// translation (see the kiro adapter's package doc) has no confident
// reverse. Hooks have no import support yet (matches Cursor, which also
// emits hooks natively with no read-back path).
func importFromKiro(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.MCPs); err != nil {
		return err
	}
	c, err := importKiroSteering(root, src)
	if err != nil {
		return err
	}
	agents, err := importKiroAgents(root, filepath.Join(root, src.Agents))
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
		c.rules, c.agents+agents, c.skills, mcps)
	printImportNextSteps(root, "kiro")
	return nil
}

// importKiroAgents copies every native agent profile under
// `.kiro/agents/` into the agents source dir verbatim, stripping the
// agnostic-ai provenance header when present. Kiro's agent frontmatter
// carries no `name:` key, so spec loading's own filename fallback
// recovers the identity; `description`, `model`, and any `x-kiro` keys
// pass through unchanged. A missing directory imports nothing.
func importKiroAgents(root, dstDir string) (int, error) {
	src := filepath.Join(root, kiroAgentsDir)
	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		srcPath := filepath.Join(src, e.Name())
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return count, fmt.Errorf("read %s: %w", srcPath, err)
		}
		dst := filepath.Join(dstDir, e.Name())
		if err := importWriteFile(dst, []byte(header.Strip(string(data))), 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", dst, err)
		}
		count++
	}
	return count, nil
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
