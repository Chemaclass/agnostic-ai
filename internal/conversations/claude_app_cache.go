package conversations

import (
	"encoding/json"
	"errors"
)

const (
	maxClaudeCacheJSONBytes = 64 << 20
	maxClaudeCacheValues    = 1 << 20
	maxClaudeCacheDepth     = 256
)

func claudeAppCacheJSON(value any) ([]byte, error) {
	budget := claudeCacheBudget{bytes: maxClaudeCacheJSONBytes, values: maxClaudeCacheValues}
	if !budget.fits(value, 0) {
		return nil, errors.New("claude app cache exceeds the JSON expansion budget")
	}
	return json.Marshal(value)
}

type claudeCacheBudget struct {
	bytes, values int
}

// Charge each occurrence, since JSON expands shared references into separate values.
func (b *claudeCacheBudget) fits(value any, depth int) bool {
	if depth > maxClaudeCacheDepth || b.values == 0 || !b.take(1) {
		return false
	}
	b.values--
	switch value := value.(type) {
	case nil, bool, float64:
		return b.take(32)
	case string:
		if len(value) > b.bytes/6 {
			return false
		}
		return b.take(6*len(value) + 2)
	case []any:
		if !b.take(2) {
			return false
		}
		for _, item := range value {
			if !b.fits(item, depth+1) {
				return false
			}
		}
		return true
	case map[string]any:
		if !b.take(2) {
			return false
		}
		for key, item := range value {
			if !b.take(1) || !b.fits(key, depth+1) || !b.fits(item, depth+1) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (b *claudeCacheBudget) take(size int) bool {
	if size > b.bytes {
		return false
	}
	b.bytes -= size
	return true
}
