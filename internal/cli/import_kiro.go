package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	kiroSteeringDir = ".kiro/steering"
	kiroAgentsDir   = ".kiro/agents"
	kiroSkillsDir   = ".kiro/skills"
	kiroCommandsDir = ".kiro/prompts"
	kiroMCPFile     = ".kiro/settings/mcp.json"
	kiroMCPKey      = "mcpServers"
	kiroMainFile    = "AGENTS.md"
)

// importFromKiro reads an existing AWS Kiro project and writes specs into
// the configured source directories, reversing the kiro emit:
//
//   - `.kiro/agents/**/*.md` and `*.json` native agent profiles carry a
//     frontmatter-first block (JSON keys map the same way; `prompt` is the
//     body). A nested path joins with hyphens. The path remains the canonical spec name; an explicit
//     native `name` display value imports as `x-kiro.name` so the two
//     identities survive a round-trip.
//   - `.kiro/skills/<name>/SKILL.md` native skill folders copy
//     byte-for-byte via importSkillFolders, so bundled sibling assets
//     (`scripts/`, `references/`, `assets/`) round-trip along with
//     SKILL.md, the same shared helper cursor/gemini/opencode/copilot use.
//   - `.kiro/steering/*.md` steering files carry a frontmatter-first
//     `inclusion:` block. The filename prefix picks the kind
//     (`agent-<name>.md` -> agent, `skill-<name>.md` -> skill, otherwise
//     a rule) and a rule's `inclusion:` maps back to scope: `fileMatch`
//     with `fileMatchPattern` becomes a `globs:` rule, `always` an
//     unscoped rule. The `agent-<name>.md` and `skill-<name>.md` forms
//     are the flattened surfaces this adapter wrote before agents and
//     skills moved to their native `.kiro/agents/` and `.kiro/skills/`
//     trees; still read here for projects synced before those changes,
//     and merged with the native trees by name (the native copy wins on
//     a collision, since `importKiroAgents` and `importSkillFolders` for
//     `.kiro/skills/` both run after `importKiroSteering`).
//   - `.kiro/hooks/*.json` reconstructs one hook spec per group of
//     entries that differ only in `action.command`, splitting a file
//     that carries several `trigger` values. See import_kiro_hooks.go.
//   - `.kiro/prompts/*.md` copies command bodies and argument templates
//     from the configured native output directory.
//   - `.kiro/settings/mcp.json` (`mcpServers` map) reconstructs MCP specs.
//   - a hand-authored `.kiroignore` reconstructs an ignore spec (#754).
//   - `AGENTS.md` (the shared entry-point Kiro reads directly) mirrors to
//     `.agnostic-ai/AGNOSTIC_AI.md`.
//
// Lossy fields (Kiro's emit cannot carry them, so a round-trip drops them
// without changing Kiro's output): a rule's source-layout scope collapses
// into an equivalent `globs:`; a legacy steering agent or skill keeps only
// its body (the flattened forms held no description/model, and no bundled
// assets). Native tool names become capabilities only when the reverse
// mapping is exact. A hook's explicit `enabled: true` leaves no key, since that
// is the vendor default the emit side writes nothing for, and a hook
// name declared by two files keeps only one of the two labels, since a
// spec name is unique across the bundle.
func importFromKiro(root string, cfg *config.Config) error {
	src := cfg.Sources
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.Hooks, src.MCPs, src.Commands); err != nil {
		return err
	}
	c, err := importKiroSteering(root, src)
	if err != nil {
		return err
	}
	projectServers, err := readJSONMapAt(filepath.Join(root, kiroMCPFile), kiroMCPKey)
	if err != nil {
		return err
	}
	projectServers, oauthSkipped := withoutMCPOAuthCredentials(projectServers)
	agents, err := importKiroAgents(root, importSourcePath(root, src.Agents), projectServers)
	if err != nil {
		return err
	}
	skills, err := importSkillFolders(root, filepath.Join(root, kiroSkillsDir), importSourcePath(root, src.Skills))
	if err != nil {
		return err
	}
	hooks, err := importKiroHooks(root, importSourcePath(root, src.Hooks))
	if err != nil {
		return err
	}
	commands, err := importKiroCommands(root, importSourcePath(root, src.Commands), cfg)
	if err != nil {
		return err
	}
	mcps, err := writeMCPYAMLs("kiro", projectServers, importSourcePath(root, src.MCPs))
	if err != nil {
		return err
	}
	reportMCPLiteralRefs(oauthSkipped)
	ignores, err := importIgnoreFile(root, "kiro", src)
	if err != nil {
		return err
	}
	if _, err := mirrorMainFile(root, kiroMainFile); err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d hooks, %d mcps, %d commands, %d ignores\n",
		c.rules, c.agents+agents, c.skills+skills, hooks, mcps, commands, ignores)
	return nil
}

