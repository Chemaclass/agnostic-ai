package codex

import (
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func (Adapter) TranslatePermission(list string, rule string) ([]string, bool) {
	pattern, ok := bashPermissionPrefix(rule)
	if !ok || list == "allow" && isExactBashRule(rule) {
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
	view := *cfg
	view.OnUnsupported = emit.OnUnsupportedSilent
	policies, _, err := resolveExecPolicies(settings, &view)
	if err != nil {
		return emit.CapabilityTranslation{}
	}
	present := slices.ContainsFunc(policies, func(policy config.CodexExecPolicy) bool {
		return policy.Decision == permissionDecision(list) && slices.Equal(policy.Pattern, pattern)
	})
	if !present {
		result := emit.CapabilityTranslation{}
		if list == "allow" && isExactBashRule(rule) {
			result.Widening = []string{"Exact allow is not written: Codex command prefixes also allow extra arguments"}
		}
		return result
	}
	return emit.CapabilityTranslation{Native: []string{"prefix_rule(" + strings.Join(pattern, " ") + ")"}, Supported: true}
}
