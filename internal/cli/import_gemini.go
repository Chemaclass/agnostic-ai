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

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

const (
	geminiMainFile    = "GEMINI.md"
	geminiCommandsDir = ".gemini/commands"
	geminiSettings    = ".gemini/settings.json"
)

// importFromGemini reads an existing Gemini CLI project (root GEMINI.md
// plus any nested <dir>/GEMINI.md, `.gemini/commands/`,
// `.gemini/settings.json`) under root and writes specs into the
// configured source directories.
func importFromGemini(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Hooks, src.MCPs); err != nil {
		return err
	}
	rules, err := importGeminiRules(root, filepath.Join(root, src.Rules), src)
	if err != nil {
		return err
	}
	agents, err := importGeminiCommands(root, filepath.Join(root, src.Agents))
	if err != nil {
		return err
	}
	mcps, hooks, err := importGeminiSettings(root, filepath.Join(root, src.MCPs), filepath.Join(root, src.Hooks))
	if err != nil {
		return err
	}
	if err := mirrorMainFile(root, geminiMainFile); err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d mcps, %d hooks\n", rules, agents, mcps, hooks)
	printImportNextSteps(root, "gemini")
	return nil
}

// importGeminiRules walks the project for GEMINI.md files and writes
// one rule per ## section into dstDir. Rules from <dir>/GEMINI.md
// inherit `globs: <dir>/**`, mirroring the codex importer. Slug
// collisions across files are deduplicated.
func importGeminiRules(root, dstDir string, src config.Sources) (int, error) {
	files, err := findHierarchicalMainFiles(root, geminiMainFile, src)
	if err != nil {
		return 0, err
	}
	if len(files) == 0 {
		return 0, nil
	}
	used := map[string]int{}
	count := 0
	for _, f := range files {
		data, err := os.ReadFile(f.path)
		if err != nil {
			return count, fmt.Errorf("read %s: %w", f.path, err)
		}
		_, sections := splitH2Sections(string(data))
		if len(sections) == 0 {
			body := strings.TrimSpace(string(data))
			if body == "" {
				continue
			}
			name := dedupSlug(used, projectSlug(root))
			if err := writeScopedRule(dstDir, name, f.globs, body); err != nil {
				return count, err
			}
			count++
			continue
		}
		for _, s := range sections {
			name := dedupSlug(used, s.slug)
			if err := writeScopedRule(dstDir, name, f.globs, s.body); err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}

// importGeminiCommands reads `.gemini/commands/*.toml` and writes one
// agent spec per command into dstDir. The `prompt` field becomes the
// agent body; `description` is preserved in frontmatter.
func importGeminiCommands(root, dstDir string) (int, error) {
	src := filepath.Join(root, geminiCommandsDir)
	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		full := filepath.Join(src, e.Name())
		data, err := os.ReadFile(full)
		if err != nil {
			return count, fmt.Errorf("read %s: %w", full, err)
		}
		desc, body := parseGeminiCommandTOML(string(data))
		name := strings.TrimSuffix(e.Name(), ".toml")
		out := filepath.Join(dstDir, name+".md")
		if err := writeAgentMD(out, name, desc, nil, body); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// parseGeminiCommandTOML extracts `description` and `prompt` from a
// minimal subset of TOML that matches what the gemini emitter writes:
// quoted string for description, triple-quoted multiline for prompt.
// Tolerant of either order. Anything else passes through as the body
// raw text if `prompt` is missing.
func parseGeminiCommandTOML(s string) (description, body string) {
	description = extractTOMLString(s, "description")
	body = extractTOMLMultiline(s, "prompt")
	if body == "" {
		body = strings.TrimSpace(s)
	}
	return description, body
}

// extractTOMLString finds `key = "value"` and returns value. Empty if
// not present. Does not handle escapes; gemini emitter does not write
// them. The key boundary is enforced so a search for `description`
// does not match `description-extra`.
func extractTOMLString(s, key string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, key) {
			continue
		}
		after := line[len(key):]
		if after == "" || (after[0] != '=' && after[0] != ' ' && after[0] != '\t') {
			continue
		}
		rest := strings.TrimSpace(after)
		rest = strings.TrimPrefix(rest, "=")
		rest = strings.TrimSpace(rest)
		if strings.HasPrefix(rest, "\"") && strings.HasSuffix(rest, "\"") && len(rest) >= 2 {
			return rest[1 : len(rest)-1]
		}
	}
	return ""
}

// extractTOMLMultiline finds `key = """\n...\n"""` and returns the
// inner block. Returns empty if not present.
func extractTOMLMultiline(s, key string) string {
	open := key + " = \"\"\""
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}
	rest := s[i+len(open):]
	rest = strings.TrimPrefix(rest, "\n")
	end := strings.Index(rest, "\"\"\"")
	if end < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimRight(rest[:end], "\n")
}

// importGeminiSettings reads `.gemini/settings.json` and writes one
// yaml per `mcpServers` entry and one yaml per `hooks.<event>` entry.
// Returns (mcps, hooks, err).
func importGeminiSettings(root, mcpDst, hooksDst string) (int, int, error) {
	src := filepath.Join(root, geminiSettings)
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("read %s: %w", src, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return 0, 0, fmt.Errorf("parse %s: %w", src, err)
	}
	mcps := 0
	if servers, ok := doc["mcpServers"].(map[string]any); ok {
		clean := map[string]any{}
		for k, v := range servers {
			if sub, ok := v.(map[string]any); ok {
				clean[k] = sub
			}
		}
		n, err := writeMCPYAMLs(clean, mcpDst)
		if err != nil {
			return n, 0, err
		}
		mcps = n
	}
	hooks := 0
	if hooksMap, ok := doc["hooks"].(map[string]any); ok {
		n, err := writeGeminiHooks(hooksMap, hooksDst)
		if err != nil {
			return mcps, n, err
		}
		hooks = n
	}
	return mcps, hooks, nil
}

// writeGeminiHooks writes one yaml per `hooks.<event>[i]` entry into
// dstDir. Filenames come from hookSpecName so the same hook lands at
// the same path across re-imports.
func writeGeminiHooks(hooks map[string]any, dstDir string) (int, error) {
	events := make([]string, 0, len(hooks))
	for e := range hooks {
		events = append(events, e)
	}
	sort.Strings(events)
	count := 0
	for _, event := range events {
		entries, ok := hooks[event].([]any)
		if !ok {
			continue
		}
		for _, raw := range entries {
			entry, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			cmd, _ := entry["command"].(string)
			matcher, _ := entry["matcher"].(string)
			var cmds []string
			if cmd != "" {
				cmds = []string{cmd}
			}
			name := hookSpecName(event, matcher, cmds)
			doc := map[string]any{
				"name":  name,
				"event": event,
			}
			if cmd != "" {
				doc["command"] = cmd
			}
			if matcher != "" {
				doc["matcher"] = matcher
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
	}
	return count, nil
}
