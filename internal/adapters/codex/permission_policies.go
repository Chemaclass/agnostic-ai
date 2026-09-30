package codex

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type permissionRule struct {
	path, list, rule string
}

var plainCommandWord = regexp.MustCompile(`^[a-zA-Z0-9_./:@%+,=-]+$`)

func bashPermissionPrefix(rule string) ([]string, bool) {
	tool, command, ok := spec.SplitPermissionRule(rule)
	if !ok || tool != "Bash" || strings.ContainsAny(command, "\r\n") {
		return nil, false
	}
	command = strings.TrimSuffix(command, ":*")
	words := strings.FieldsFunc(command, func(r rune) bool { return r == ' ' || r == '\t' })
	if len(words) == 0 || strings.Contains(words[0], "=") {
		return nil, false
	}
	switch words[0] {
	case "if", "then", "else", "elif", "fi", "for", "while", "until", "do", "done", "case", "esac", "in", "function", "select", "time", "coproc", "!", "[[", "]]", "{", "}":
		return nil, false
	}
	for _, word := range words {
		if !plainCommandWord.MatchString(word) {
			return nil, false
		}
	}
	return words, true
}

func permissionRules(settings []spec.Entry, cfg *config.Config) []permissionRule {
	var rules []permissionRule
	for _, list := range []string{"allow", "deny", "ask"} {
		seen := map[string]bool{}
		add := func(path string, values []string) {
			for _, rule := range values {
				if rule == "" || seen[rule] {
					continue
				}
				seen[rule] = true
				rules = append(rules, permissionRule{path: path, list: list, rule: rule})
			}
		}
		for _, entry := range settings {
			perms, _ := entry.Meta["permissions"].(map[string]any)
			values, ok := perms[list].([]string)
			if !ok {
				values = emit.StringSlice(perms[list])
			}
			path := entry.Path
			if path == "" {
				path = "settings " + entry.Name
			}
			add(path, values)
		}
		if settings := cfg.Outputs["claude"].Settings; settings != nil && settings.Permissions != nil {
			permissions := settings.Permissions
			values := map[string][]string{"allow": permissions.Allow, "deny": permissions.Deny, "ask": permissions.Ask}
			add(config.ConfigFileName, values[list])
		}
	}
	return rules
}

func permissionDecision(list string) string {
	switch list {
	case "allow":
		return "allow"
	case "deny":
		return "forbidden"
	default:
		return "prompt"
	}
}

func nativeExecPoliciesConfigured(cfg *config.Config) bool {
	out := cfg.Outputs[target]
	if len(out.ExecPolicies) > 0 || out.ExecPoliciesFile != "" {
		return true
	}
	_, err := os.Stat(execPoliciesOverlayPath)
	return err == nil
}

func resolveExecPolicies(settings []spec.Entry, cfg *config.Config) ([]config.CodexExecPolicy, bool, error) {
	policies, err := loadExecPolicies(cfg)
	if err != nil {
		return nil, false, err
	}
	if !cfg.Outputs[target].ExecPoliciesFromPermissions || nativeExecPoliciesConfigured(cfg) {
		return policies, false, nil
	}
	for _, rule := range permissionRules(settings, cfg) {
		pattern, ok := bashPermissionPrefix(rule.rule)
		if !ok {
			reason := fmt.Sprintf("%s: permissions.%s rule %s cannot translate to a Codex command prefix; use outputs.codex.exec-policies", rule.path, rule.list, rule.rule)
			switch cfg.OnUnsupported {
			case emit.OnUnsupportedError:
				return nil, true, fmt.Errorf("codex: %s", reason)
			case emit.OnUnsupportedSilent:
			default:
				emit.NoteFieldNoOp(target, spec.KindSettings, "permissions."+rule.list, 1, reason)
			}
			continue
		}
		policies = append(policies, config.CodexExecPolicy{Pattern: pattern, Decision: permissionDecision(rule.list)})
	}
	return policies, true, nil
}

// PermissionPolicyMismatch names a portable rule explicit native policies do not cover with the same decision.
type PermissionPolicyMismatch struct {
	Path, List, Rule, Decision string
}

// PermissionPolicyDrift checks declared command prefixes, without reading user policy files.
func PermissionPolicyDrift(settings []spec.Entry, cfg *config.Config) ([]PermissionPolicyMismatch, error) {
	if !nativeExecPoliciesConfigured(cfg) {
		return nil, nil
	}
	policies, err := loadExecPolicies(cfg)
	if err != nil {
		return nil, err
	}
	for i, policy := range policies {
		if err := validateExecPolicy(policy, i); err != nil {
			return nil, err
		}
	}
	rules := permissionRules(settings, cfg)
	var portable []config.CodexExecPolicy
	for _, rule := range rules {
		if pattern, ok := bashPermissionPrefix(rule.rule); ok {
			portable = append(portable, config.CodexExecPolicy{Pattern: pattern, Decision: permissionDecision(rule.list)})
		}
	}
	var mismatches []PermissionPolicyMismatch
	for _, rule := range rules {
		if rule.list == "ask" {
			continue
		}
		pattern, ok := bashPermissionPrefix(rule.rule)
		if !ok {
			continue
		}
		decision := prefixDecision(policies, pattern)
		mismatch := decision != prefixDecision(portable, pattern)
		if !mismatch && rule.list == "allow" {
			for _, policy := range policies {
				if policy.Decision == "allow" || len(pattern) > len(policy.Pattern) || !slices.Equal(pattern, policy.Pattern[:len(pattern)]) {
					continue
				}
				// Portable deny and ask exclusions can account for a native restriction.
				actual := prefixDecision(policies, policy.Pattern)
				if actual != prefixDecision(portable, policy.Pattern) {
					decision, mismatch = actual, true
					break
				}
			}
		}
		if mismatch {
			mismatches = append(mismatches, PermissionPolicyMismatch{Path: rule.path, List: rule.list, Rule: rule.rule, Decision: decision})
		}
	}
	return mismatches, nil
}

func prefixDecision(policies []config.CodexExecPolicy, pattern []string) string {
	decision := ""
	for _, policy := range policies {
		if len(policy.Pattern) <= len(pattern) && slices.Equal(policy.Pattern, pattern[:len(policy.Pattern)]) && policySeverity(policy.Decision) > policySeverity(decision) {
			decision = policy.Decision
		}
	}
	return decision
}

func policySeverity(decision string) int {
	switch decision {
	case "allow":
		return 1
	case "prompt":
		return 2
	case "forbidden":
		return 3
	default:
		return 0
	}
}
