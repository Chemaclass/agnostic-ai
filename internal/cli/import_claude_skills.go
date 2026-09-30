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
)

func preserveClaudeReadonly(existing, imported *yaml.Node) {
	var meta map[string]any
	if existing.Decode(&meta) != nil {
		return
	}
	readonly := meta["readonly"] == true
	if custom, ok := meta["x-claude"].(map[string]any); ok {
		if value, set := custom["readonly"]; set {
			readonly = value == true
		}
	}
	hasTools := mappingHasKey(imported, "disallowedTools")
	for i := 0; i+1 < len(existing.Content); i += 2 {
		if existing.Content[i].Value != "x-claude" {
			continue
		}
		custom := detachClaudeOverride(existing.Content[i+1])
		var kept []*yaml.Node
		if custom.Kind == yaml.MappingNode {
			for j := 0; j+1 < len(custom.Content); j += 2 {
				key, value := custom.Content[j], custom.Content[j+1]
				if key.Value == "readonly" || (key.Value == "disallowedTools" && value.Tag == "!!null" && !hasTools) {
					kept = append(kept, key, value)
				}
			}
		}
		if len(kept) == 0 {
			existing.Content = append(existing.Content[:i], existing.Content[i+2:]...)
		} else {
			custom.Content = kept
			existing.Content[i+1] = custom
		}
		break
	}
	// Removing native restrictions must override the retained portable translation.
	if readonly && !hasTools {
		custom := findOrAppendMapping(existing, "x-claude")
		if !mappingHasKey(custom, "disallowedTools") {
			setMappingValue(custom, "disallowedTools", nil)
		}
	}
}

func detachClaudeOverride(node *yaml.Node) *yaml.Node {
	if node.Kind == yaml.AliasNode {
		return detachClaudeOverride(node.Alias)
	}
	detached := *node
	detached.Anchor = ""
	detached.Content = make([]*yaml.Node, len(node.Content))
	for i, child := range node.Content {
		detached.Content[i] = detachClaudeOverride(child)
	}
	return &detached
}

// importClaudeAgents copies .claude/agents/*.md to dstDir, keeping the
// frontmatter keys only the existing spec declares.
// When the project also carries a Codex installation but the matching
// codex agent is absent there, the captured spec gains `target: claude`
// frontmatter so the next sync does not cross-emit a claude-only agent
// into `.codex/`.
func importClaudeAgents(root, dstDir string, layout claudeLayout) (int, error) {
	src := filepath.Join(root, layout.agents)
	if !dirExists(src) {
		return 0, nil
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	codexPresent := codexTreeExists(root)
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
		out := scopeClaudeModel(header.Strip(string(data)))
		name := strings.TrimSuffix(e.Name(), ".md")
		if codexPresent && !codexHasAgent(root, canonicalSpecSlug(name)) {
			out = addTargetFrontmatter(out, "claude")
		}
		dstPath := filepath.Join(dstDir, e.Name())
		if err := importMkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return count, fmt.Errorf("mkdir %s: %w", filepath.Dir(dstPath), err)
		}
		if err := importWriteSpecMarkdown(dstPath, []byte(out), 0o644, claudeAgentFields); err != nil {
			return count, fmt.Errorf("write %s: %w", dstPath, err)
		}
		count++
	}
	return count, nil
}

// importClaudeSkills copies each `.claude/skills/<name>/` directory tree
// into `<dstDir>/<name>/`. SKILL.md must exist for the
// skill to be considered; once present, every sibling file (and nested
// subdirectories such as `scripts/`, `assets/`, helper Python/JS
// modules, fixtures, etc.) is mirrored verbatim so a roundtrip
// preserves the full skill payload. SKILL.md gains `target: claude`
// frontmatter when the project has a codex tree but no matching codex
// skill (auto-scoping per #299).
func importClaudeSkills(root, dstDir string, layout claudeLayout) (int, error) {
	src := filepath.Join(root, layout.skills)
	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	codexPresent := codexTreeExists(root)
	count := 0
	for _, e := range entries {
		skillSrc, ok := skillFolderSource(root, src, dstDir, e)
		if !ok {
			continue
		}
		if _, err := os.Stat(filepath.Join(skillSrc, "SKILL.md")); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return count, fmt.Errorf("stat skill %s: %w", e.Name(), err)
		}
		skillDst := filepath.Join(dstDir, e.Name())
		if err := copyDirTree(skillSrc, skillDst); err != nil {
			return count, fmt.Errorf("copy skill %s: %w", e.Name(), err)
		}
		if err := moveClaudeOnlyKeysInFile(filepath.Join(skillDst, "SKILL.md")); err != nil {
			return count, err
		}
		if codexPresent && !codexHasSkill(root, e.Name()) {
			if err := injectTargetInSkillMD(filepath.Join(skillDst, "SKILL.md"), "claude"); err != nil {
				return count, fmt.Errorf("scope skill %s: %w", e.Name(), err)
			}
		}
		count++
	}
	return count, nil
}
