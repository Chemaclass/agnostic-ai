package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func globalSourceHome(home string) string {
	if source := os.Getenv("AGNOSTIC_AI_HOME"); source != "" {
		return source
	}
	return filepath.Join(home, ".agnostic-ai")
}

func globalLayers(source string) []spec.Layer {
	sources := config.Sources{Agents: "agents", Skills: "skills", Rules: "rules", Hooks: "hooks"}
	return []spec.Layer{
		{Name: "global", Root: source, Sources: sources},
		{Name: "global-local", Root: filepath.Join(source, "local"), Sources: sources, Extends: true},
	}
}

// globalSourceRoot resolves the source root as sync --global does,
// reading HOME only when AGNOSTIC_AI_HOME is unset.
func globalSourceRoot() (string, error) {
	if source := os.Getenv(envUserGlobalRoot); source != "" {
		return source, nil
	}
	home, err := globalUserHome()
	if err != nil {
		return "", err
	}
	return globalSourceHome(home), nil
}

// checkScope is the set of specs list, lint, and validate read, with the
// targets to check them against.
type checkScope struct {
	global bool
	// source is the global source root; empty for a project.
	source string
	// cfg is nil for the global scope, which has no config.
	cfg     *config.Config
	targets []string
	// hookTargets narrows targets to those that write hooks, which for
	// a project is every enabled target.
	hookTargets []string
	bundle      spec.Bundle
}

// loadCheckScope loads the project in the working directory, or the
// global and global-local layers sync --global reads. The global scope
// checks against every target a default sync --global writes, and hook
// events only against the targets it writes hooks for.
func loadCheckScope(global bool) (checkScope, error) {
	if !global {
		cfg, b, err := loadProject(".")
		if err != nil {
			return checkScope{}, err
		}
		return checkScope{cfg: cfg, targets: cfg.Targets, hookTargets: cfg.Targets, bundle: b}, nil
	}
	source, err := globalSourceRoot()
	if err != nil {
		return checkScope{}, err
	}
	b, err := spec.LoadLayered(globalLayers(source))
	if err != nil {
		return checkScope{}, err
	}
	return checkScope{global: true, source: source, targets: globalTargetNames(), hookTargets: globalHookTargetNames(), bundle: b}, nil
}

func (s checkScope) emptyHint() string {
	if !s.global {
		return emptySpecsHint
	}
	return fmt.Sprintf("no global specs found in %s. add files under its "+
		"{agents,skills,rules,hooks}/ or local/ directories, or set "+
		"AGNOSTIC_AI_HOME to another root.", s.source)
}

// lintGlobalRules reports each rule sync --global would refuse, so
// lint and validate catch it before a sync does.
func lintGlobalRules(rules []spec.Entry) []validationIssue {
	var out []validationIssue
	for _, rule := range rules {
		if rule.Scope == "" && !hasGlobalRuleCondition(rule.Meta) {
			continue
		}
		out = append(out, validationIssue{
			Path:    rule.Path,
			Message: fmt.Sprintf("global rule %q is scoped or conditional; global rules must apply unconditionally", rule.Name),
		})
	}
	return out
}

// lintGlobalRuleFindings reports the same rules as lint findings
// (LINT010, error).
func lintGlobalRuleFindings(rules []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, issue := range lintGlobalRules(rules) {
		out = append(out, lintFinding{Code: "LINT010", Severity: lintError, Path: issue.Path, Message: issue.Message})
	}
	return out
}

func globalInstructions(source string, rules []spec.Entry) ([]byte, error) {
	var parts []string
	for i, root := range []string{source, filepath.Join(source, "local")} {
		path := filepath.Join(root, "AGNOSTIC_AI.md")
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		if body := strings.TrimSpace(string(data)); body != "" {
			parts = append(parts, body)
		}
		if i == 0 {
			for _, rule := range rules {
				parts = append(parts, "## "+rule.Name+"\n\n"+strings.TrimSpace(rule.Body))
			}
		}
	}
	return []byte(strings.Join(parts, "\n\n")), nil
}
