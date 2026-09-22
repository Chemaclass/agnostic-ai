package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

const (
	openhandsMainFile  = "AGENTS.md"
	openhandsAgentsDir = ".agents/agents"
	// openhandsSkillsDir is the location OpenHands recommends; the two
	// legacy trees "remain supported", and ".agents/skills/ takes
	// precedence over the legacy directories"
	// (docs.openhands.dev/overview/skills).
	openhandsSkillsDir     = ".agents/skills"
	openhandsHooksFile     = ".openhands/hooks.json"
	openhandsSetupFile     = ".openhands/setup.sh"
	openhandsMCPFile       = "config.toml"
	openhandsSetupSpecName = "openhands-setup"
	openhandsNativeMetaKey = "x-openhands"
)

// openhandsLegacySkillDirs follow openhandsSkillsDir in precedence
// order.
var openhandsLegacySkillDirs = []string{".openhands/skills", ".openhands/microagents"}

// openhandsLegacyKeys are microagent frontmatter keys from before
// Agent Skills. OpenHands infers the same behavior from `triggers` and
// `paths` today, so they carry nothing a spec needs.
var openhandsLegacyKeys = map[string]bool{"name": true, "type": true, "version": true, "agent": true}

// importFromOpenhands reads an existing OpenHands project and writes
// specs into the configured source directories, reversing the
// openhands emit:
//
//   - `AGENTS.md` carries always-on rules inlined in a sentinel-marked
//     block; each `### <name>` child becomes a rule, and the file
//     mirrors to `.agnostic-ai/AGNOSTIC_AI.md`.
//   - `.agents/skills/<name>/SKILL.md` becomes a skill, or a rule when
//     its frontmatter has `paths` (a path-triggered rule). The legacy
//     `.openhands/skills/` and `.openhands/microagents/` trees follow,
//     and a flat `<name>.md` in any of the three becomes a rule, or a
//     keyword skill when it carries `triggers`.
//   - `.agents/agents/*.md` becomes agents, the tree Goose shares.
//   - `.openhands/hooks.json` becomes hook specs, in either the native
//     snake_case layout or the Claude-compatible one sync writes.
//   - `config.toml` `[mcp]` becomes MCP specs.
//   - `.openhands/setup.sh` becomes one environment spec.
//
// Lossy fields: an `[mcp]` sse or shttp entry has no name, so import
// derives one from its URL host; a rule's source-layout scope comes
// back as the `paths` glob it was widened to.
func importFromOpenhands(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.Hooks, src.MCPs, src.Environments); err != nil {
		return err
	}
	rules, err := sliceEntryPointRules(root, openhandsMainFile, filepath.Join(root, src.Rules))
	if err != nil {
		return err
	}
	skillRules, skills, err := importOpenhandsSkillTrees(root, src)
	if err != nil {
		return err
	}
	agents, err := importFlatMarkdownFiles(filepath.Join(root, openhandsAgentsDir), filepath.Join(root, src.Agents), gooseAgentFields)
	if err != nil {
		return err
	}
	hooks, err := importOpenhandsHooks(filepath.Join(root, openhandsHooksFile), filepath.Join(root, src.Hooks))
	if err != nil {
		return err
	}
	mcps, err := importOpenhandsMCP(filepath.Join(root, openhandsMCPFile), filepath.Join(root, src.MCPs))
	if err != nil {
		return err
	}
	envs, err := importOpenhandsSetup(filepath.Join(root, openhandsSetupFile), filepath.Join(root, src.Environments))
	if err != nil {
		return err
	}
	if _, err := mirrorMainFile(root, openhandsMainFile); err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d hooks, %d mcps, %d environments\n",
		rules+skillRules, agents, skills, hooks, mcps, envs)
	printImportNextSteps(root, "openhands")
	return nil
}

// importOpenhandsSkillTrees walks the three skill directories in
// precedence order. A name taken by an earlier directory is skipped in
// later ones, whatever kind it imported as.
func importOpenhandsSkillTrees(root string, src config.Sources) (rules, skills int, err error) {
	seen := map[string]bool{}
	for _, dir := range append([]string{openhandsSkillsDir}, openhandsLegacySkillDirs...) {
		r, s, err := importOpenhandsSkillDir(filepath.Join(root, filepath.FromSlash(dir)), root, src, seen)
		if err != nil {
			return rules, skills, err
		}
		rules += r
		skills += s
	}
	return rules, skills, nil
}

