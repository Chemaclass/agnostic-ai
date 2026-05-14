package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// importJSONMCPMap reads a JSON file at srcPath, extracts the
// flat-or-dotted key, and writes one yaml per server into dstDir.
// Common helper for amp / opencode / vscode-style MCP shapes where
// servers are a map keyed by name.
func importJSONMCPMap(srcPath, mapKey, dstDir string) (int, error) {
	servers, err := readJSONMapAt(srcPath, mapKey)
	if err != nil || len(servers) == 0 {
		return 0, err
	}
	return writeMCPYAMLs(servers, dstDir)
}

// readJSONMapAt loads srcPath as JSON and returns the map at mapKey.
// Supports a dotted key like "amp.mcpServers" so callers can target a
// nested key without writing custom decoders. Missing file or missing
// key returns (nil, nil); only IO and parse failures bubble up.
func readJSONMapAt(srcPath, mapKey string) (map[string]any, error) {
	data, err := os.ReadFile(srcPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", srcPath, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", srcPath, err)
	}
	v, ok := doc[mapKey]
	if !ok {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, nil
	}
	out := map[string]any{}
	for k, val := range m {
		sub, ok := val.(map[string]any)
		if !ok {
			continue
		}
		out[k] = sub
	}
	return out, nil
}

// writeMCPYAMLs writes one yaml file per server into dstDir. Each
// destination doc has `name: <key>` prepended; server fields pass
// through verbatim so transport-specific keys (command/args/env or
// url/headers) survive a round-trip.
func writeMCPYAMLs(servers map[string]any, dstDir string) (int, error) {
	names := make([]string, 0, len(servers))
	for k := range servers {
		names = append(names, k)
	}
	sort.Strings(names)
	count := 0
	for _, name := range names {
		entry, _ := servers[name].(map[string]any)
		doc := map[string]any{"name": name}
		for k, v := range entry {
			doc[k] = v
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
