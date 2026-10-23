package mcpprobe

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"time"
)

// CacheVersion is the shape of the cache file.
const CacheVersion = 1

// Cache is what the last `agnostic-ai mcp tools` run found, per server,
// for one configuration root. The overview reads it and never connects.
type Cache struct {
	Version int                     `json:"version"`
	Root    string                  `json:"root"`
	Servers map[string]ServerResult `json:"servers"`
}

// ServerResult is one server's listing.
type ServerResult struct {
	// Spec fingerprints the server's spec when it was listed, so a view
	// can tell that the listing predates an edit.
	Spec     string    `json:"spec"`
	ListedAt time.Time `json:"listedAt"`
	Tools    []Tool    `json:"tools"`
	// Error says why the listing failed; Tools is then empty.
	Error string `json:"error,omitempty"`
}

// CachePath is where the listing for the configuration at root lives:
// the user cache directory, outside any repository, keyed by the root.
func CachePath(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("user cache directory: %w", err)
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(abs))
	return filepath.Join(dir, "agnostic-ai", "mcp-tools", hex.EncodeToString(h.Sum(nil))+".json"), nil
}

// LoadCache reads the listing for root; a missing or unreadable file is
// an empty cache.
func LoadCache(root string) Cache {
	empty := Cache{Version: CacheVersion, Servers: map[string]ServerResult{}}
	path, err := CachePath(root)
	if err != nil {
		return empty
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return empty
	}
	var c Cache
	if json.Unmarshal(data, &c) != nil || c.Version != CacheVersion || c.Servers == nil {
		return empty
	}
	return c
}

// SaveCache writes the listing for root, replacing the previous one.
func SaveCache(root string, c Cache) error {
	path, err := CachePath(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// SpecFingerprint names a server spec's fields, so a change to any of
// them marks its listing stale.
func SpecFingerprint(meta map[string]any) string {
	fields := make(map[string]any, len(meta))
	for k, v := range meta {
		if k != "description" {
			fields[k] = v
		}
	}
	data, _ := json.Marshal(fields)
	h := fnv.New64a()
	_, _ = h.Write(data)
	return hex.EncodeToString(h.Sum(nil))[:12]
}
