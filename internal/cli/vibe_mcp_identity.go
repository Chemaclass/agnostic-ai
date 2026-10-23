package cli

import (
	"fmt"
	"maps"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

func validateVibeMCPDocument(path string, data []byte) error {
	doc, err := decodeTOMLDocument(path, data)
	if err != nil {
		return err
	}
	records, err := nativeTOMLRecords(path, "mcp_servers", doc["mcp_servers"])
	if err != nil {
		return err
	}
	if _, err := adapters.UserMCPArrayIdentities("vibe", records); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func reconcileVibeMCPNames(path string, want map[string]map[string]any, current map[string]tomlArrayRecord, previous map[string]any) (map[string]map[string]any, error) {
	desired := make([]map[string]any, 0, len(want))
	for name, r := range want {
		if r["name"] != name {
			return nil, fmt.Errorf("%s: desired MCP key %q disagrees with its native name", path, name)
		}
		desired = append(desired, r)
	}
	desiredIDs, err := adapters.UserMCPArrayIdentities("vibe", desired)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	present := make([]map[string]any, 0, len(current))
	for _, r := range current {
		present = append(present, r.value)
	}
	currentIDs, err := adapters.UserMCPArrayIdentities("vibe", present)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	prior := make([]map[string]any, 0, len(previous))
	for name, raw := range previous {
		record, ok := raw.(map[string]any)
		if !ok || record["name"] != name {
			return nil, fmt.Errorf("%s: owned MCP key %q disagrees with its native name", path, name)
		}
		prior = append(prior, record)
	}
	previousIDs, err := adapters.UserMCPArrayIdentities("vibe", prior)
	if err != nil {
		return nil, fmt.Errorf("%s: ownership state: %w", path, err)
	}
	for id, name := range previousIDs {
		if actual, ok := currentIDs[id]; ok && actual != name {
			return nil, fmt.Errorf("%s: owned MCP %q was renamed to native alias %q", path, name, actual)
		}
	}
	reconciled := make(map[string]map[string]any, len(want))
	for id, name := range desiredIDs {
		desired := want[name]
		actual, exists := currentIDs[id]
		if !exists || actual == name {
			reconciled[name] = desired
			continue
		}
		have := current[actual].value
		if sameSetting(adapters.CanonicalUserArrayRecord("vibe", "mcp_servers", have), adapters.CanonicalUserArrayRecord("vibe", "mcp_servers", desired)) {
			reconciled[actual] = have
		} else if old, ok := previous[actual]; ok && sameSetting(have, old) {
			update := maps.Clone(desired)
			update["name"] = actual
			reconciled[actual] = update
		} else {
			return nil, fmt.Errorf("%s: MCP %q conflicts with native alias %q (identity %q)", path, name, actual, id)
		}
	}
	return reconciled, nil
}
