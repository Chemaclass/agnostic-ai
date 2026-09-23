package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

const (
	factoryMainFile    = "AGENTS.md"
	factoryDroidsDir   = ".factory/droids"
	factoryCommandsDir = ".factory/commands"
	factoryMCPFile     = ".factory/mcp.json"
	factoryMCPKey      = "mcpServers"
	factoryHooksFile   = ".factory/hooks.json"
	// factoryLegacyHooksFile "still loads" when the current file is
	// absent (docs.factory.ai/harness/hooks).
	factoryLegacyHooksFile = ".factory/hooks/hooks.json"
	factorySettingsFile    = ".factory/settings.json"
	factorySettingsSpec    = "factory"
	factoryNativeMetaKey   = "x-factory"
)

// factorySkillDirs lists sync's root skill path first, then Factory's
// own project path, "<repo>/.factory/skills/<skill-name>/SKILL.md"
// (docs.factory.ai/harness/skills), which scoped skills also use.
var factorySkillDirs = []string{".agents/skills", ".factory/skills"}

// factoryToolPortable reverses the three renames the factory adapter
// applies to a droid's `tools`. Factory's other tool IDs are spelled
// the same on both sides.
var factoryToolPortable = map[string]string{
	"Read": "Read", "LS": "LS", "Grep": "Grep", "Glob": "Glob", "Edit": "Edit",
	"ApplyPatch": "ApplyPatch", "WebSearch": "WebSearch",
	"Execute": "Bash", "Create": "Write", "FetchUrl": "WebFetch",
}

// factoryDroidPortableKeys are the droid frontmatter keys that map onto
// spec fields. Every other key lands under x-factory, which the adapter
// passes through verbatim.
var factoryDroidPortableKeys = map[string]bool{
	"name": true, "description": true, "model": true, "tools": true, "mcpServers": true,
}

// factoryCommandList maps Factory's three command lists back to the
// portable permission lists. The adapter sends `ask` to
// commandDenylist, which prompts, and `deny` to commandBlocklist, which
// has no approval path.
var factoryCommandList = []struct{ native, portable string }{
	{"commandAllowlist", "allow"},
	{"commandDenylist", "ask"},
	{"commandBlocklist", "deny"},
}

// importFromFactory reads an existing Factory Droid CLI project and
// writes specs into the configured source directories, reversing the
// factory emit:
//
//   - `AGENTS.md` carries rules inlined in a sentinel-marked block and
//     mirrors to `.agnostic-ai/AGNOSTIC_AI.md`.
//   - `.factory/droids/*.md` becomes agents, with `Execute`, `Create`,
//     and `FetchUrl` renamed back to `Bash`, `Write`, and `WebFetch`.
//   - `.agents/skills/` and `.factory/skills/` folders become skills;
//     a `<scope>/.factory/skills/` folder keeps its scope.
//   - `.factory/commands/*.md` becomes commands.
//   - `.factory/mcp.json` becomes MCP specs.
//   - `.factory/hooks.json`, or the legacy `.factory/hooks/hooks.json`,
//     becomes hook specs.
//   - `.factory/settings.json` `model` and command lists become one
//     settings spec.
func importFromFactory(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.Commands, src.Hooks, src.MCPs, src.Settings); err != nil {
		return err
	}
	rules, err := sliceEntryPointRules(root, factoryMainFile, filepath.Join(root, src.Rules))
	if err != nil {
		return err
	}
	agents, err := importFactoryDroids(filepath.Join(root, factoryDroidsDir), filepath.Join(root, src.Agents))
	if err != nil {
		return err
	}
	skills, err := importScopedSkillFoldersFrom(root, factorySkillDirs, filepath.Join(root, src.Skills))
	if err != nil {
		return err
	}
	commands, err := importFlatMarkdownFiles(filepath.Join(root, factoryCommandsDir), filepath.Join(root, src.Commands), factoryCommandFields)
	if err != nil {
		return err
	}
	mcps, err := importJSONMCPMap(filepath.Join(root, factoryMCPFile), factoryMCPKey, filepath.Join(root, src.MCPs))
	if err != nil {
		return err
	}
	hooks, err := importFactoryHooks(root, filepath.Join(root, src.Hooks))
	if err != nil {
		return err
	}
	settings, err := importFactorySettings(filepath.Join(root, factorySettingsFile), filepath.Join(root, src.Settings))
	if err != nil {
		return err
	}
	if _, err := mirrorMainFile(root, factoryMainFile); err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d commands, %d mcps, %d hooks, %d settings\n",
		rules, agents, skills, commands, mcps, hooks, settings)
	printImportNextSteps(root, "factory")
	return nil
}

