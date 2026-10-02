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

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	continueRulesDir      = ".continue/rules"
	continueMCPServersDir = ".continue/mcpServers"
	continueSkillsDir     = ".continue/skills"
)

// importFromContinue reads an existing Continue (continue.dev) project
// under root and writes specs into the configured source directories.
//
//   - `.continue/rules/*.md` walks via the shared rules-directory
//     importer (agent-<name>.md routes into agents, skill-<name>.md
//     into skills, the rest into rules; provenance + leading H1 are
//     stripped). Native globs and regex conditions retain their scalar
//     or array values under x-continue.
//   - `.continue/skills/<name>/SKILL.md` native skill folders copy
//     byte-for-byte via importSkillFolders, so bundled assets survive.
//     A `skill-<name>.md` rule from an older sync still imports above;
//     the native folder imports after it and merges onto that spec.
//   - `.continue/mcpServers/*.yaml` imports one MCP spec per file.
//   - `.continue/mcpServers/*.json` accepts JSONC with a named
//     `mcpServers` map or a single server named after its source file.
func importFromContinue(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.MCPs); err != nil {
		return err
	}
	c, err := importRulesDirectoryWith(root, continueRulesDir, src, rulesDirImportOpts{
		NativeTarget: "continue", NativeKeys: []string{"globs", "regex"},
	})
	if err != nil {
		return err
	}
	skills, err := importSkillFolders(root, filepath.Join(root, continueSkillsDir), importSourcePath(root, src.Skills))
	if err != nil {
		return err
	}
	mcps, err := importContinueMCPs(root, importSourcePath(root, src.MCPs))
	if err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d mcps\n",
		c.rules, c.agents, c.skills+skills, mcps)
	printImportNextSteps(root, "continue")
	return nil
}

type continueMCPFile struct {
	name, source, serverName string
	body                     []byte
}

// importContinueMCPs imports YAML blocks and JSONC server definitions.
// It prepares every destination before writing so colliding names
// cannot overwrite another imported server, even on case-insensitive
// filesystems.
func importContinueMCPs(root, dstDir string) (int, error) {
	srcDir := filepath.Join(root, continueMCPServersDir)
	entries, err := os.ReadDir(srcDir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", srcDir, err)
	}
	var files []continueMCPFile
	sources := map[string]string{}
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if e.IsDir() || (ext != ".yaml" && ext != ".json") {
			continue
		}
		src := filepath.Join(srcDir, e.Name())
		data, err := os.ReadFile(src)
		if err != nil {
			return 0, fmt.Errorf("read %s: %w", src, err)
		}
		var imported []continueMCPFile
		if ext == ".json" {
			imported, err = parseContinueJSONMCPs(src, data)
			if err != nil {
				return 0, err
			}
		} else {
			body, err := parseContinueYAMLMCP(src, []byte(strings.TrimLeft(header.Strip(string(data)), "\n")))
			if err != nil {
				return 0, err
			}
			imported = []continueMCPFile{{name: e.Name(), body: body}}
		}
		for _, file := range imported {
			file.source = src
			key := strings.ToLower(file.name)
			if previous, exists := sources[key]; exists {
				return 0, fmt.Errorf("%s: MCP destination %q conflicts with %s", src, file.name, previous)
			}
			sources[key] = src
			files = append(files, file)
		}
	}
	if len(files) == 0 {
		return 0, nil
	}
	servers := make(map[string]any, len(files))
	var names []string
	for i := range files {
		file := &files[i]
		var server map[string]any
		if err := yaml.Unmarshal(file.body, &server); err != nil {
			return 0, fmt.Errorf("parse %s: %w", file.source, err)
		}
		if server == nil {
			return 0, fmt.Errorf("parse %s: MCP server must be an object", file.source)
		}
		file.serverName, _ = server["name"].(string)
		if file.serverName == "" {
			file.serverName = strings.TrimSuffix(file.name, ".yaml")
			server["name"] = file.serverName
		}
		names = append(names, file.serverName)
		unvendorContinueServer(server)
		if err := normalizeContinueMCPCredentials(server); err != nil {
			return 0, fmt.Errorf("parse %s: %w", file.source, err)
		}
		adapters.ReadMCPEnvRefs("continue", server)
		servers[file.serverName] = server
	}
	if err := spec.ValidateMCPNames(names); err != nil {
		return 0, fmt.Errorf("parse %s: %w", srcDir, err)
	}
	refs := referenceMCPLiterals(servers)
	if err := importMkdirAll(dstDir, 0o755); err != nil {
		return 0, fmt.Errorf("mkdir %s: %w", dstDir, err)
	}
	count := 0
	for _, file := range files {
		raw, err := yaml.Marshal(servers[file.serverName])
		if err != nil {
			return count, fmt.Errorf("marshal MCP %s: %w", file.name, err)
		}
		dst := filepath.Join(dstDir, file.name)
		if err := importWriteFile(dst, raw, 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", dst, err)
		}
		count++
	}
	reportMCPLiteralRefsWithHint(refs, "Continue IDE reads secrets from .env files; set these variables in .continue/.env or ~/.continue/.env")
	return count, nil
}

