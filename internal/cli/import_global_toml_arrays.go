package cli

import (
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/vibe"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func stageGlobalTOMLArrays(home, stage string, targets []string, b spec.Bundle, state globalState, warn io.Writer) error {
	for _, target := range targets {
		g := globalTargets[target]

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

func stageGlobalMCPArray(path, key, target, dir string, have map[string]bool, previous map[string]any, skip func(string, string, string) error) (map[string]any, error) {
	doc, err := readVibeTOML(path)
	if err != nil {
		return nil, err
	}
	records, err := nativeTOMLRecords(path, key, doc[key])
	if err != nil {
		return nil, err
	}
	if _, err := namedTOMLRecords(path, records); err != nil {
		return nil, err
	}
	var user []map[string]any
	native := map[string]any{}
	for _, r := range records {
		name, _ := r["name"].(string)
		if have[name] || sameSetting(r, previous[name]) {
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
func validateGlobalTOMLHookNames(home string, targets []string) error {
	for _, target := range targets {
		g := globalTargets[target]
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
