package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

var (
	copilotMainFile        = filepath.Join(".github", "copilot-instructions.md")
	copilotInstructionsDir = filepath.Join(".github", "instructions")
	copilotAgentsDir       = filepath.Join(".github", "agents")
	copilotSkillsDir       = filepath.Join(".github", "skills")
	copilotChatmodesDir    = filepath.Join(".github", "chatmodes")
	copilotMCPFile         = filepath.Join(".vscode", "mcp.json")
)

const (
	copilotInstructionSuffix = ".instructions.md"
	copilotAgentSuffix       = ".agent.md"
	copilotChatmodeSuffix    = ".chatmode.md"
)

// importFromCopilot reads an existing GitHub Copilot project
// (`.github/copilot-instructions.md`, `.github/instructions/`,
// `.github/chatmodes/`, `.vscode/mcp.json`) under root and writes
// specs into the configured source directories.
func importFromCopilot(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.MCPs); err != nil {
		return err
	}
	counts, err := importCopilotRules(root, src)
	if err != nil {
		return err
	}
	agents, err := importCopilotAgents(root, filepath.Join(root, src.Agents))
	if err != nil {
		return err
	}
	skills, err := importSkillFolders(filepath.Join(root, copilotSkillsDir), filepath.Join(root, src.Skills))
	if err != nil {
		return err
	}
	chatmodes, err := importCopilotChatmodes(root, filepath.Join(root, src.Agents))
	if err != nil {
		return err
	}
	mcps, err := importCopilotMCP(root, filepath.Join(root, src.MCPs))
	if err != nil {
		return err
	}
	if _, err := mirrorMainFile(root, copilotMainFile); err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d mcps\n",
		counts.rules, counts.agents+agents+chatmodes, counts.skills+skills, mcps)
	printImportNextSteps(root, "copilot")
	return nil
}

// importCopilotAgents copies every native agent profile under
// `.github/agents/` into the agents source dir. Both documented
// filename forms are accepted (`<name>.agent.md` and `<name>.md`); the
// `.agent` infix is dropped so the spec lands at `<agents>/<name>.md`.
// The agnostic-ai provenance header is stripped when present.
func importCopilotAgents(root, dstDir string) (int, error) {
	src := filepath.Join(root, copilotAgentsDir)
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
		name := strings.TrimSuffix(strings.TrimSuffix(e.Name(), ".md"), ".agent")
		srcPath := filepath.Join(src, e.Name())
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return count, fmt.Errorf("read %s: %w", srcPath, err)
		}
		dst := filepath.Join(dstDir, name+".md")
		if err := importWriteFile(dst, []byte(header.Strip(string(data))), 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", dst, err)
		}
		count++
	}
	return count, nil
}

type copilotCounts struct{ rules, agents, skills int }

// importCopilotRules prefers `.github/instructions/*.instructions.md`
// (one file becomes one rule, agent, or skill depending on filename
// prefix; `applyTo:` frontmatter translates to `globs:`). Falls back
// to slicing `.github/copilot-instructions.md` on `## ` headings when
// no instructions dir exists.
func importCopilotRules(root string, src config.Sources) (copilotCounts, error) {
	var c copilotCounts
	instrDir := filepath.Join(root, copilotInstructionsDir)
	if dirExists(instrDir) {
		return importCopilotInstructions(instrDir, root, src)
	}
	n, err := sliceMainFileByH2(root, copilotMainFile, filepath.Join(root, src.Rules))
	if err != nil {
		return c, err
	}
	c.rules = n
	return c, nil
}

// importCopilotInstructions reads every *.instructions.md under src
// and routes it by filename prefix:
//   - `agent-<name>.instructions.md` → <agents>/<name>.md
//   - `skill-<name>.instructions.md` → <skills>/<name>.md
//   - `<name>.instructions.md`       → <rules>/<name>.md
//
// `applyTo` (Copilot's path glob) translates to `globs`. The catch-all
// `**` is dropped since it carries no scope.
func importCopilotInstructions(src, root string, sources config.Sources) (copilotCounts, error) {
	var c copilotCounts
	walkErr := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), copilotInstructionSuffix) {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		base := strings.TrimSuffix(d.Name(), copilotInstructionSuffix)
		kind, name := classifyRulesDirFile(base + ".md")
		dstDir := pickKindDir(kind, sources)
		translated, err := translateCopilotInstruction(name, data)
		if err != nil {
			return fmt.Errorf("translate %s: %w", rel, err)
		}
		out := filepath.Join(root, dstDir, scopeDir(rel), name+".md")
		if err := importMkdirAll(filepath.Dir(out), 0o755); err != nil {
			return fmt.Errorf("%s: %w", filepath.Dir(out), err)
		}
		if err := importWriteFile(out, translated, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", out, err)
		}
		switch kind {
		case "agents":
			c.agents++
		case "skills":
			c.skills++
		default:
			c.rules++
		}
		return nil
	})
	if errors.Is(walkErr, fs.ErrNotExist) {
		return copilotCounts{}, nil
	}
	if walkErr != nil {
		return c, walkErr
	}
	return c, nil
}

