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

// loadGlobalBundle loads the layers sync --global reads, with no project
// config, so it works from any directory.
func loadGlobalBundle() (spec.Bundle, error) {
	home, err := globalUserHome()
	if err != nil {
		return spec.Bundle{}, err
	}
	return spec.LoadLayered(globalLayers(globalSourceHome(home)))
}

// loadCheckScope loads the specs lint and validate check, with the
// targets to check them against. The global scope has no config, so it
// checks against every target a default sync --global writes; cfg is
// nil there.
func loadCheckScope(global bool) (*config.Config, []string, spec.Bundle, error) {
	if global {
		b, err := loadGlobalBundle()
		return nil, globalTargetNames(), b, err
	}
	cfg, b, err := loadProject(".")
	if err != nil {
		return nil, nil, spec.Bundle{}, err
	}
	return cfg, cfg.Targets, b, nil
}

func emptyHint(global bool) string {
	if global {
		return emptyGlobalSpecsHint
	}
	return emptySpecsHint
}

// emptyGlobalSpecsHint is the global counterpart of emptySpecsHint.
const emptyGlobalSpecsHint = "no global specs found. add files under " +
	"$AGNOSTIC_AI_HOME/{agents,skills,rules,hooks}/ or its local/ layer " +
	"(default home: ~/.agnostic-ai)."

// lintGlobalRules reports each rule sync --global would refuse, so
// validate --global catches it before a sync does.
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
