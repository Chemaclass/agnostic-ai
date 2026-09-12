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
)

const (
	continueRulesDir      = ".continue/rules"
	continueMCPServersDir = ".continue/mcpServers"
)

// importFromContinue reads an existing Continue (continue.dev) project
// under root and writes specs into the configured source directories.
//
//   - `.continue/rules/*.md` walks via the shared rules-directory
//     importer (agent-<name>.md routes into agents, skill-<name>.md
//     into skills, the rest into rules; provenance + leading H1 are
//     stripped).
//   - `.continue/mcpServers/*.yaml` copies one MCP spec per file with
//     the provenance header stripped on the way back in.
//   - `.continue/mcpServers/*.json` accepts JSONC with a named
//     `mcpServers` map or a single server named after its source file.
func importFromContinue(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.MCPs); err != nil {
		return err
	}
	c, err := importRulesDirectory(root, continueRulesDir, src)
	if err != nil {
		return err
	}
	mcps, err := importContinueMCPs(root, filepath.Join(root, src.MCPs))
	if err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d mcps\n",
		c.rules, c.agents, c.skills, mcps)
	printImportNextSteps(root, "continue")
	return nil
}

type continueMCPFile struct {
	name string
	body []byte
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
			body := unwrapContinueMCP(strings.TrimLeft(header.Strip(string(data)), "\n"))
			imported = []continueMCPFile{{name: e.Name(), body: []byte(body)}}
		}
		for _, file := range imported {
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
	if err := importMkdirAll(dstDir, 0o755); err != nil {
		return 0, fmt.Errorf("mkdir %s: %w", dstDir, err)
	}
	count := 0
	for _, file := range files {
		dst := filepath.Join(dstDir, file.name)
		if err := importWriteFile(dst, file.body, 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", dst, err)
		}
		count++
	}
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
	files := make([]continueMCPFile, 0, len(names))
	for _, name := range names {
		if !filepath.IsLocal(name) || name == "." || strings.ContainsAny(name, "/\\\x00") {
			return nil, fmt.Errorf("parse %s: invalid MCP server name %q: must be a single safe path segment", src, name)
		}
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
		files = append(files, continueMCPFile{name: name + ".yaml", body: raw})
	}
	return files, nil
}

// unwrapContinueMCP converts a Continue block file (`name`/`version`/
// `schema: v1` wrapper with the server nested under `mcpServers:`) back
// into a flat single-server MCP spec. When the document is not a block
// wrapper (a hand-authored flat file, or a future schema), the input is
// returned unchanged so import stays lossless.
func unwrapContinueMCP(body string) string {
	var doc struct {
		MCPServers []map[string]any `yaml:"mcpServers"`
	}
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil || len(doc.MCPServers) == 0 {
		return body
	}
	server := doc.MCPServers[0]
	if len(server) == 0 {
		return body
	}
	unvendorContinueServer(server)
	raw, err := yaml.Marshal(server)
	if err != nil {
		return body
	}
	return string(raw)
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
