package cli

import (
	"fmt"
	"os"

	"github.com/chemaclass/agnostic-ai/internal/adapters/codebuddy"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func readCodebuddyMCPMap(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	doc, err := settingsValues(path, "json", data)
	if err != nil {
		return nil, err
	}
	raw, set := doc["mcpServers"]
	if !set {
		return nil, nil
	}
	servers, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: codebuddy: mcpServers must be an object", path)
	}
	for name, raw := range servers {
		server, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: codebuddy: MCP %q must be an object", path, name)
		}
		kind, err := codebuddy.MCPType(server)
		if err != nil {
			return nil, fmt.Errorf("%s: codebuddy: MCP %q: %w", path, name, err)
		}
		server["type"] = kind
		if err := codebuddy.ValidateMCP([]spec.Entry{{Name: name, Path: path, Meta: server}}); err != nil {
			return nil, err
		}
	}
	return servers, nil
}
