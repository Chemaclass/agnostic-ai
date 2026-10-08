package cli

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

var capabilitiesSkillToolsMigration = specMigration{ID: "capabilities-skill-tools", Group: "capabilities", Release: "0.79.0", Summary: "rewrite skill allowed-tools with neutral capability names", Plan: planCapabilitiesSkillTools}

func planCapabilitiesSkillTools(s migrationScope) ([]migrationChange, []migrationSkip, error) {
	b, layers, err := s.loadSpecs()
	if err != nil {
		return nil, nil, err
	}
	extended, err := s.extendedSpecNames(layers, func(lb spec.Bundle) []spec.Entry { return lb.Skills })
	if err != nil {
		return nil, nil, err
	}
	roots, packs := s.specRoots()
	var changes []migrationChange
	var skips []migrationSkip
	for _, a := range b.Skills {
		raw, hasTools := a.Meta["allowed-tools"]
		if !hasTools {
			continue
		}
		skip := func(reason string) { skips = append(skips, migrationSkip{Path: a.Path, Reason: reason}) }
		if pack, ok := strings.CutPrefix(a.Layer, layerNamePackPrefix); ok {
			skips = append(skips, packSkip(a.Path, pack))
			continue
		}
		if extended[a.Name] {
			skips = append(skips, migrationSkip{Path: a.Path, Reason: "a local/ spec extends this skill; rewrite both files by hand", Actionable: true})
			continue
		}
		list, ok := raw.([]any)
		if !ok {
			skip("allowed-tools: is not a list")
			continue
		}
		items := map[int]string{}
		var kept []string
		tools := make([]string, len(list))
		for i, v := range list {
			name, _ := v.(string)
			tools[i] = name
			if c, ok := spec.NeutralCapability(name); ok {
				items[i] = c
			} else {
				kept = append(kept, name)
			}
		}
		if len(items) == 0 && !hasWebPair(list) {
			skip("keeps allowed-tools: as written: no entry has a capability of its own")
			continue
		}
		if outside, ok := s.outsideSpecRoots(migrationChange{Path: a.Path}, roots, packs); ok {
			skips = append(skips, outside)
			continue
		}
		can := slices.Clone(tools)
		for i, c := range items {
			can[i] = c
		}
		if got, problem := spec.CapabilityTools(can); problem != "" || !slices.Equal(got, tools) {
			skip("allowed-tools: would not stand for the same tools")
			continue
		}
		body, err := os.ReadFile(a.Path)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", a.Path, err)
		}
		after, err := rewriteFrontmatter(string(body), func(front string) (string, error) {
			rewritten, err := rewriteTopLevelYAMLSequence(front, yamlSequenceRewrite{Key: "allowed-tools", NewKey: "allowed-tools", Items: items})
			if err != nil {
				return "", err
			}
			return collapseWebPairs(rewritten, []string{"allowed-tools"})
		})
		if err != nil {
			skip("cannot rewrite in place: " + err.Error())
			continue
		}
		changes = append(changes, migrationChange{Path: a.Path, Before: string(body), After: after})
		for _, reason := range aliasReasons(withoutAdjacentWebPair(kept)) {
			skip(reason)
		}
	}
	return changes, skips, nil
}
