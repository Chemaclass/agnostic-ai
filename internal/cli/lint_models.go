package cli

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// lintModels checks spec models against the enabled targets. A tier that
// names a model, used by a spec with no entry and no `default` for one of
// the spec's targets, is LINT025: the spec falls back to that tool's
// default model. So is a tier named like a Claude model. A Claude model
// name that reaches a target unable to load it, through a tier's
// `default` or an agent's shared `model`, is LINT026.
// Tier findings name configPath, the config that declares tiers.
func lintModels(tiers map[string]config.ModelTier, configPath string, targets []string, support kindSupport, b spec.Bundle) []lintFinding {
	sorted := slices.Sorted(slices.Values(targets))
	var agentTargets []string
	for _, target := range sorted {
		if _, ok := support[spec.KindAgent][target]; ok {
			agentTargets = append(agentTargets, target)
		}
	}
	agents := b.Agents
	reached := map[string]map[string]bool{}
	for _, entries := range [][]spec.Entry{b.Agents, b.Skills, b.Commands, b.Settings} {
		for _, entry := range entries {
			if entry.ModelTier == "" {
				continue
			}
			for _, target := range sorted {
				if _, ok := support[entry.Kind][target]; ok && entry.EmitsTo(target) {
					if reached[entry.ModelTier] == nil {
						reached[entry.ModelTier] = map[string]bool{}
					}
					reached[entry.ModelTier][target] = true
				}
			}
		}
	}
	var findings []lintFinding
	for _, name := range sortedTierNames(tiers) {
		tier := tiers[name]
		if adapters.ClaudeModel(name) {
			findings = append(findings, lintFinding{Code: "LINT025", Severity: lintWarn, Path: configPath, Message: tierNameShadowsClaudeModel(name)})
		}
		if len(tier.Models) == 0 {
			continue
		}
		shared, hasDefault := tier.Models["default"]
		var missing, foreign []string
		for _, target := range sorted {
			if !reached[name][target] {
				continue
			}
			if _, own := tier.Models[target]; own {
				continue
			}
			switch {
			case !hasDefault:
				missing = append(missing, target)
			case adapters.ForeignClaudeModel(target, shared):
				foreign = append(foreign, target)
			}
		}
		if len(missing) > 0 {
			findings = append(findings, lintFinding{Code: "LINT025", Severity: lintWarn, Path: configPath,
				Message: fmt.Sprintf("models.%s has no %s model and no default; specs naming it use that tool's default model", name, strings.Join(missing, ", "))})
		}
		if len(foreign) > 0 {
			findings = append(findings, lintFinding{Code: "LINT026", Severity: lintWarn, Path: configPath,
				Message: fmt.Sprintf("models.%s.default %q is a Claude model name %s cannot load; name a model for each under models.%s", name, shared, strings.Join(foreign, ", "), name)})
		}
	}
	for _, agent := range agents {
		if agent.ModelTier != "" {
			continue
		}
		byModel := map[string][]string{}
		for _, target := range agentTargets {
			if !agent.EmitsTo(target) {
				continue
			}
			if model := adapters.SharedModel(agent.Meta, target); adapters.ForeignClaudeModel(target, model) {
				byModel[model] = append(byModel[model], target)
			}
		}
		for model, foreign := range byModel {
			findings = append(findings, lintFinding{Code: "LINT026", Severity: lintWarn, Path: agent.Path,
				Message: fmt.Sprintf("model %q is a Claude model name %s cannot load; write model: {claude: %s} or name a tier", model, strings.Join(foreign, ", "), model)})
		}
	}
	return findings
}

func sortedTierNames(tiers map[string]config.ModelTier) []string {
	names := make([]string, 0, len(tiers))
	for name := range tiers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