// translateCopilotInstruction rewrites a Copilot `.instructions.md`
// file as an agnostic rule. `applyTo:` becomes `globs:` (the catch-all
// `**` is dropped). A `name:` field is injected from the filename when
// absent. A leading single-line italic paragraph (the form the emitter
// writes for `description:`) is lifted out of the body into the
// `description:` frontmatter so a round-trip is loss-free.
func translateCopilotInstruction(name string, data []byte) ([]byte, error) {
	data = []byte(header.Strip(string(data)))
	meta, body := splitMdcFrontmatter(data)
	if _, ok := meta["name"]; !ok {
		meta["name"] = name
	}
	if applyTo, ok := meta["applyTo"].(string); ok {
		delete(meta, "applyTo")
		if applyTo != "" && applyTo != "**" {
			if _, exists := meta["globs"]; !exists {
				meta["globs"] = applyTo
			}
		}
	}
	if desc, stripped, ok := extractLeadingItalic(body); ok {
		if _, exists := meta["description"]; !exists {
			meta["description"] = desc
		}
		body = stripped
	}
	var fm strings.Builder
	enc := yaml.NewEncoder(&fm)
	enc.SetIndent(2)
	if err := enc.Encode(meta); err != nil {
		return nil, fmt.Errorf("marshal frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("close encoder: %w", err)
	}
	var out strings.Builder
	out.WriteString("---\n")
	out.WriteString(fm.String())
	out.WriteString("---\n\n")
	out.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		out.WriteString("\n")
	}
	return []byte(out.String()), nil
}

// importCopilotChatmodes reads `.github/chatmodes/*.chatmode.md` and
// writes one agent spec per chatmode into dstDir.
func importCopilotChatmodes(root, dstDir string) (int, error) {
	src := filepath.Join(root, copilotChatmodesDir)
	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), copilotChatmodeSuffix) {
			continue
		}
		full := filepath.Join(src, e.Name())
		data, err := os.ReadFile(full)
		if err != nil {
			return count, fmt.Errorf("read %s: %w", full, err)
		}
		name := strings.TrimSuffix(e.Name(), copilotChatmodeSuffix)
		data = []byte(header.Strip(string(data)))
		meta, body := splitMdcFrontmatter(data)
		if _, ok := meta["name"]; !ok {
			meta["name"] = name
		}
		translated, err := writeFrontmatter(meta, body)
		if err != nil {
			return count, fmt.Errorf("translate %s: %w", e.Name(), err)
		}
		out := filepath.Join(dstDir, name+".md")
		if err := importWriteFile(out, translated, 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", out, err)
		}
		count++
	}
	return count, nil
}

// writeFrontmatter re-marshals meta + body as a markdown file with
// `---` frontmatter delimiters. Generic helper for translated specs
// where keys are not known up front.
func writeFrontmatter(meta map[string]any, body string) ([]byte, error) {
	var fm strings.Builder
	enc := yaml.NewEncoder(&fm)
	enc.SetIndent(2)
	if err := enc.Encode(meta); err != nil {
		return nil, fmt.Errorf("marshal frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("close encoder: %w", err)
	}
	var out strings.Builder
	out.WriteString("---\n")
	out.WriteString(fm.String())
	out.WriteString("---\n\n")
	out.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		out.WriteString("\n")
	}
	return []byte(out.String()), nil
}

// importCopilotMCP reads `.vscode/mcp.json` (VS Code's MCP shape:
// `{servers: {name: {...}}}`) and writes one yaml per server.
func importCopilotMCP(root, dstDir string) (int, error) {
	return importJSONMCPMap(filepath.Join(root, copilotMCPFile), "servers", dstDir)
}
