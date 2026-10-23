package cli

import (
	"fmt"
	"maps"

	"github.com/chemaclass/agnostic-ai/internal/adapters/kimi"
)

func kimiHookIdentity(record map[string]any) string {
	canonical := maps.Clone(record)
	if command, ok := canonical["command"].(string); ok {
		canonical["command"] = kimi.DeclaredHookCommand(command)
	}
	return "kimi-" + globalSum(kimiRecordJSON(canonical))
}
func kimiHookRecordsByID(records []map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, r := range records {
		out[kimiHookIdentity(r)] = r
	}
	return out
}
func kimiHookPrevious(path string, records []any) (map[string]any, error) {
	out := map[string]any{}
	for _, raw := range records {
		r, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: invalid Kimi hook ownership record", path)
		}
		out[kimiHookIdentity(r)] = r
	}
	return out, nil
}
