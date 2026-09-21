package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	// claudeMCPFile is the project-root MCP server registry Claude Code reads.
	claudeMCPFile = ".mcp.json"
	// claudeMCPKey is the top-level JSON object holding the server map.
	claudeMCPKey = "mcpServers"
)

// importClaudeMCP reads `<root>/.mcp.json` and writes one yaml per
// `mcpServers.<name>` entry into dstDir. Project settings rejections restore
// the portable disabled flag. No-op when the registry is absent.
func importClaudeMCP(root, dstDir string) (int, error) {
	return importClaudeMCPWithSettings(root, dstDir, claudeDir)
}

func importClaudeMCPWithSettings(root, dstDir, settingsDir string) (int, error) {
	servers, err := readJSONMapAt(filepath.Join(root, claudeMCPFile), claudeMCPKey)
	if err != nil || len(servers) == 0 {
		return 0, err
	}
	path := filepath.Join(root, settingsDir, "settings.json")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	if err == nil {
		var settings struct {
			Disabled []string `json:"disabledMcpjsonServers"`
		}
		if err := json.Unmarshal(data, &settings); err != nil {
			return 0, fmt.Errorf("parse %s: %w", path, err)
		}
		for _, name := range settings.Disabled {
			if server, ok := servers[name].(map[string]any); ok {
				server["disabled"] = true
			}
		}
	}
	return writeMCPYAMLs(servers, dstDir)
}
