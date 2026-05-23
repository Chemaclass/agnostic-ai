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
		if err := writeAgentMD(out, agentName, desc, tags, body); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