func importKiroCommands(root, dstDir string, cfg *config.Config) (int, error) {
	dir := kiroCommandsDir
	for _, artifact := range adapters.NativeArtifactsFor("kiro", cfg) {
		if artifact.Label == "Commands" {
			dir = filepath.Clean(filepath.FromSlash(strings.TrimSuffix(artifact.Location, "/")))
			break
		}
	}
	src := config.ResolveSourcePath(root, dir)
	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(src, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return count, fmt.Errorf("read %s: %w", path, err)
		}
		importReadingNative(path)
		body, generated := strings.CutPrefix(string(data), header.Line(header.FormatMarkdown)+"\n")
		if !generated {
			body = header.Strip(string(data))
		}
		if strings.HasPrefix(body, "---") {
			name := strings.TrimSuffix(entry.Name(), ".md")
			body = "---\n" + yamlFrontmatterLine("name", name) + "---\n" + body
		}
		dst := filepath.Join(dstDir, entry.Name())
		if err := importMkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return count, fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
		}
		// A Kiro prompt is only a body, so it carries no spec key.
		if err := importWriteMarkdownCarrying(dst, []byte(body), 0o644, importCarrying(nil)); err != nil {
			return count, fmt.Errorf("write %s: %w", dst, err)
		}
		count++
	}
	return count, nil
}

// importKiroAgents copies every native agent profile under
// `.kiro/agents/` into the agents source dir: `.md` and `.json` files,
// at the top or in subfolders. A nested file `team/planner.md` becomes
// the spec `team-planner`, since a spec name is one path segment. An
// explicit native `name` becomes `x-kiro.name`, leaving the path as the
// canonical spec name. An inline `mcpServers` map is read by
// importKiroAgentServers. A JSON profile is converted to the Markdown
// form first (its `prompt` becomes the body), so one mapping serves
// both. A JSON profile that cannot be read as one is skipped with a
// note. When two files map to one spec name, the first in this order
// wins and the rest are reported: top-level `.md`, top-level `.json`,
// subfolder `.md`, then subfolder `.json`. A missing directory imports
// nothing.
func importKiroAgents(root, dstDir string, projectServers map[string]any) (int, error) {
	src := filepath.Join(root, kiroAgentsDir)
	if _, err := os.Stat(src); errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	files, err := kiroAgentFiles(src)
	if err != nil {
		return 0, err
	}
	type winner struct {
		rel       string
		topMD     bool
		generated bool
	}
	imported := map[string]winner{}
	count := 0
	for _, f := range files {
		rel := filepath.ToSlash(filepath.Join(kiroAgentsDir, f.rel))
		if first, taken := imported[f.name]; taken {
			if first.topMD && first.generated && (f.json || f.nested) {
				keptf("note: %s already imported as %s, which sync wrote, so the original can be removed (%s)\n", rel, first.rel, f.leftoverEffect())
			} else {
				keptf("note: skipped %s: spec agent %s already imported from %s; rename one of them to import both\n", rel, f.name, first.rel)
			}
			continue
		}
		data, err := os.ReadFile(f.path)
		if err != nil {
			return count, fmt.Errorf("read %s: %w", f.path, err)
		}
		generated := header.Has(string(data))
		if f.json {
			data, err = kiroJSONAgentToMarkdown(data, filepath.Dir(f.path), root)
			if err != nil {
				keptf("note: skipped %s: %v\n", rel, err)
				continue
			}
		}
		dst := filepath.Join(dstDir, f.name+".md")
		shared := projectServers
		// A spec that scopes its servers to Kiro keeps them as a map: a
		// list of names there would bypass sync's server expansion.
		if specScopesKiroServers(dst) {
			shared = nil
		}
		translated, err := importKiroAgentDocument([]byte(header.Strip(string(data))), f.name, shared)
		if err != nil {
			return count, fmt.Errorf("translate %s: %w", f.path, err)
		}
		if err := importWriteSpecMarkdown(dst, translated, 0o644, kiroAgentFields); err != nil {
			return count, fmt.Errorf("write %s: %w", dst, err)
		}
		imported[f.name] = winner{rel: rel, topMD: !f.json && !f.nested, generated: generated}
		count++
		if f.json || f.nested {
			keptf("note: imported %s as spec agent %s; sync writes %s, so remove the original (%s)\n",
				rel, f.name, filepath.ToSlash(filepath.Join(kiroAgentsDir, f.name+".md")), f.leftoverEffect())
		}
	}
	return count, nil
}

