package cli

import (
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/vibe"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func stageGlobalTOMLArrays(home, stage string, targets []string, b spec.Bundle, state globalState, warn io.Writer) error {
	for _, target := range targets {
		g := globalTargets[target]
		if target == "kimi" {
			path := g.path(home, g.hooks)
			doc, err := readVibeTOML(path)
			if err != nil {
				return err
			}
			records, err := nativeTOMLRecords(path, "hooks", doc["hooks"])
			if err != nil {
				return err
			}
			previous, err := kimiHookPrevious(path, state.Hooks[target]["records"])
			if err != nil {
				return err
			}
			var user []map[string]any
			for id, r := range kimiHookRecordsByID(records) {
				if !sameSetting(r, previous[id]) {
					user = append(user, r)
				}
			}
			if err := importKimiHooks(path, user, filepath.Join(stage, "hooks")); err != nil {
				return err
			}
			continue
		}

		if g.hooksFormat == "vibe-toml" {
			path := g.path(home, g.hooks)
			doc, err := readVibeTOML(path)
			if err != nil {
				return err
			}
			records, err := nativeTOMLRecords(path, "hooks", doc["hooks"])
			if err != nil {
				return err
			}
			if err := validateImportedVibeHookNames(path, records); err != nil {
				return err
			}
			owned, err := namedTOMLAnyRecords(path, state.Hooks[target]["records"])
			if err != nil {
				return err
			}
			for _, r := range records {
				name, _ := r["name"].(string)
				if sameSetting(r, owned[name]) {
					continue
				}
				covered := false
				for _, e := range b.Hooks {
					if e.Name == name && e.EmitsTo(target) {
						covered = true
						break
					}
				}
				if covered {
					continue
				}
				meta := map[string]any{"name": name, "target": target, "event": r["type"], "command": vibe.DeclaredHookCommand(fmt.Sprint(r["command"]))}
				rest := maps.Clone(r)
				delete(rest, "name")
				delete(rest, "type")
				delete(rest, "command")
				if len(rest) > 0 {
					meta["x-"+target] = rest
				}
				if err := importMkdirAll(filepath.Join(stage, "hooks"), 0o755); err != nil {
					return fmt.Errorf("create hook source directory: %w", err)
				}
				if err := writeHookSpecFile(filepath.Join(stage, "hooks"), name, meta); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func stageGlobalMCPArray(path, key, target, dir string, homeRecords []map[string]any, previous map[string]any, skip func(string, string, string) error) (map[string]any, error) {
	doc, err := readVibeTOML(path)
	if err != nil {
		return nil, err
	}
	records, err := nativeTOMLRecords(path, key, doc[key])
	if err != nil {
		return nil, err
	}
	if err := validateImportedVibeMCPNames(path, records); err != nil {
		return nil, err
	}
	homeIDs, err := adapters.UserMCPArrayIdentities(target, homeRecords)
	if err != nil {
		return nil, err
	}
	homeByName, err := namedTOMLRecords(path, homeRecords)
	if err != nil {
		return nil, err
	}
	current := map[string]tomlArrayRecord{}
	for _, r := range records {
		current[r["name"].(string)] = tomlArrayRecord{value: r}
	}
	if _, err := reconcileVibeMCPNames(path, nil, current, previous); err != nil {
		return nil, err
	}
	var user []map[string]any
	native := map[string]any{}
	for _, r := range records {
		name, _ := r["name"].(string)
		if sameSetting(r, previous[name]) {
			continue
		}
		ids, err := adapters.UserMCPArrayIdentities(target, []map[string]any{r})
		if err != nil {
			return nil, err
		}
		for id := range ids {
			if sourceName, covered := homeIDs[id]; covered {
				if !sameSetting(adapters.CanonicalUserArrayRecord(target, key, r), adapters.CanonicalUserArrayRecord(target, key, homeByName[sourceName])) {
					return nil, fmt.Errorf("%s: MCP %q conflicts with existing home spec %q (native identity %q)", path, name, sourceName, id)
				}
				name = ""
			}
		}
		if name == "" {
			continue
		}
		user = append(user, r)
		native[name] = jsonRoundTrip(r)
	}
	safe := map[string]any{}
	for _, r := range user {
		name, _ := r["name"].(string)
		probe := maps.Clone(r)
		if auth, ok := r["auth"].(map[string]any); ok {
			if h, ok := auth["headers"]; ok {
				probe["headers"] = h
			}
		}
		safe[name] = probe
	}
	if err := withoutMCPCredentials(target, safe, skip); err != nil {
		return nil, err
	}
	if err := withoutDataStreamMCPNames(target, path, dir, safe, skip); err != nil {
		return nil, err
	}
	user = slices.DeleteFunc(user, func(r map[string]any) bool {
		name, _ := r["name"].(string)
		_, ok := safe[name]
		if !ok {
			delete(native, name)
		}
		return !ok
	})
	if err := importVibeMCPRecords(user, dir); err != nil {
		return nil, err
	}
	return native, nil
}
func validateGlobalTOMLNames(home string, targets []string) error {
	for _, target := range targets {
		g := globalTargets[target]
		if target == "vibe" {
			path := g.mcpPath(home)
			doc, err := readVibeTOML(path)
			if err != nil {
				return err
			}
			records, err := nativeTOMLRecords(path, g.mcp.key, doc[g.mcp.key])
			if err != nil {
				return err
			}
			if err := validateImportedVibeMCPNames(path, records); err != nil {
				return err
			}
		}
		if g.hooksFormat != "vibe-toml" {
			continue
		}
		path := g.path(home, g.hooks)
		doc, err := readVibeTOML(path)
		if err != nil {
			return err
		}
		records, err := nativeTOMLRecords(path, "hooks", doc["hooks"])
		if err != nil {
			return err
		}
		if err := validateImportedVibeHookNames(path, records); err != nil {
			return err
		}
	}
	return nil
}
