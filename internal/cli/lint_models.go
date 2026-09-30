package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// lintModels checks agent models against the enabled targets that write
// agents. A tier an agent names with no entry and no `default` for one of
// the agent's targets is LINT025: the agent falls back to that tool's
// default model. A Claude model name that reaches a target unable to load
// it, through a tier's `default` or an agent's shared `model`, is LINT026.
func lintModels(cfg *config.Config, targets []string, support kindSupport, agents []spec.Entry) []lintFinding {
	var agentTargets []string
	for _, target := range targets {
		if _, ok := support[spec.KindAgent][target]; ok {
			agentTargets = append(agentTargets, target)
		}
	}
	sort.Strings(agentTargets)
	reached := map[string]map[string]bool{}
	for _, agent := range agents {
		if agent.ModelTier == "" {
			continue
		}
		for _, target := range agentTargets {
			if agent.EmitsTo(target) {
				if reached[agent.ModelTier] == nil {
					reached[agent.ModelTier] = map[string]bool{}
				}
				reached[agent.ModelTier][target] = true
			}
		}
	}
	var findings []lintFinding
	for _, name := range sortedTierNames(cfg.Models) {
		tier := cfg.Models[name]
		shared, hasDefault := tier.Models["default"]
		var missing, foreign []string
		for _, target := range agentTargets {
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
			findings = append(findings, lintFinding{Code: "LINT025", Severity: lintWarn, Path: config.ConfigFileName,
				Message: fmt.Sprintf("models.%s has no %s model and no default; agents naming it use that tool's default model", name, strings.Join(missing, ", "))})
		}
		if len(foreign) > 0 {
			findings = append(findings, lintFinding{Code: "LINT026", Severity: lintWarn, Path: config.ConfigFileName,
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