type kiroAgentFile struct {
	path, rel, name string
	json, nested    bool
}

// leftoverEffect says what Kiro does when the original stays beside the
// file sync writes.
func (f kiroAgentFile) leftoverEffect() string {
	if f.nested {
		return fmt.Sprintf("Kiro would list two agents, %s and %s", strings.TrimSuffix(filepath.ToSlash(f.rel), filepath.Ext(f.rel)), f.name)
	}
	return fmt.Sprintf("Kiro would read both files as the agent %s and they would clash", f.name)
}

// kiroAgentFiles lists agent files under dir in import precedence order.
func kiroAgentFiles(dir string) ([]kiroAgentFile, error) {
	var files []kiroAgentFile
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(d.Name())
		if ext != ".md" && ext != ".json" {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		slashed := filepath.ToSlash(rel)
		files = append(files, kiroAgentFile{
			path:   path,
			rel:    rel,
			name:   strings.ReplaceAll(strings.TrimSuffix(slashed, ext), "/", "-"),
			json:   ext == ".json",
			nested: strings.Contains(slashed, "/"),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	rank := func(f kiroAgentFile) int {
		r := 0
		if f.nested {
			r = 2
		}
		if f.json {
			r++
		}
		return r
	}
	sort.SliceStable(files, func(i, j int) bool { return rank(files[i]) < rank(files[j]) })
	return files, nil
}

// kiroJSONAgentToMarkdown rewrites a JSON agent profile as the Markdown
// form Kiro documents as equivalent: every key but `prompt` becomes
// frontmatter and `prompt` becomes the body. Keys keep their order.
// `prompt` is a string, null (empty), or a `file://` URI read relative
// to the profile's folder; the file must sit inside the project.
func kiroJSONAgentToMarkdown(data []byte, dir, root string) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		if err == nil {
			err = errors.New("agent profile is not a JSON object")
		}
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	mapping := &yaml.Node{Kind: yaml.MappingNode}
	prompt := ""
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		key, _ := tok.(string)
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		value, err := decodeJSONValue(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		if key == "prompt" {
			if prompt, err = kiroPromptText(value, dir, root); err != nil {
				return nil, err
			}
			continue
		}
		var node yaml.Node
		if err := node.Encode(value); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &node)
	}
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	front, err := yaml.Marshal(mapping)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	out := "---\n" + string(front) + "---\n"
	if prompt != "" {
		out += "\n" + strings.TrimRight(prompt, "\n") + "\n"
	}
	return []byte(out), nil
}

// decodeJSONValue decodes one JSON value, keeping whole numbers whole.
func decodeJSONValue(raw json.RawMessage) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return plainJSONNumbers(v), nil
}

func plainJSONNumbers(v any) any {
	switch x := v.(type) {
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		f, _ := x.Float64()
		return f
	case []any:
		for i := range x {
			x[i] = plainJSONNumbers(x[i])
		}
	case map[string]any:
		for k := range x {
			x[k] = plainJSONNumbers(x[k])
		}
	}
	return v
}