// importOpenhandsSkillDir imports one skill directory. Flat files and
// path-triggered skill folders are classified first; the remaining
// folders then copy as ordinary skills.
func importOpenhandsSkillDir(dir, root string, src config.Sources, seen map[string]bool) (rules, skills int, err error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("read %s: %w", dir, err)
	}
	for _, e := range entries {
		name, file := strings.TrimSuffix(e.Name(), ".md"), filepath.Join(dir, e.Name())
		if e.IsDir() {
			name, file = e.Name(), filepath.Join(dir, e.Name(), "SKILL.md")
		} else if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if seen[name] {
			continue
		}
		data, err := os.ReadFile(file)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return rules, skills, fmt.Errorf("read %s: %w", file, err)
		}
		meta, body := splitMdcFrontmatter([]byte(header.Strip(string(data))))
		switch openhandsSkillKind(meta, e.IsDir()) {
		case "skill":
			continue // copied with its assets below
		case "rule":
			err = writeOpenhandsRule(filepath.Join(root, src.Rules, name+".md"), name, meta, body)
			rules++
		default:
			err = writeOpenhandsKeywordSkill(filepath.Join(root, src.Skills, name, "SKILL.md"), name, meta, body)
			skills++
		}
		if err != nil {
			return rules, skills, err
		}
		seen[name] = true
	}
	n, err := importSkillFoldersWith(dir, filepath.Join(root, src.Skills), skillFolderImportOpts{SkipNames: seen})
	return rules, skills + n, err
}

// openhandsSkillKind classifies one skill file the way OpenHands loads
// it: `paths` makes a path-triggered rule and wins over `triggers`
// (docs.openhands.dev/overview/skills/path); a folder is otherwise an
// Agent Skill; a flat file with `triggers` is a keyword skill; and "a
// legacy `.md` skill without a trigger is always loaded in full"
// (docs.openhands.dev/overview/skills), which is a rule.
func openhandsSkillKind(meta map[string]any, folder bool) string {
	switch {
	case len(openhandsPaths(meta["paths"])) > 0:
		return "rule"
	case folder:
		return "skill"
	case meta["triggers"] != nil:
		return "keyword"
	default:
		return "rule"
	}
}

