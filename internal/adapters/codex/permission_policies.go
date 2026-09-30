package codex

import (
	"fmt"
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
	// Claude reads `:*` and a trailing ` *` as the same wildcard; only one of them makes a prefix.
	command, legacy := strings.CutSuffix(command, ":*")
	if !legacy {
		command = strings.TrimSuffix(command, " *")
	}
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

func isBashRule(rule string) bool {
	tool, _, _ := strings.Cut(rule, "(")
	return tool == "Bash"
}

func specPermissions(entry spec.Entry, list string) []string {
	perms, _ := entry.Meta["permissions"].(map[string]any)
	if values, ok := perms[list].([]string); ok {
		return values
	}
	return emit.StringSlice(perms[list])
}

func specsWithOtherToolPermissions(settings []spec.Entry) int {
	otherTool := func(rule string) bool { return rule != "" && !isBashRule(rule) }
	n := 0
	for _, entry := range settings {
		if slices.ContainsFunc(specPermissions(entry, "allow"), otherTool) ||
			slices.ContainsFunc(specPermissions(entry, "deny"), otherTool) ||
			slices.ContainsFunc(specPermissions(entry, "ask"), otherTool) {
			n++
		}
	}
	return n
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
			path := entry.Path
			if path == "" {
				path = "settings " + entry.Name
			}
			add(path, specPermissions(entry, list))
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

func nativeExecPoliciesSource(cfg *config.Config) string {
	var sources []string
	if cfg.Outputs[target].ExecPolicies != nil {
		sources = append(sources, "outputs.codex.exec-policies")
	}
	if file := execPoliciesSourceFile(cfg); file != "" {
		sources = append(sources, file)
	}
	return strings.Join(sources, " and ")
}

func resolveExecPolicies(settings []spec.Entry, cfg *config.Config) ([]config.CodexExecPolicy, bool, error) {
	policies, err := loadExecPolicies(cfg)
	if err != nil {
		return nil, false, err
	}
	if !cfg.Outputs[target].ExecPoliciesFromPermissions {
		return policies, false, nil
	}
	if source := nativeExecPoliciesSource(cfg); source != "" {
		emit.NoteProject(fmt.Sprintf("codex: exec policies come from %s, so outputs.codex.exec-policies-from-permissions has no effect", source))
		return policies, false, nil
	}
	for _, rule := range permissionRules(settings, cfg) {
		if !isBashRule(rule.rule) {
			continue
		}
		pattern, ok := bashPermissionPrefix(rule.rule)
		if !ok {
			unsupported := fmt.Errorf("%s: permissions.%s rule %s cannot translate to a Codex command prefix; use outputs.codex.exec-policies", rule.path, rule.list, rule.rule)
			switch cfg.OnUnsupported {
			case emit.OnUnsupportedError:
				return nil, true, unsupported
			case emit.OnUnsupportedSilent:
			default:
				emit.NoteFieldNoOp(target, spec.KindSettings, "permissions."+rule.list, 1, unsupported.Error())
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
	if nativeExecPoliciesSource(cfg) == "" {
		return nil, nil
	}
	rules := permissionRules(settings, cfg)
	var portable []config.CodexExecPolicy
	for _, rule := range rules {
		if pattern, ok := bashPermissionPrefix(rule.rule); ok {
			portable = append(portable, config.CodexExecPolicy{Pattern: pattern, Decision: permissionDecision(rule.list)})
		}
	}
	if len(portable) == 0 {
		return nil, nil
	}
	policies, err := loadExecPolicies(cfg)
	if err != nil {
		return nil, err
	}
	inline, file := len(cfg.Outputs[target].ExecPolicies), execPoliciesSourceFile(cfg)
	for i, policy := range policies {
		source, index := config.ConfigFileName, i
		if i >= inline {
			source, index = file, i-inline
		}
		if err := validateExecPolicy(policy, index); err != nil {
			return nil, fmt.Errorf("%s: %w", source, err)
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
