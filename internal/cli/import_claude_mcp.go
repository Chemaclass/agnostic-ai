package cli

import (
	"path/filepath"
)

const (
	// claudeMCPFile is the project-root MCP server registry Claude Code reads.
	claudeMCPFile = ".mcp.json"
	// claudeMCPKey is the top-level JSON object holding the server map.
	claudeMCPKey = "mcpServers"
)

// importClaudeMCP reads `<root>/.mcp.json` and writes one yaml per
// `mcpServers.<name>` entry into dstDir. No-op when the file is absent.
func importClaudeMCP(root, dstDir string) (int, error) {
	return importJSONMCPMap(filepath.Join(root, claudeMCPFile), claudeMCPKey, dstDir)
}