// importFactoryDroids writes one agent spec per `.factory/droids/*.md`.
func importFactoryDroids(srcDir, dstDir string) (int, error) {
	entries, err := os.ReadDir(srcDir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", srcDir, err)
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		src := filepath.Join(srcDir, e.Name())
		data, err := os.ReadFile(src)
		if err != nil {
			return count, fmt.Errorf("read %s: %w", src, err)
		}
		meta, body := splitMdcFrontmatter([]byte(header.Strip(string(data))))
		out, err := writeFrontmatter(factoryDroidSpecMeta(meta), body)
		if err != nil {
			return count, fmt.Errorf("render %s: %w", src, err)
		}
		dst := filepath.Join(dstDir, e.Name())
		if err := importWriteSpecMarkdown(dst, out, 0o644, factoryAgentFields); err != nil {
			return count, fmt.Errorf("write %s: %w", dst, err)
		}
		count++
	}
	return count, nil
}

// factoryDroidSpecMeta translates droid frontmatter into agent spec
// frontmatter. `reasoningEffort` becomes the portable `effort`, which
// the adapter writes back under Factory's own key.
//
// `tools` renames back only when every entry is a Factory tool ID. A
// category (`read-only`) or an MCP tool ID has no portable spelling, so
// such a list moves under x-factory whole, where the adapter writes it
// untranslated, rather than losing the names it cannot map.
func factoryDroidSpecMeta(meta map[string]any) map[string]any {
	out := map[string]any{}
	native := map[string]any{}
	for k, v := range meta {
		switch {
		case k == "reasoningEffort":
			out["effort"] = v
		case k == "tools":
			if portable, ok := portableFactoryTools(v); ok {
				out["tools"] = portable
			} else {
				native["tools"] = v
			}
		case factoryDroidPortableKeys[k]:
			out[k] = v
		default:
			native[k] = v
		}
	}
	if len(native) > 0 {
		out[factoryNativeMetaKey] = native
	}
	return out
}

func portableFactoryTools(v any) ([]string, bool) {
	list, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		name, _ := item.(string)
		portable, known := factoryToolPortable[name]
		if !known {
			return nil, false
		}
		out = append(out, portable)
	}
	return out, true
}

// importFactoryHooks reads `.factory/hooks.json`, falling back to the
// legacy `.factory/hooks/hooks.json`, and writes one hook spec per
// matcher group. Reading one file and not both keeps a project that
// carries both from importing every hook twice.
func importFactoryHooks(root, dstDir string) (int, error) {
	src := filepath.Join(root, factoryHooksFile)
	if _, err := os.Stat(src); errors.Is(err, fs.ErrNotExist) {
		src = filepath.Join(root, factoryLegacyHooksFile)
	}
	byEvent, err := readEventKeyedHooks[groupedHookGroup](src)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, event := range sortedHookEvents(byEvent) {
		for _, g := range byEvent[event] {
			n, err := writeGroupedHookSpec(dstDir, "factory", event, g.Matcher, g.Hooks, nil)
			if err != nil {
				return count, err
			}
			count += n
		}
	}
	return count, nil
}

// importFactorySettings reads `model`, `reasoningEffort`, and the three
// command lists from `.factory/settings.json` into one settings spec. Other keys stay in
// the file, which sync merges into rather than replaces.
func importFactorySettings(src, dstDir string) (int, error) {
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	data, _ = adapters.StripJSONC(data)
	var native map[string]any
	if err := json.Unmarshal(data, &native); err != nil {
		return 0, fmt.Errorf("parse %s: %w", src, err)
	}
	doc := map[string]any{}
	if model, _ := native["model"].(string); model != "" {
		doc["model"] = model
	}
	permissions := map[string][]string{}
	for _, list := range factoryCommandList {
		for _, pattern := range stringSliceFromAny(native[list.native]) {
			permissions[list.portable] = appendUnique(permissions[list.portable], portableFactoryCommand(pattern))
		}
	}
	if len(permissions) > 0 {
		doc["permissions"] = permissions
	}
	plan, level, err := planSettingsEffort("factory", native["reasoningEffort"], dstDir, factorySettingsSpec+".yaml")
	if err != nil {
		return 0, err
	}
	maps.Copy(doc, settingsEffortSpec(plan, "factory", "reasoningEffort", level))
	if len(doc) == 0 {
		return 0, nil
	}
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return 0, fmt.Errorf("marshal settings from %s: %w", src, err)
	}
	dst := filepath.Join(dstDir, factorySettingsSpec+".yaml")
	if err := importWriteFile(dst, raw, 0o644); err != nil {
		return 0, fmt.Errorf("write %s: %w", dst, err)
	}
	return 1, nil
}

// portableFactoryCommand turns a shell-command pattern into a Bash
// rule. The adapter writes a portable prefix rule `Bash(x:*)` as
// `x *`, so a trailing ` *` reads back as the prefix form.
func portableFactoryCommand(pattern string) string {
	if prefix, ok := strings.CutSuffix(pattern, " *"); ok && prefix != "" {
		return "Bash(" + prefix + ":*)"
	}
	return "Bash(" + pattern + ")"
}
