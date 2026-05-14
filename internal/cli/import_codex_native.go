package cli

import (
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
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// importCodexSkills walks `<root>/.agents/skills/<name>/SKILL.md` and
// copies each into `<dstDir>/<name>/SKILL.md` byte-for-byte. The skills
// emitter writes SKILL.md with frontmatter (`name`, `description`) plus
// body; that shape round-trips as-is.
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
		src := filepath.Join(srcDir, e.Name(), "SKILL.md")
		data, err := os.ReadFile(src)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return count, fmt.Errorf("read %s: %w", src, err)
		}
		dstSubdir := filepath.Join(dstDir, e.Name())
		if err := os.MkdirAll(dstSubdir, 0o755); err != nil {
			return count, fmt.Errorf("mkdir %s: %w", dstSubdir, err)
		}
		dst := filepath.Join(dstSubdir, "SKILL.md")
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", dst, err)
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
	Matcher string `toml:"matcher"`
	Command string `toml:"command"`
}

type codexMCPEntry struct {
	Command           string            `toml:"command"`
	Args              []string          `toml:"args"`
	Env               map[string]string `toml:"env"`
	URL               string            `toml:"url"`
	BearerTokenEnvVar string            `toml:"bearer_token_env_var"`
	HTTPHeaders       map[string]string `toml:"http_headers"`
}

// importCodexConfig reads `<root>/.codex/config.toml` and writes one
// yaml per `[[hooks.<event>]]` entry to hooksDst and one yaml per
// `[mcp_servers.<name>]` table to mcpsDst. Returns (hooks, mcps).
func importCodexConfig(root, hooksDst, mcpsDst string) (int, int, error) {
	path := filepath.Join(root, codexConfigTOML)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("read %s: %w", path, err)
	}
	var doc codexConfigDoc
	if _, err := toml.Decode(string(data), &doc); err != nil {
		return 0, 0, fmt.Errorf("parse %s: %w", path, err)
	}

	hooks, err := writeCodexHooks(doc.Hooks, hooksDst)
	if err != nil {
		return hooks, 0, err
	}
	mcps, err := writeCodexMCPs(doc.MCPServers, mcpsDst)
	if err != nil {
		return hooks, mcps, err
	}
	return hooks, mcps, nil
}

func writeCodexHooks(byEvent map[string][]codexHookEntry, dstDir string) (int, error) {
	events := make([]string, 0, len(byEvent))
	for e := range byEvent {
		events = append(events, e)
	}
	sort.Strings(events)

	count := 0
	for _, event := range events {
		for _, h := range byEvent[event] {
			if h.Command == "" {
				continue
			}
			name := hookSpecName(event, h.Matcher, []string{h.Command})
			doc := map[string]any{
				"name":    name,
				"event":   event,
				"command": h.Command,
			}
			if h.Matcher != "" {
				doc["matcher"] = h.Matcher
			}
			raw, err := yaml.Marshal(doc)
			if err != nil {
				return count, fmt.Errorf("marshal hook %s: %w", name, err)
			}
			path := filepath.Join(dstDir, name+".yaml")
			if err := os.WriteFile(path, raw, 0o644); err != nil {
				return count, fmt.Errorf("write %s: %w", path, err)
			}
			count++
		}
	}
	return count, nil
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
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", path, err)
		}
		count++
	}
	return count, nil
}
