package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// Codex stores subagents and skills under `.agents/`. Older layouts used
// `.codex/agents/` for TOMLs; both are scanned so an upgrade picks up
// either location.
var codexAgentDirs = []string{".agents/agents", ".codex/agents"}

const (
	codexDir        = ".codex"
	codexSkillsDir  = ".agents/skills"
	codexConfigTOML = ".codex/config.toml"
)

// importCodexAgents reads every `<root>/<dir>/*.toml` agent file (where
// <dir> is one of `.agents/agents/` or `.codex/agents/`) and writes one
// `.md` per agent to dstDir. Frontmatter captures `name`, `description`,
// `model`, and codex-specific `x-codex.*` keys; the body comes from
// `developer_instructions`.
func importCodexAgents(root, dstDir string) (int, error) {
	count := 0
	seen := map[string]bool{}
	for _, sub := range codexAgentDirs {
		dir := filepath.Join(root, sub)
		entries, err := os.ReadDir(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return count, fmt.Errorf("read %s: %w", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			doc, err := readCodexAgentTOML(path)
			if err != nil {
				return count, err
			}
			name := strings.TrimSuffix(e.Name(), ".toml")
			if n, _ := doc["name"].(string); n != "" {
				name = n
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			if err := writeCodexAgentSpec(dstDir, name, doc); err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}

func readCodexAgentTOML(path string) (map[string]any, error) {
	doc := map[string]any{}
	if _, err := toml.DecodeFile(path, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc, nil
}

// codexAgentTopLevel are the keys the codex emitter writes at the TOML
// root that map to frontmatter top-level slots on round-trip. Everything
// else gets carried under `x-codex` so unknown / future Codex keys pass
// through without data loss.
var codexAgentTopLevel = map[string]bool{
	"name":                   true,
	"description":            true,
	"model":                  true,
	"developer_instructions": true,
}

// writeCodexAgentSpec renders a codex agent TOML as an agnostic-ai agent
// spec (.md with frontmatter + body). Known top-level keys map directly;
// every other key lands under `x-codex` so the emitter round-trips them.
func writeCodexAgentSpec(dstDir, name string, doc map[string]any) error {
	body, _ := doc["developer_instructions"].(string)
	body = strings.TrimRight(body, "\n")

	fm := map[string]any{"name": name}
	if d, _ := doc["description"].(string); d != "" {
		fm["description"] = d
	}
	if m, _ := doc["model"].(string); m != "" {
		fm["model"] = m
	}
	xcodex := map[string]any{}
	for key, val := range doc {
		if codexAgentTopLevel[key] {
			continue
		}
		xcodex[key] = val
	}
	if len(xcodex) > 0 {
		fm["x-codex"] = xcodex
	}

	raw, err := yaml.Marshal(fm)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", name, err)
	}
	out := "---\n" + string(raw) + "---\n\n" + body + "\n"

	path := filepath.Join(dstDir, name+".md")
	if err := importWriteFile(path, []byte(out), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// importCodexSkills walks `<root>/.agents/skills/<name>/` and mirrors
// each skill folder byte-for-byte into `<dstDir>/<name>/`. Every file
// under the skill directory — SKILL.md, `agents/openai.yaml`, helper
// scripts, fixtures, nested subdirectories — is preserved so an import
// then `sync` keeps the full skill payload intact across all targets.
func importCodexSkills(root, dstDir string) (int, error) {
	srcDir := filepath.Join(root, codexSkillsDir)
	entries, err := os.ReadDir(srcDir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", srcDir, err)
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillSrc := filepath.Join(srcDir, e.Name())
		if _, err := os.Stat(filepath.Join(skillSrc, "SKILL.md")); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return count, fmt.Errorf("stat skill %s: %w", e.Name(), err)
		}
		skillDst := filepath.Join(dstDir, e.Name())
		if err := copyDirTree(skillSrc, skillDst); err != nil {
			return count, fmt.Errorf("copy skill %s: %w", e.Name(), err)
		}
		count++
	}
	return count, nil
}

// codexConfigDoc mirrors the relevant `.codex/config.toml` shape: nested
// `[[hooks.<event>]]` arrays and `[mcp_servers.<name>]` tables.
type codexConfigDoc struct {
	Hooks      map[string][]codexHookEntry `toml:"hooks"`
	MCPServers map[string]codexMCPEntry    `toml:"mcp_servers"`
}

type codexHookEntry struct {
	Matcher       string `toml:"matcher"`
	Command       string `toml:"command"`
	Timeout       int    `toml:"timeout"`
	StatusMessage string `toml:"statusMessage"`
}

type codexMCPEntry struct {
	Command           string            `toml:"command"`
	Args              []string          `toml:"args"`
	Env               map[string]string `toml:"env"`
	URL               string            `toml:"url"`
	BearerTokenEnvVar string            `toml:"bearer_token_env_var"`
	HTTPHeaders       map[string]string `toml:"http_headers"`
}

// importCodexConfig reads `<root>/.codex/config.toml` plus the
// standalone `<root>/.codex/hooks.json` (if present) and writes one
// yaml per discovered hook to hooksDst and one yaml per `[mcp_servers.<name>]`
// table to mcpsDst. Hooks declared in both files are deduped by
// (event, matcher, command); the hooks.json variant wins because it
// can carry timeout + statusMessage.
//
// Returns (hooks, mcps).
func importCodexConfig(root, hooksDst, mcpsDst string) (int, int, error) {
	hooksByKey, mcpServers, err := readCodexConfigTOML(root)
	if err != nil {
		return 0, 0, err
	}
	if err := mergeCodexHooksJSON(root, hooksByKey); err != nil {
		return 0, 0, err
	}

	hooks, err := writeCodexHooksFromMap(hooksByKey, hooksDst)
	if err != nil {
		return hooks, 0, err
	}
	mcps, err := writeCodexMCPs(mcpServers, mcpsDst)
	if err != nil {
		return hooks, mcps, err
	}
	return hooks, mcps, nil
}

// codexHookKey identifies a hook for dedupe across sources. Hooks with
// the same (event, matcher, command) are considered the same entry.
type codexHookKey struct {
	event, matcher, command string
}

// codexHookSlot is the merged hook representation built up by reading
// every codex hook source. The order field preserves discovery order
// so emitted spec files stay byte-stable across re-imports.
type codexHookSlot struct {
	order         int
	entry         codexHookEntry
	event         string
	fromHooksJSON bool
}

// readCodexConfigTOML reads `.codex/config.toml` and returns a
// dedupe-keyed map of hooks plus the MCP servers section. The dedupe key
// uses (event, matcher, command) so the standalone hooks.json layer can
// overwrite TOML-defined entries that carry less information.
func readCodexConfigTOML(root string) (map[codexHookKey]*codexHookSlot, map[string]codexMCPEntry, error) {
	hooks := map[codexHookKey]*codexHookSlot{}
	path := filepath.Join(root, codexConfigTOML)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return hooks, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	var doc codexConfigDoc
	if _, err := toml.Decode(string(data), &doc); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for _, event := range sortedMapKeys(doc.Hooks) {
		for _, h := range doc.Hooks[event] {
			if h.Command == "" {
				continue
			}
			k := codexHookKey{event: event, matcher: h.Matcher, command: h.Command}
			if _, exists := hooks[k]; exists {
				continue
			}
			hooks[k] = &codexHookSlot{order: len(hooks), entry: h, event: event}
		}
	}
	return hooks, doc.MCPServers, nil
}

// mergeCodexHooksJSON layers `.codex/hooks.json` over the config.toml
// dedupe map. When the same (event, matcher, command) appears in both,
// the JSON entry wins so timeout + statusMessage propagate even when
// the TOML copy carried only matcher + command.
func mergeCodexHooksJSON(root string, hooks map[codexHookKey]*codexHookSlot) error {
	path := filepath.Join(root, codexDir, "hooks.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	var s claudeSettings // identical shape: hooks[<event>][n].{matcher, hooks[m].{...}}
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	for _, event := range sortedMapKeys(s.Hooks) {
		for _, g := range s.Hooks[event] {
			for _, h := range g.Hooks {
				if h.Command == "" {
					continue
				}
				k := codexHookKey{event: event, matcher: g.Matcher, command: h.Command}
				slot, exists := hooks[k]
				entry := codexHookEntry{
					Matcher:       g.Matcher,
					Command:       h.Command,
					Timeout:       h.Timeout,
					StatusMessage: h.StatusMessage,
				}
				if !exists {
					hooks[k] = &codexHookSlot{
						order:         len(hooks),
						entry:         entry,
						event:         event,
						fromHooksJSON: true,
					}
					continue
				}
				slot.entry = entry
				slot.fromHooksJSON = true
			}
		}
	}
	return nil
}

// writeCodexHooksFromMap emits one yaml per discovered hook. Hooks are
// written in (event, order) order so a re-import produces byte-stable
// output regardless of which source file changed.
func writeCodexHooksFromMap(hooks map[codexHookKey]*codexHookSlot, dstDir string) (int, error) {
	slots := make([]*codexHookSlot, 0, len(hooks))
	for _, slot := range hooks {
		slots = append(slots, slot)
	}
	sort.SliceStable(slots, func(i, j int) bool {
		if slots[i].event != slots[j].event {
			return slots[i].event < slots[j].event
		}
		return slots[i].order < slots[j].order
	})

	count := 0
	for _, slot := range slots {
		h := slot.entry
		name := hookSpecName(slot.event, h.Matcher, []string{h.Command})
		doc := map[string]any{
			"name":    name,
			"event":   slot.event,
			"command": h.Command,
		}
		if h.Matcher != "" {
			doc["matcher"] = h.Matcher
		}
		if h.Timeout != 0 {
			doc["timeout"] = h.Timeout
		}
		if h.StatusMessage != "" {
			doc["statusMessage"] = h.StatusMessage
		}
		raw, err := yaml.Marshal(doc)
		if err != nil {
			return count, fmt.Errorf("marshal hook %s: %w", name, err)
		}
		path := filepath.Join(dstDir, name+".yaml")
		if err := importWriteFile(path, raw, 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", path, err)
		}
		count++
	}
	return count, nil
}

// sortedMapKeys returns a copy of map[string]V's keys sorted for
// deterministic iteration. Local to the codex importer to avoid
// colliding with the validate.go variant that takes a set.
func sortedMapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func writeCodexMCPs(servers map[string]codexMCPEntry, dstDir string) (int, error) {
	names := make([]string, 0, len(servers))
	for n := range servers {
		names = append(names, n)
	}
	sort.Strings(names)

	count := 0
	for _, name := range names {
		s := servers[name]
		doc := map[string]any{"name": name}
		switch {
		case s.URL != "":
			doc["type"] = "http"
			doc["url"] = s.URL
			if s.BearerTokenEnvVar != "" {
				doc["bearer_token_env_var"] = s.BearerTokenEnvVar
			}
			if len(s.HTTPHeaders) > 0 {
				doc["headers"] = s.HTTPHeaders
			}
		default:
			doc["type"] = "stdio"
			if s.Command != "" {
				doc["command"] = s.Command
			}
			if len(s.Args) > 0 {
				doc["args"] = s.Args
			}
		}
		if len(s.Env) > 0 {
			doc["env"] = s.Env
		}
		raw, err := yaml.Marshal(doc)
		if err != nil {
			return count, fmt.Errorf("marshal mcp %s: %w", name, err)
		}
		path := filepath.Join(dstDir, name+".yaml")
		if err := importWriteFile(path, raw, 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", path, err)
		}
		count++
	}
	return count, nil
}
