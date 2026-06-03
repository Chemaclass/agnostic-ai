package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

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

// importContinueMCPs copies each `.continue/mcpServers/<name>.yaml`
// file into <dstDir>/<name>.yaml with the provenance header stripped
// so the spec lands clean for the next sync.
func importContinueMCPs(root, dstDir string) (int, error) {
	srcDir := filepath.Join(root, continueMCPServersDir)
	entries, err := os.ReadDir(srcDir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", srcDir, err)
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		src := filepath.Join(srcDir, e.Name())
		data, err := os.ReadFile(src)
		if err != nil {
			return count, fmt.Errorf("read %s: %w", src, err)
		}
		body := unwrapContinueMCP(strings.TrimLeft(header.Strip(string(data)), "\n"))
		dst := filepath.Join(dstDir, e.Name())
		if err := importMkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return count, fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
		}
		if err := importWriteFile(dst, []byte(body), 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", dst, err)
		}
		count++
	}
	return count, nil
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
	raw, err := yaml.Marshal(server)
	if err != nil {
		return body
	}
	return string(raw)
}
