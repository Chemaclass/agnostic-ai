package codex

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func (Adapter) TranslatePermission(_ string, rule string) ([]string, bool) {
	pattern, ok := bashPermissionPrefix(rule)
	if !ok {
		return nil, false
	}
	return []string{"prefix_rule(" + strings.Join(pattern, " ") + ")"}, true
}

func (Adapter) TranslatePermissionContext(list, rule string, _ spec.Entry, settings []spec.Entry, cfg *config.Config) emit.CapabilityTranslation {
	if cfg == nil {
		return emit.CapabilityTranslation{}
	}
	if source := nativeExecPoliciesSource(cfg); source != "" {
		result := emit.CapabilityTranslation{Supported: true, Override: source}
		policies, err := loadExecPolicies(cfg)
		if err != nil {
			return result
		}
		pattern, ok := bashPermissionPrefix(rule)
		if !ok {
			return result
		}
		if decision := prefixDecision(policies, pattern); decision != "" {
			result.Native = []string{"prefix_rule(" + strings.Join(pattern, " ") + "): " + decision}
		}
		return result
	}
	if !cfg.Outputs[target].ExecPoliciesFromPermissions {
		return emit.CapabilityTranslation{}
	}
	pattern, ok := bashPermissionPrefix(rule)
	if !ok {
		return emit.CapabilityTranslation{}
	}
	var policies, wildcards []config.CodexExecPolicy
	for _, candidate := range permissionRules(settings, cfg) {
		command, valid := bashPermissionPrefix(candidate.rule)
		if !valid {
			continue
		}
		policy := config.CodexExecPolicy{Pattern: command, Decision: permissionDecision(candidate.list)}
		policies = append(policies, policy)
		if !isExactBashRule(candidate.rule) {
			wildcards = append(wildcards, policy)
		}
	}
	result := emit.CapabilityTranslation{Native: []string{"prefix_rule(" + strings.Join(pattern, " ") + ")"}, Supported: true}
	if list == "allow" && isExactBashRule(rule) && prefixDecision(policies, pattern) == "allow" && prefixDecision(wildcards, pattern) != "allow" {
		result.Widening = []string{"Codex exec policies match command prefixes, so extra arguments are also allowed"}
	}
	return result
}
