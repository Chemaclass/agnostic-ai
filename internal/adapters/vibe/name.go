package vibe

import (
	"fmt"
	"strings"
)

func nativeMCPName(name string) string {
	var out strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			out.WriteRune(r)
		} else {
			out.WriteByte('_')
		}
	}
	normalized := strings.Trim(out.String(), "_-")
	if len(normalized) > 256 {
		normalized = normalized[:256]
	}
	return normalized
}

// UserMCPArrayIdentities validates Vibe's runtime names without changing physical names.
func (Adapter) UserMCPArrayIdentities(records []map[string]any) (map[string]string, error) {
	out := map[string]string{}
	for _, record := range records {
		name, ok := record["name"].(string)
		if !ok {
			return nil, fmt.Errorf("vibe: MCP name must be a string")
		}
		id := nativeMCPName(name)
		if id == "" {
			return nil, fmt.Errorf("vibe: MCP name %q has an empty native identity", name)
		}
		if prior, ok := out[id]; ok {
			return nil, fmt.Errorf("vibe: MCP names %q and %q share native identity %q", prior, name, id)
		}
		out[id] = name
	}
	return out, nil
}

func validateConfigMCPNames(doc map[string]any) error {
	value, exists := doc["mcp_servers"]
	if !exists {
		return nil
	}
	var records []map[string]any
	switch list := value.(type) {
	case []map[string]any:
		records = list
	case []any:
		for _, item := range list {
			record, ok := item.(map[string]any)
			if !ok {
				return fmt.Errorf("vibe: mcp_servers must be an array of tables")
			}
			records = append(records, record)
		}
	default:
		return fmt.Errorf("vibe: mcp_servers must be an array of tables")
	}
	_, err := (Adapter{}).UserMCPArrayIdentities(records)
	return err
}
