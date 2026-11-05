package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// mcpsCodexEnvironmentIDMigration renames the Codex-only MCP field
// `experimental_environment: local` to `environment_id: local`. Codex
// replaced the `local | remote` key with an executor ID (#2976), and sync
// writes the old `local` as `environment_id = "local"`, so the output
// stays the same. Any other value names no executor and stays for the
// user to change.
var mcpsCodexEnvironmentIDMigration = specMigration{
	ID:      "mcps-codex-environment-id",
	Group:   "mcps",
	Release: "0.92.0",
	Summary: "rename the Codex MCP field experimental_environment: local to environment_id: local",
	Plan:    planMCPsCodexEnvironmentID,
}

func planMCPsCodexEnvironmentID(s migrationScope) ([]migrationChange, []migrationSkip, error) {
	b, layers, err := s.loadSpecs()
	if err != nil {
		return nil, nil, err
	}
	extended, err := s.extendedSpecNames(layers, func(lb spec.Bundle) []spec.Entry { return lb.MCPs })
	if err != nil {
		return nil, nil, err
	}
	roots, packs := s.specRoots()
	var changes []migrationChange
	var skips []migrationSkip
	for _, m := range b.MCPs {
		old, ok := m.Meta["experimental_environment"]
		if !ok {
			continue
		}
		skip := func(reason string) {
			skips = append(skips, migrationSkip{Path: m.Path, Reason: reason, Actionable: true})
		}
		if pack, ok := strings.CutPrefix(m.Layer, layerNamePackPrefix); ok {
			skips = append(skips, packSkip(m.Path, pack))
			continue
		}
		if extended[m.Name] {
			skip("a local/ spec extends this MCP server; rewrite both files by hand")
			continue
		}
		if _, both := m.Meta["environment_id"]; both {
			skip("sets both environment_id and experimental_environment; remove experimental_environment by hand")
			continue
		}
		if old != "local" {
			skip(fmt.Sprintf("experimental_environment: %v names no executor; set environment_id to a configured executor ID, or remove the key", old))
			continue
		}
		if outside, ok := s.outsideSpecRoots(migrationChange{Path: m.Path}, roots, packs); ok {
			skips = append(skips, outside)
			continue
		}
		body, err := os.ReadFile(m.Path)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", m.Path, err)
		}
		after, err := rewriteTopLevelYAMLKeys(string(body), []yamlKeyRewrite{{Key: "experimental_environment", NewKey: "environment_id", Value: "local"}})
		if err != nil {
			skip("cannot rewrite in place: " + err.Error())
			continue
		}
		changes = append(changes, migrationChange{Path: m.Path, Before: string(body), After: after})
	}
	return changes, skips, nil
}