// openhandsPaths normalizes `paths`, which OpenHands accepts as a YAML
// list or a comma-separated string.
func openhandsPaths(v any) []string {
	var raw []string
	if s, ok := v.(string); ok {
		raw = strings.Split(s, ",")
	} else {
		raw = stringSliceFromAny(v)
	}
	var out []string
	for _, p := range raw {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// openhandsSpecMeta splits native frontmatter into the spec's own keys
// and the rest, which lands under x-openhands so the emitter writes it
// back. Legacy microagent keys and the named portable keys drop out.
func openhandsSpecMeta(name string, meta map[string]any, portable ...string) map[string]any {
	out := map[string]any{"name": name}
	if desc, _ := meta["description"].(string); desc != "" {
		out["description"] = desc
	}
	native := map[string]any{}
	for k, v := range meta {
		if k == "description" || openhandsLegacyKeys[k] || slices.Contains(portable, k) {
			continue
		}
		native[k] = v
	}
	if len(native) > 0 {
		out[openhandsNativeMetaKey] = native
	}
	return out
}

// writeOpenhandsRule writes a rule spec. A path-triggered rule keeps its
// globs as `paths` and its other native keys under x-openhands, which
// the emitter merges back into the SKILL.md. An always-on rule reaches
// OpenHands through AGENTS.md, which has no frontmatter, so its native
// keys are dropped.
func writeOpenhandsRule(path, name string, meta map[string]any, body string) error {
	paths := openhandsPaths(meta["paths"])
	doc := map[string]any{"name": name}
	if len(paths) > 0 {
		doc = openhandsSpecMeta(name, meta, "paths")
		doc["paths"] = paths
	} else if desc, _ := meta["description"].(string); desc != "" {
		doc["description"] = desc
	}
	return writeOpenhandsSpec(path, doc, body)
}

// writeOpenhandsKeywordSkill writes a flat keyword-triggered file as a
// skill folder. `triggers` has no portable spelling, so it stays under
// x-openhands with any other native key.
func writeOpenhandsKeywordSkill(path, name string, meta map[string]any, body string) error {
	return writeOpenhandsSpec(path, openhandsSpecMeta(name, meta), body)
}

func writeOpenhandsSpec(path string, meta map[string]any, body string) error {
	out, err := writeFrontmatter(meta, strings.TrimSpace(body))
	if err != nil {
		return fmt.Errorf("render %s: %w", path, err)
	}
	if err := importMkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := importWriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// openhandsHookEntry is one command in a matcher group. OpenHands reads
// `async` beside the shared `{type, command, timeout}` object.
type openhandsHookEntry struct {
	groupedHookEntry
	Async bool `json:"async"`
}

type openhandsHookGroup struct {
	Matcher string               `json:"matcher"`
	Hooks   []openhandsHookEntry `json:"hooks"`
}

// importOpenhandsHooks reads `.openhands/hooks.json` and writes one hook
// spec per matcher group. The native layout keys events in snake_case
// (`pre_tool_use`); spec events are PascalCase, the spelling sync
// writes and OpenHands accepts as well.
func importOpenhandsHooks(src, dstDir string) (int, error) {
	byEvent, err := readEventKeyedHooks[openhandsHookGroup](src)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, key := range sortedHookEvents(byEvent) {
		event := pascalHookEvent(key)
		for _, g := range byEvent[key] {
			entries := make([]groupedHookEntry, 0, len(g.Hooks))
			var extra map[string]any
			for _, h := range g.Hooks {
				entries = append(entries, h.groupedHookEntry)
				if h.Async {
					extra = map[string]any{"async": true}
				}
			}
			n, err := writeGroupedHookSpec(dstDir, "openhands", event, g.Matcher, entries, extra)
			if err != nil {
				return count, err
			}
			count += n
		}
	}
	return count, nil
}

// pascalHookEvent turns `pre_tool_use` into `PreToolUse`. A key already
// in PascalCase passes through.
func pascalHookEvent(key string) string {
	var sb strings.Builder
	for _, part := range strings.Split(key, "_") {
		if part == "" {
			continue
		}
		sb.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return sb.String()
}

// importOpenhandsMCP reads the `[mcp]` table of `config.toml` and writes
// one MCP spec per server. Other tables in the file are not ours to
// read. A stdio server carries its own `name`; an sse or shttp element
// is a URL string or a `{ url, api_key, timeout }` table with no name,
// so the name comes from the URL host.
func importOpenhandsMCP(src, dstDir string) (int, error) {
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	var doc struct {
		MCP struct {
			Stdio []map[string]any `toml:"stdio_servers"`
			SSE   []any            `toml:"sse_servers"`
			SHTTP []any            `toml:"shttp_servers"`
		} `toml:"mcp"`
	}
	if _, err := toml.Decode(string(data), &doc); err != nil {
		return 0, fmt.Errorf("parse %s: %w", src, err)
	}
	servers := map[string]any{}
	for _, s := range doc.MCP.Stdio {
		name, _ := s["name"].(string)
		if name == "" {
			continue
		}
		entry := map[string]any{}
		for k, v := range s {
			if k != "name" {
				entry[k] = v
			}
		}
		servers[name] = entry
	}
	for _, bucket := range []struct {
		transport string
		elements  []any
	}{{"sse", doc.MCP.SSE}, {"http", doc.MCP.SHTTP}} {
		for _, el := range bucket.elements {
			entry := openhandsRemoteEntry(el)
			rawURL, _ := entry["url"].(string)
			if rawURL == "" {
				continue
			}
			entry["type"] = bucket.transport
			servers[uniqueMCPName(servers, mcpNameFromURL(rawURL))] = entry
		}
	}
	return writeMCPYAMLs(servers, dstDir)
}

// openhandsRemoteEntry reads one sse/shttp element in either documented
// form.
func openhandsRemoteEntry(el any) map[string]any {
	switch v := el.(type) {
	case string:
		return map[string]any{"url": v}
	case map[string]any:
		entry := map[string]any{}
		for k, val := range v {
			entry[k] = val
		}
		return entry
	}
	return map[string]any{}
}

// mcpNameFromURL derives a server name from a URL host:
// `https://docs.example.test/sse` becomes `docs-example-test`.
func mcpNameFromURL(rawURL string) string {
	host := rawURL
	if u, err := url.Parse(rawURL); err == nil && u.Hostname() != "" {
		host = u.Hostname()
	}
	name := strings.Trim(strings.NewReplacer(".", "-", "/", "-", ":", "-").Replace(host), "-")
	if name == "" {
		return "remote"
	}
	return name
}

// uniqueMCPName suffixes name with -2, -3, ... until servers has no
// entry by that name.
func uniqueMCPName(servers map[string]any, name string) string {
	if _, taken := servers[name]; !taken {
		return name
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", name, i)
		if _, taken := servers[candidate]; !taken {
			return candidate
		}
	}
}

// importOpenhandsSetup reads `.openhands/setup.sh` into one environment
// spec whose `install` is the script body. The shebang and provenance
// header are dropped because sync writes both back.
func importOpenhandsSetup(src, dstDir string) (int, error) {
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	script := string(data)
	if strings.HasPrefix(script, "#!") {
		_, script, _ = strings.Cut(script, "\n")
	}
	install := strings.TrimSpace(header.Strip(strings.TrimLeft(script, "\n")))
	if install == "" {
		return 0, nil
	}
	raw, err := yaml.Marshal(map[string]any{"name": openhandsSetupSpecName, "install": install + "\n"})
	if err != nil {
		return 0, fmt.Errorf("marshal %s: %w", src, err)
	}
	dst := filepath.Join(dstDir, openhandsSetupSpecName+".yaml")
	if err := importWriteFile(dst, raw, 0o644); err != nil {
		return 0, fmt.Errorf("write %s: %w", dst, err)
	}
	return 1, nil
}
