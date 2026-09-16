package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteMCPYAMLs_PackageNameUsesFlatFilename(t *testing.T) {
	dir := t.TempDir()
	want := "npm:@modelcontextprotocol/server-sequential.thinking"
	count, err := writeMCPYAMLs(map[string]any{
		want: map[string]any{"command": "npx"},
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("wrote %d specs, want 1", count)
	}
	path := filepath.Join(dir, "npm%3A%40modelcontextprotocol%2Fserver-sequential.thinking.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read encoded MCP spec: %v", err)
	}
	if !strings.Contains(string(data), "name: "+want) {
		t.Errorf("logical MCP name changed:\n%s", data)
	}
}
