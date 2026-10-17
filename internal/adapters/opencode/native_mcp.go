package opencode

import (
	"fmt"
	"maps"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func validateMCPOwners(mcps, settings []spec.Entry) error {
	var owner *spec.Entry
	for i := range settings {
		entry := &settings[i]
		x, _ := entry.Meta["x-opencode"].(map[string]any)
		if _, present := x["mcp"]; !present {
			continue
		}
		if len(mcps) > 0 {
			return fmt.Errorf("opencode: settings %s claims mcp already owned by MCP spec %s", entry.Path, mcps[0].Path)
		}
		if owner != nil {
			return fmt.Errorf("opencode: settings %s and %s both claim mcp", owner.Path, entry.Path)
		}
		owner = entry
	}
	return nil
}

func mergeNativeMCP(sess *emit.Session, keys map[string]any, path string, dryRun bool) (bool, error) {
	mcp, object := keys["mcp"].(map[string]any)
	servers, native := mcp["servers"].(map[string]any)
	if !object || !native {
		return false, nil
	}
	// A V1 server may itself be named servers.
	if _, command := servers["command"].([]string); command {
		return false, nil
	}
	if _, command := servers["command"].([]any); command {
		return false, nil
	}
	if _, command := servers["command"].(string); command {
		return false, nil
	}
	if _, url := servers["url"].(string); url {
		return false, nil
	}
	mcp = maps.Clone(mcp)
	existing := sess.ExistingObjectAt(path, []string{"mcp", "servers"}, dryRun)
	if existing == nil {
		existing = emit.NewOrderedJSON()
	}
	for _, name := range emit.PriorClaimedEntries(path, []string{"mcp", "servers"}) {
		if _, wanted := servers[name]; !wanted && sess.ClaimsUnchangedValue(path, dryRun, "mcp", "servers", name) {
			existing.Delete(name)
		}
	}
	names := slices.Sorted(maps.Keys(servers))
	for _, name := range names {
		if err := existing.Set(name, servers[name]); err != nil {
			return false, fmt.Errorf("%s: mcp.servers.%s cannot be encoded: %w", path, name, err)
		}
	}
	mcp["servers"] = emit.ClaimedJSONEntries(existing, names)
	keys["mcp"] = mcp
	return true, nil
}