func parseContinueJSONMCPs(src string, data []byte) ([]continueMCPFile, error) {
	data, _ = adapters.StripJSONC(data)
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", src, err)
	}
	servers := map[string]any{}
	if value, exists := doc["mcpServers"]; exists {
		var ok bool
		servers, ok = value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("parse %s: mcpServers must be an object keyed by server name", src)
		}
	} else {
		name := strings.TrimSuffix(filepath.Base(src), ".json")
		servers[name] = doc
	}
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	if err := spec.ValidateMCPNames(names); err != nil {
		return nil, fmt.Errorf("parse %s: %w", src, err)
	}
	files := make([]continueMCPFile, 0, len(names))
	for _, name := range names {
		server, ok := servers[name].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("parse %s: MCP server %q must be an object", src, name)
		}
		command, _ := server["command"].(string)
		url, _ := server["url"].(string)
		if command == "" && url == "" {
			return nil, fmt.Errorf("parse %s: MCP server %q requires a command or url", src, name)
		}
		server["name"] = name
		unvendorContinueServer(server)
		if _, hasType := server["type"]; !hasType && command == "" && url != "" {
			server["type"] = "http"
		}
		raw, err := yaml.Marshal(server)
		if err != nil {
			return nil, fmt.Errorf("parse %s: marshal MCP server %q: %w", src, name, err)
		}
		files = append(files, continueMCPFile{name: spec.MCPFileName(name), body: raw})
	}
	return files, nil
}

func parseContinueYAMLMCP(src string, data []byte) ([]byte, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, fmt.Errorf("parse %s: %w", src, continueMCPYAMLError{err})
	}
	if err := continueMCPStringKeys(&node); err != nil {
		return nil, fmt.Errorf("parse %s: %w", src, err)
	}
	var server map[string]any
	if err := node.Decode(&server); err != nil {
		return nil, fmt.Errorf("parse %s: %w", src, continueMCPYAMLError{err})
	}
	if raw, wrapped := server["mcpServers"]; wrapped {
		blocks, ok := raw.([]any)
		if !ok || len(blocks) != 1 {
			return nil, fmt.Errorf("parse %s: an MCP YAML block must contain exactly one server", src)
		}
		server, _ = blocks[0].(map[string]any)
	}
	if server == nil {
		return nil, fmt.Errorf("parse %s: MCP server must be an object", src)
	}
	unvendorContinueServer(server)
	raw, err := yaml.Marshal(server)
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", src, err)
	}
	return raw, nil
}

func continueMCPStringKeys(node *yaml.Node) error {
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode {
				return fmt.Errorf("MCP mapping keys must be scalar values")
			}
			if key.Tag != "!!merge" {
				key.Tag = "!!str"
			}
		}
	}
	for _, child := range node.Content {
		if err := continueMCPStringKeys(child); err != nil {
			return err
		}
	}
	return nil
}

type continueMCPYAMLError struct{ err error }

func (e continueMCPYAMLError) Error() string { return "invalid YAML MCP data" }
func (e continueMCPYAMLError) Unwrap() error { return e.err }

func normalizeContinueMCPCredentials(server map[string]any) error {
	if opts := server["requestOptions"]; opts != nil {
		if _, ok := opts.(map[string]any); !ok {
			return fmt.Errorf("MCP requestOptions must be a mapping")
		}
	}
	for _, field := range []string{"env", "headers"} {
		if server[field] == nil {
			continue
		}
		raw, err := yaml.Marshal(server[field])
		if err != nil {
			return fmt.Errorf("marshal MCP %s: %w", field, err)
		}
		var values map[string]string
		if err := yaml.Unmarshal(raw, &values); err != nil {
			return fmt.Errorf("MCP %s must be a mapping of scalar values: %w", field, continueMCPYAMLError{err})
		}
		converted := make(map[string]any, len(values))
		for key, value := range values {
			converted[key] = value
		}
		server[field] = converted
	}
	return nil
}

// unvendorContinueServer rewrites the two Continue-native spellings back
// to the agnostic ones in place, so an imported spec stays portable to
// every other target rather than carrying Continue's dialect. `type:
// streamable-http` becomes `http`, the canonical spec spelling, which
// adapters whose vendor accepts only `http` would otherwise skip; and
// `requestOptions.headers` lifts back to a top-level `headers` map.
// Other `requestOptions` keys stay put: nothing else in the spec format
// has a home for them.
func unvendorContinueServer(server map[string]any) {
	if t, _ := server["type"].(string); t == "streamable-http" {
		server["type"] = "http"
	}
	opts, ok := server["requestOptions"].(map[string]any)
	if !ok {
		return
	}
	if headers, ok := opts["headers"]; ok {
		server["headers"] = headers
		delete(opts, "headers")
	}
	if len(opts) == 0 {
		delete(server, "requestOptions")
	}
}
