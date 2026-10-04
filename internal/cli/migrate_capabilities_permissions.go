package cli

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// capabilitiesSettingsPermissionsMigration rewrites each settings
// permission rule a capability stands for alone as that capability.
// Every other rule stays as an alias, which sync reads the same, so it
// writes the same files after it.
var capabilitiesSettingsPermissionsMigration = specMigration{
	ID:      "capabilities-settings-permissions",
	Group:   "capabilities",
	Release: "0.79.0",
	Summary: "rewrite settings permission rules with neutral capability names",
	Plan:    planCapabilitiesSettingsPermissions,
}

func planCapabilitiesSettingsPermissions(s migrationScope) ([]migrationChange, []migrationSkip, error) {
	b, layers, err := s.loadSpecs()
	if err != nil {
		return nil, nil, err
	}
	extended, err := extendedSpecNames(layers, func(lb spec.Bundle) []spec.Entry { return lb.Settings })
	if err != nil {
		return nil, nil, err
	}
	roots, packs := s.specRoots()
	var changes []migrationChange
	var skips []migrationSkip
	for _, e := range b.Settings {
		perms, ok := e.Meta["permissions"].(map[string]any)
		if !ok {
			continue
		}
		skip := func(reason string) { skips = append(skips, migrationSkip{Path: e.Path, Reason: reason}) }
		rewrites := map[string]map[int]string{}
		var kept []string
		for _, list := range spec.PermissionLists {
			rules, _ := perms[list].([]any)
			if hasWebPair(rules) {
				rewrites[list] = map[int]string{}
			}
			for i, raw := range rules {
				rule, _ := raw.(string)
				c, ok := spec.NeutralPermission(rule)
				if !ok {
					if rule != "" && isClaudeAliasRule(rule) && !slices.Contains(kept, rule) {
						kept = append(kept, rule)
					}
					continue
				}
				if got, problem := spec.PermissionRules(list, c); problem != "" || len(got) != 1 || got[0] != rule {
					continue
				}
				if rewrites[list] == nil {
					rewrites[list] = map[int]string{}
				}
				rewrites[list][i] = c
			}
		}
		if len(rewrites) == 0 {
			continue
		}
		if pack, ok := strings.CutPrefix(e.Layer, layerNamePackPrefix); ok {
			skips = append(skips, packSkip(e.Path, pack))
			continue
		}
		if extended[e.Name] {
			skips = append(skips, migrationSkip{Path: e.Path, Reason: "a local/ spec extends these settings; rewrite both files by hand", Actionable: true})
			continue
		}
		if outside, ok := s.outsideSpecRoots(migrationChange{Path: e.Path}, roots, packs); ok {
			skips = append(skips, outside)
			continue
		}
		body, err := os.ReadFile(e.Path)
		if err != nil {
			return nil, nil, err
		}
		after := string(body)
		for _, list := range spec.PermissionLists {
			if rewrites[list] == nil {
				continue
			}
			if after, err = rewriteYAMLSequenceItems(after, []string{"permissions", list}, rewrites[list]); err == nil {
				after, err = collapseWebPairs(after, []string{"permissions", list})
			}
			if err != nil {
				break
			}
		}
		if err != nil {
			skip("cannot rewrite in place: " + err.Error())
			continue
		}
		changes = append(changes, migrationChange{Path: e.Path, Before: string(body), After: after})
		for _, rule := range kept {
			skip(fmt.Sprintf("keeps %s as an alias: %s", rule, permissionAliasReason(rule)))
		}
	}
	return changes, skips, nil
}

// isClaudeAliasRule reports whether a rule is written in Claude Code
// names, so a kept one is worth a reason.
func isClaudeAliasRule(rule string) bool {
	_, problem := spec.PermissionRules("allow", rule)
	return problem == "" && rule[0] >= 'A' && rule[0] <= 'Z'
}

// permissionAliasReason says why a Claude Code rule has no capability of
// its own.
func permissionAliasReason(rule string) string {
	if strings.HasPrefix(rule, "Write(") {
		return "Claude Code never consults a Write(path) rule, and edit(<path>) would also cover edits"
	}
	return "no capability stands for it alone"
}
