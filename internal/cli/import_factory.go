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
	// absent (docs.factory.com/harness/hooks).
	factoryLegacyHooksFile = ".factory/hooks/hooks.json"
	factorySettingsFile    = ".factory/settings.json"
	factorySettingsSpec    = "factory"
	factoryNativeMetaKey   = "x-factory"
)

// factorySkillDirs lists sync's root skill path first, then Factory's
// own project path, "<repo>/.factory/skills/<skill-name>/SKILL.md"
// (docs.factory.com/harness/skills), which scoped skills also use.
var factorySkillDirs = []string{".agents/skills", ".factory/skills"}

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
	rules, err := sliceEntryPointRules(root, factoryMainFile, importSourcePath(root, src.Rules))
	if err != nil {
		return err
	}
	agents, err := importFactoryDroids(filepath.Join(root, factoryDroidsDir), importSourcePath(root, src.Agents))
	if err != nil {
		return err
	}
	skills, err := importScopedSkillFoldersFrom(root, factorySkillDirs, importSourcePath(root, src.Skills))
	if err != nil {
		return err
	}
	commands, err := importFlatMarkdownFiles(filepath.Join(root, factoryCommandsDir), importSourcePath(root, src.Commands), factoryCommandFields)
	if err != nil {
		return err
	}
	mcps, err := importJSONMCPMap("factory", filepath.Join(root, factoryMCPFile), factoryMCPKey, importSourcePath(root, src.MCPs))
	if err != nil {
		return err
	}
	hooks, err := importFactoryHooks(root, importSourcePath(root, src.Hooks))
	if err != nil {
		return err
	}
	settings, err := importFactorySettings(filepath.Join(root, factorySettingsFile), importSourcePath(root, src.Settings))
	if err != nil {
		return err
	}
	if _, err := mirrorMainFile(root, factoryMainFile); err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d commands, %d mcps, %d hooks, %d settings\n",
		rules, agents, skills, commands, mcps, hooks, settings)
	return nil
}

// importFactoryDroids writes one agent spec per `.factory/droids/*.md`.
func importFactoryDroids(srcDir, dstDir string) (int, error) {
	return importFlatMarkdownFilesWith(srcDir, dstDir, factoryAgentFields, func(data []byte) ([]byte, error) {
		meta, body := splitMdcFrontmatter(data)
		return writeFrontmatter(factoryDroidSpecMeta(meta), body)
	})
}

// Native tool lists stay intact when any entry has no exact neutral form.
func factoryDroidSpecMeta(meta map[string]any) map[string]any {
	out := map[string]any{}
	native := map[string]any{}
	for k, v := range meta {
		switch {
		case k == "reasoningEffort":
			out["effort"] = v
		case k == "tools":
			if portable, ok := portableFactoryTools(v); ok {
				out["can"] = portable
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

func portableFactoryTools(value any) ([]string, bool) {
	return importNativeAgentCan(value, "factory")
}

// factoryHookEntry is a grouped hook entry plus Factory's per-entry
// `onFailure` and `preserveRiskLevel`. Both are `any` so a value of
// another type is skipped instead of failing the whole file.
type factoryHookEntry struct {
	groupedHookEntry
	OnFailure         any `json:"onFailure"`
	PreserveRiskLevel any `json:"preserveRiskLevel"`
}

// factoryHookGroup is a matcher group plus Factory's group-level
// `commandRegex` filter, which the spec keeps under `x-factory`.
type factoryHookGroup struct {
	Matcher      string             `json:"matcher"`
	CommandRegex any                `json:"commandRegex"`
	Hooks        []factoryHookEntry `json:"hooks"`
}

// factoryEntrySettings are the per-entry keys that split a group into
// specs, since every command in one spec shares them.
type factoryEntrySettings struct {
	onFailure    string
	preserveRisk bool
	timeout      int
}

// importFactoryHooks reads `.factory/hooks.json`, falling back to the
// legacy `.factory/hooks/hooks.json`, and writes one hook spec per
// consecutive run of commands with the same entry settings. Reading
// one file keeps a project that carries both from importing every hook
// twice. `onFailure: "block"` becomes the portable `failClosed: true`;
// another `onFailure` value and `preserveRiskLevel` stay under
// `x-factory`, which sync writes back.
func importFactoryHooks(root, dstDir string) (int, error) {
	src := filepath.Join(root, factoryHooksFile)
	if _, err := os.Stat(src); errors.Is(err, fs.ErrNotExist) {
		src = filepath.Join(root, factoryLegacyHooksFile)
	}
	byEvent, err := readEventKeyedHooks[factoryHookGroup](src)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, event := range sortedHookEvents(byEvent) {
		for groupIndex, g := range byEvent[event] {
			regex, _ := g.CommandRegex.(string)
			type hookRun struct {
				settings factoryEntrySettings
				entries  []groupedHookEntry
			}
			var runs []hookRun
			for _, h := range g.Hooks {
				k := factoryEntrySettings{timeout: h.Timeout}
				k.onFailure, _ = h.OnFailure.(string)
				k.preserveRisk, _ = h.PreserveRiskLevel.(bool)
				if len(runs) == 0 || runs[len(runs)-1].settings != k {
					runs = append(runs, hookRun{settings: k})
				}
				last := &runs[len(runs)-1]
				last.entries = append(last.entries, h.groupedHookEntry)
			}
			for runIndex, run := range runs {
				extra, salt := factoryHookExtra(regex, run.settings)
				prefix := fmt.Sprintf("factory-%09d-%09d-", groupIndex, runIndex)
				n, err := writeGroupedHookSpecWithPrefix(dstDir, "factory", event, g.Matcher, prefix, salt, run.entries, extra)
				if err != nil {
					return count, err
				}
				count += n
			}
		}
	}
	return count, nil
}

// factoryHookExtra is the spec's Factory-only keys for one group's
// commandRegex and entry settings, and the salt that keeps its name
// apart from a spec split from the same group. Default settings add no salt.
func factoryHookExtra(regex string, k factoryEntrySettings) (map[string]any, string) {
	native := map[string]any{}
	extra := map[string]any{}
	var salt []string
	if regex != "" {
		native["commandRegex"] = regex
		salt = append(salt, "commandRegex="+regex)
	}
	switch k.onFailure {
	case "":
	case "block":
		extra["failClosed"] = true
	default:
		native["onFailure"] = k.onFailure
	}
	if k.preserveRisk {
		native["preserveRiskLevel"] = true
	}
	if k != (factoryEntrySettings{}) {
		salt = append(salt, fmt.Sprintf("onFailure=%s preserveRiskLevel=%t timeout=%d", k.onFailure, k.preserveRisk, k.timeout))
	}
	if len(native) > 0 {
		extra[factoryNativeMetaKey] = native
	}
	if len(extra) == 0 {
		extra = nil
	}
	return extra, strings.Join(salt, " ")
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
			permissions[list.portable] = appendUnique(permissions[list.portable], importNeutralPermissionRule(portableFactoryCommand(pattern)))
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

func normalizeFactoryMCPEntry(server map[string]any) {
	tools, present := server["enabledTools"]
	if !present {
		return
	}
	native, ok := server[factoryNativeMetaKey].(map[string]any)
	if !ok {
		native = map[string]any{}
	}
	native["enabledTools"] = tools
	server[factoryNativeMetaKey] = native
	delete(server, "enabledTools")
}
