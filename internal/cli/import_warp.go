package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

const (
	warpMainFile     = "AGENTS.md"
	warpWorkflowsDir = ".warp/workflows"
	warpMCPFile      = ".warp/.mcp.json"
	warpMCPKey       = "mcpServers"
)

// importFromWarp reads an existing Warp project (AGENTS.md,
// `.warp/workflows/`, `.warp/.mcp.json`) under root and writes specs
// into the configured source directories.
func importFromWarp(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.MCPs); err != nil {
		return err
	}
	rules, err := sliceMainFileByH2(root, warpMainFile, filepath.Join(root, src.Rules))
	if err != nil {
		return err
	}
	agents, err := importWarpWorkflows(root, filepath.Join(root, src.Agents))
	if err != nil {
		return err
	}
	mcps, err := importJSONMCPMap(filepath.Join(root, warpMCPFile), warpMCPKey, filepath.Join(root, src.MCPs))
	if err != nil {
		return err
	}
	if _, err := mirrorMainFile(root, warpMainFile); err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d mcps\n", rules, agents, mcps)
	printImportNextSteps(root, "warp")
	return nil
}

// importWarpWorkflows reads `.warp/workflows/*.yaml` and writes one
// agent spec per workflow into dstDir. The workflow `command` field
// becomes the agent body; `description` and `tags` survive in frontmatter.
func importWarpWorkflows(root, dstDir string) (int, error) {
	src := filepath.Join(root, warpWorkflowsDir)
	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		full := filepath.Join(src, name)
		data, err := os.ReadFile(full)
		if err != nil {
			return count, fmt.Errorf("read %s: %w", full, err)
		}
		var doc map[string]any
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return count, fmt.Errorf("parse %s: %w", full, err)
		}
		agentName, _ := doc["name"].(string)
		if agentName == "" {
			agentName = strings.TrimSuffix(strings.TrimSuffix(name, ".yml"), ".yaml")
		}
		body, _ := doc["command"].(string)
		desc, _ := doc["description"].(string)
		tags := stringSliceFromAny(doc["tags"])
		out := filepath.Join(dstDir, agentName+".md")
		if err := writeWarpAgentMD(out, agentName, desc, tags, body, doc); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// warpWorkflowTopLevel are the workflow keys captured as first-class
// agent frontmatter fields (or the body). Any other documented workflow
// field (docs.warp.dev/terminal/entry/yaml-workflows: `shells`,
// `arguments`, `source_url`, `author`, `author_url`, ...) is preserved
// under `x-warp` so a sync -> import -> sync cycle does not silently
// drop it (#538).
var warpWorkflowTopLevel = map[string]bool{
	"name": true, "command": true, "description": true, "tags": true,
}

// writeWarpAgentMD writes an imported Warp workflow as an agent spec.
// Keeps the historic name/description/tags frontmatter shape (tags in
// flow style) byte-compatible with writeAgentMD's output, then appends
// any other workflow field under `x-warp` so it survives the round-trip.
func writeWarpAgentMD(path, name, description string, tags []string, body string, doc map[string]any) error {
	xwarp := map[string]any{}
	for k, v := range doc {
		if warpWorkflowTopLevel[k] {
			continue
		}
		xwarp[k] = v
	}
	fm := map[string]any{"name": name}
	if description != "" {
		fm["description"] = description
	}
	if len(tags) > 0 {
		fm["tags"] = tags
	}
	if len(xwarp) > 0 {
		fm["x-warp"] = xwarp
	}
	raw, err := marshalOrderedFrontmatter(fm, []string{"name", "description", "tags"}, "x-warp")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", name, err)
	}
	out := "---\n" + string(raw) + "---\n\n" + strings.TrimRight(body, "\n") + "\n"
	if err := importWriteFile(path, []byte(out), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// marshalOrderedFrontmatter renders fm as YAML with order's keys first
// (skipping any absent from fm), then trailing last (skipping if
// absent). `tags`, when present, renders in flow style (`[a, b]`) to
// match the historic Warp import format. import_codex_native.go's
// marshalAgentFrontmatter serves the same purpose for codex's larger,
// claude-shaped key set; this stays local since warp's key set is
// small and fixed.
func marshalOrderedFrontmatter(fm map[string]any, order []string, trailing string) ([]byte, error) {
	root := &yaml.Node{Kind: yaml.MappingNode}
	add := func(key string) error {
		val, ok := fm[key]
		if !ok {
			return nil
		}
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
		valNode := &yaml.Node{}
		if err := valNode.Encode(val); err != nil {
			return err
		}
		if key == "tags" {
			valNode.Style = yaml.FlowStyle
		}
		root.Content = append(root.Content, keyNode, valNode)
		return nil
	}
	for _, k := range order {
		if err := add(k); err != nil {
			return nil, err
		}
	}
	if err := add(trailing); err != nil {
		return nil, err
	}
	return yaml.Marshal(root)
}
