package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/claude"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func preserveClaudeReadonly(existing, imported *yaml.Node) {
	var meta map[string]any
	if existing.Decode(&meta) != nil {
		return
	}
	readonly := meta["readonly"] == true
	customMeta, _ := meta["x-claude"].(map[string]any)
	if customMeta != nil {
		if value, set := customMeta["readonly"]; set {
			readonly = value == true
		}
	}
	hasTools := mappingHasKey(imported, "disallowedTools")
	for i := 0; i+1 < len(existing.Content); i += 2 {
		if existing.Content[i].Value != "x-claude" {
			continue
		}
		custom := detachImportOverride(existing.Content[i+1])
		var kept []*yaml.Node
		if custom.Kind == yaml.MappingNode {
			for _, key := range []string{"readonly", "disallowedTools"} {
				if value, set := customMeta[key]; set && !mappingHasKey(custom, key) {
					setMappingValue(custom, key, value)
				}
			}
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
	tiers := importModelTiers(root)
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
		importReadingNative(srcPath)
		dstPath := filepath.Join(dstDir, e.Name())
		scoped, err := importScopedTargetKeys([]byte(header.Strip(string(data))), dstPath, "claude", scopedKeysOwned("claude", dstPath)...)
		if err != nil {
			return count, err
		}
		out := string(scoped)
		if !specScopesKey(dstPath, "claude", "model") {
			out = scopeClaudeModel(out)
		}
		if !specScopesKey(dstPath, "claude", "tools") {
			out = importNeutralToolNames(out, "tools", true)
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		if codexPresent && !codexHasAgent(root, canonicalSpecSlug(name)) {
			out = addTargetFrontmatter(out, "claude")
		}
		if spec, err := os.ReadFile(dstPath); err == nil {
			_, _, meta := claudeFrontmatter(string(spec))
			if meta != nil {
				out = keepClaudeTierModel(out, meta, tiers)
			}
			if meta != nil || !strings.HasPrefix(string(spec), "---\n") {
				out = dropClaudeIdentityFallback(out, meta)
			}
			// An agent file sync wrote and nobody touched holds sync's own
			// widening, so only a hand-edited one gets the note.
			kept := keepScopedClaudeTools(out, header.Strip(string(data)), meta)
			restored := kept != out
			out = kept
			if restored && !unchangedSinceSync(srcPath, string(data)) {
				shown := dstPath
				if rel, inside := pathBelow(root, dstPath); inside {
					shown = rel
				}
				keptf("note: %s keeps its scoped %s; Claude Code gets the plain tools. To widen it for every tool, edit the spec\n", shown, scopedToolsKey(meta))
			}
		}
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

// keepScopedClaudeTools puts back the spec's own `can` or `tools` when
// the native file holds exactly the plain tools sync widened them to, so
// importing an edited agent keeps a pattern other tools may honor.
func keepScopedClaudeTools(doc, native string, existing map[string]any) string {
	key := "can"
	if _, set := existing[key]; !set {
		key = "tools"
	}
	value, set := existing[key]
	if !set {
		return doc
	}
	written, widened := claude.WidenedAgentTools(spec.Entry{Kind: spec.KindAgent, Meta: existing})
	_, _, nativeMeta := claudeFrontmatter(native)
	if !widened || nativeMeta == nil || !slices.Equal(written, claudeToolNames(nativeMeta["tools"])) {
		return doc
	}
	rest, end, imported := claudeFrontmatter(doc)
	if imported == nil {
		return doc
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(rest[:end]), &node); err != nil || len(node.Content) == 0 {
		return doc
	}
	mapping := node.Content[0]
	removeMappingKey(mapping, "can")
	removeMappingKey(mapping, "tools")
	setMappingValue(mapping, key, value)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if enc.Encode(mapping) != nil || enc.Close() != nil {
		return doc
	}
	return "---\n" + buf.String() + rest[end+1:]
}

func scopedToolsKey(existing map[string]any) string {
	if _, set := existing["can"]; set {
		return "can:"
	}
	return "tools:"
}

// claudeToolNames reads a Claude Code `tools` value, a list or a
// comma-separated string, as names.
func claudeToolNames(raw any) []string {
	if s, ok := raw.(string); ok {
		return spec.SplitToolList(s)
	}
	return toStringSlice(raw)
}

// dropClaudeIdentityFallback removes from an imported agent the
// `description` sync writes when the spec has none: one equal to the
// name. Copying it would turn sync's stand-in into the spec's own
// description for every target. A `name` equal to the file stem stays,
// since it is the default anyway.
func dropClaudeIdentityFallback(doc string, existing map[string]any) string {
	rest, end, imported := claudeFrontmatter(doc)
	if imported == nil {
		return doc
	}
	name, _ := imported["name"].(string)
	if declared, _ := existing["description"].(string); strings.TrimSpace(declared) != "" || name == "" || imported["description"] != name {
		return doc
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(rest[:end]), &node); err != nil || len(node.Content) == 0 {
		return doc
	}
	mapping := node.Content[0]
	removeMappingKey(mapping, "description")
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if enc.Encode(mapping) != nil || enc.Close() != nil {
		return doc
	}
	return "---\n" + buf.String() + rest[end+1:]
}

// importClaudeSkills copies each `.claude/skills/<name>/` directory tree
// into `<dstDir>/<name>/`, or updates a flat spec of the same name.
// Once SKILL.md is present, every sibling file (and nested
// subdirectories such as `scripts/`, `assets/`, helper Python/JS
// modules, fixtures, etc.) is mirrored verbatim so a roundtrip
// preserves the full skill payload. SKILL.md gains `target: claude`
// frontmatter when the project has a codex tree but no matching codex
// skill (auto-scoping per #299).
func importClaudeSkills(root, dstDir string, layout claudeLayout, skillRefs map[string]string) (int, error) {
	src := filepath.Join(root, layout.skills)
	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	portableSkills, err := claudePortableSkillPaths(dstDir)
	if err != nil {
		return 0, err
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
		skillPath := filepath.Join(dstDir, e.Name(), "SKILL.md")
		var flatRef string
		if !fileExists(skillPath) {
			matches := portableSkills[e.Name()]
			hasFlat := slices.ContainsFunc(matches, func(path string) bool { return filepath.Base(path) != "SKILL.md" })
			if hasFlat {
				if len(matches) > 1 {
					return count, fmt.Errorf("skill %q matches several portable specs in %s: %s; rename one before importing", e.Name(), dstDir, strings.Join(matches, ", "))
				}
				skillPath = filepath.Join(dstDir, matches[0])
				flatRef = filepath.ToSlash(matches[0])
			}
		}
		if err := copyDirTree(skillSrc, skillPath); err != nil {
			return count, fmt.Errorf("copy skill %s: %w", e.Name(), err)
		}
		skillData, err := os.ReadFile(skillPath)
		if err != nil {
			return count, fmt.Errorf("read %s: %w", skillPath, err)
		}
		neutral := importNeutralToolNames(string(skillData), "allowed-tools", false)
		if neutral != string(skillData) {
			if err := importWriteFile(skillPath, []byte(neutral), 0o644); err != nil {
				return count, err
			}
		}
		if filepath.Base(skillPath) == "SKILL.md" && codexPresent && !codexHasSkill(root, e.Name()) {
			if err := injectTargetInSkillMD(skillPath, "claude"); err != nil {
				return count, fmt.Errorf("scope skill %s: %w", e.Name(), err)
			}
		}
		if flatRef != "" {
			skillRefs[e.Name()] = flatRef
		}
		count++
	}
	return count, nil
}

func claudePortableSkillPaths(dstDir string) (map[string][]string, error) {
	paths := map[string][]string{}
	bundled := map[string]bool{}
	err := spec.WalkSourceRoot(dstDir, func(path string, file fs.DirEntry, err error) error {
		if err != nil {
			if path == dstDir && errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		parent := filepath.Dir(path)
		if file.IsDir() {
			bundled[path] = bundled[parent] || fileExists(filepath.Join(path, "SKILL.md"))
			return nil
		}
		folderSpec := file.Name() == "SKILL.md"
		if !file.Type().IsRegular() || filepath.Ext(file.Name()) != ".md" || (!folderSpec && bundled[parent]) {
			return nil
		}
		rel, inside := pathBelow(dstDir, path)
		if !inside || !filepath.IsLocal(rel) {
			return fmt.Errorf("%s is outside the portable skills directory %s", path, dstDir)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		entry, err := spec.ParseMarkdownBytes(spec.KindSkill, data)
		if err != nil {
			return nil
		}
		name := entry.Name
		if name == "" {
			name = strings.TrimSuffix(file.Name(), ".md")
			if folderSpec {
				name = filepath.Base(parent)
			}
		}
		paths[name] = append(paths[name], rel)
		return nil
	})
	return paths, err
}
