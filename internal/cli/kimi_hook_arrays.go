package cli

import (
	"fmt"
	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

func kimiHookRecordsByID(records []map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, r := range records {
		out["kimi-"+globalSum(kimiRecordJSON(adapters.CanonicalUserArrayRecord("kimi", "hooks", r).(map[string]any)))[:12]] = r
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
		out["kimi-"+globalSum(kimiRecordJSON(adapters.CanonicalUserArrayRecord("kimi", "hooks", r).(map[string]any)))[:12]] = r
	}
	return out, nil
}