func kiroPromptText(value any, dir, root string) (string, error) {
	switch p := value.(type) {
	case nil:
		return "", nil
	case string:
		uri, isFile := strings.CutPrefix(p, "file://")
		if !isFile {
			return p, nil
		}
		path := filepath.FromSlash(uri)
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return "", fmt.Errorf("prompt file %s cannot be read", uri)
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("prompt file %s is outside the project", uri)
		}
		if _, inside := pathBelow(resolveSymlinks(absRoot), resolveSymlinks(absPath)); !inside {
			return "", fmt.Errorf("prompt file %s is outside the project", uri)
		}
		text, err := os.ReadFile(absPath)
		if err != nil {
			return "", fmt.Errorf("prompt file %s cannot be read", uri)
		}
		return string(text), nil
	}
	return "", errors.New("prompt must be a string or a file:// URI")
}

func importKiroAgentDocument(data []byte, canonicalName string, projectServers map[string]any) ([]byte, error) {
	var err error
	data, err = importNativeAgentTools(data, "kiro", "tools")
	if err != nil {
		return nil, err
	}
	data, err = importNativeAgentFields(data, "kiro", "excludedTools", "includeMcpJson", "includePowers", "resources", "permissions", "welcomeMessage", "hooks")
	if err != nil {
		return nil, err
	}
	mapping, rest, ok := frontmatterNode(data)
	if !ok {
		return data, nil
	}
	servers, err := importKiroAgentServers(mapping, projectServers)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", canonicalName, err)
	}
	at := -1
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == "name" && mapping.Content[i+1].Kind == yaml.ScalarNode && mapping.Content[i+1].Value != "" {
			at = i
		}
	}
	if at < 0 && !servers {
		return data, nil
	}
	if at >= 0 {
		display := mapping.Content[at+1]
		mapping.Content = append(mapping.Content[:at], mapping.Content[at+2:]...)
		setMappingNode(xTargetMapping(mapping, "x-kiro"), "name", display)
	}
	out, err := encodeFrontmatter(mapping, rest, "kiro")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", canonicalName, err)
	}
	return out, nil
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
// frontmatter-first `inclusion:` block the kiro adapter writes. The
// agent and skill cases only ever see the legacy flattened forms this
// adapter no longer writes (see importFromKiro); the native
// `.kiro/agents/` and `.kiro/skills/` trees import separately and win
// on a name collision.
func importKiroSteeringFile(root, path, filename string, src config.Sources, c *rulesDirCounts) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	meta, body := splitMdcFrontmatter([]byte(header.Strip(string(data))))
	kind, name := classifyRulesDirFile(filename, data)
	switch kind {
	case "agents":
		out := filepath.Join(importSourcePath(root, src.Agents), name+".md")
		if err := writeAgentMD(out, name, "", nil, body); err != nil {
			return err
		}
		c.agents++
	case "skills":
		out := filepath.Join(importSourcePath(root, src.Skills), name, "SKILL.md")
		if err := importMkdirAll(filepath.Dir(out), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(out), err)
		}
		desc, _ := meta["description"].(string)
		if err := writeAgentMD(out, name, desc, nil, body); err != nil {
			return err
		}
		c.skills++
	default:
		// A rule spec has no on-demand mode, so importing one would make
		// sync rewrite it as `inclusion: always`.
		out := filepath.Join(importSourcePath(root, src.Rules), name+".md")
		if mode, _ := meta["inclusion"].(string); mode == "manual" || mode == "auto" {
			if _, err := os.Stat(out); err == nil {
				summaryf("  ! skipped %s: inclusion: %s has no rule equivalent, and sync will overwrite it with rule %s; rename one of them\n",
					filepath.ToSlash(path), mode, filepath.ToSlash(out))
				return nil
			}
			summaryf("  ! skipped %s: inclusion: %s has no rule equivalent; keep it as a hand-written steering file\n",
				filepath.ToSlash(path), mode)
			return nil
		}
		// A list fileMatchPattern reads back as the comma form sync splits.
		globs := spec.JoinGlobs(meta["fileMatchPattern"])
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
