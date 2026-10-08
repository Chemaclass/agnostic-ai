package cli

import (
	"fmt"
	"io"
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

func globalLayers(source string, names []string) ([]spec.Layer, error) {
	layers, err := resolveBuiltinLayers(names, "")
	if err != nil {
		return nil, err
	}
	sources := config.Sources{Agents: "agents", Skills: "skills", Rules: "rules", Hooks: "hooks", Settings: "settings", MCPs: "mcps"}
	return append(layers, []spec.Layer{
		{Name: "global", Root: source, Sources: sources},
		{Name: "global-local", Root: filepath.Join(source, "local"), Sources: sources, Extends: true},
	}...), nil
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
	cfg *config.Config
	// models are the tiers specs name: the project's, or the home configs'.
	models  map[string]config.ModelTier
	targets []string
	// hookTargets narrows targets to those that write hooks, which for
	// a project is every enabled target.
	hookTargets []string
	// support maps each kind to the targets that write it: adapter
	// output for a project, user-level surfaces for the global scope.
	support kindSupport
	bundle  spec.Bundle
	// project is the loaded project that migration plans read; nil for
	// the global scope.
	project *loadedProject
}

// migrationScope is the project scope plans read, sharing what the check
// scope loaded.
func (s checkScope) migrationScope() migrationScope {
	if s.global {
		panic("migrationScope: spec migrations plan the project, not the global scope")
	}
	return migrationScope{root: ".", project: s.project}
}

// loadCheckScope loads the project in the working directory, or the
// global and global-local layers sync --global reads. The global scope
// checks against the targets a default sync --global writes, from the
// home config or else every supported one, and against what each writes
// at user level.
func loadCheckScope(global bool) (checkScope, error) {
	scope, err := loadSpecScope(global, nil)
	if err != nil || !global {
		return scope, err
	}
	targets, err := loadGlobalTargets(scope.source, io.Discard)
	if err != nil {
		return checkScope{}, err
	}
	if targets == nil {
		targets = globalTargetNames()
	}
	scope.targets, scope.hookTargets = targets, globalHookTargets(targets)
	return scope, nil
}

// loadSpecScope is loadCheckScope without the global targets, for list,
// which reads only requires from the home config. With skipBroken set, a
// home config that does not parse warns there instead of stopping.
func loadSpecScope(global bool, skipBroken io.Writer) (checkScope, error) {
	if !global {
		cfg, b, err := loadProject(".")
		if err != nil {
			return checkScope{}, err
		}
		return checkScope{cfg: cfg, models: cfg.Models, targets: cfg.Targets, hookTargets: cfg.Targets, support: projectKindSupport(cfg), bundle: b, project: loadedMigrationScope(".", cfg, b).project}, nil
	}
	source, err := globalSourceRoot()
	if err != nil {
		return checkScope{}, err
	}
	if err := requireGlobalVersion(source, skipBroken); err != nil {
		return checkScope{}, err
	}
	names, err := loadGlobalBuiltins(source, skipBroken)
	if err != nil {
		return checkScope{}, err
	}
	layers, err := globalLayers(source, names)
	if err != nil {
		return checkScope{}, err
	}
	b, err := spec.LoadLayered(layers)
	if err != nil {
		return checkScope{}, err
	}
	tiers, unloaded, err := loadGlobalModels(source, skipBroken)
	if err != nil {
		return checkScope{}, err
	}
	warn := skipBroken
	if warn == nil {
		warn = io.Discard
	}
	if err := applyGlobalTiers(&b, tiers, unloaded, warn); err != nil {
		return checkScope{}, err
	}
	return checkScope{global: true, source: source, models: tiers, support: globalKindSupport(), bundle: b}, nil
}

func (s checkScope) emptyHint() string {
	if !s.global {
		return emptySpecsHint
	}
	return fmt.Sprintf("no global specs found in %s. add files under its "+
		"{agents,skills,rules,hooks,settings,mcps}/ or local/ directories, or set "+
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
	layers, err := globalInstructionLayers(source, rules)
	if err != nil {
		return nil, err
	}
	parts := make([]string, 0, len(layers))
	for _, l := range layers {
		parts = append(parts, l.Text)
	}
	return []byte(strings.Join(parts, "\n\n")), nil
}

// instructionLayer is the text one source adds to always-loaded
// instructions, named as lint reports it.
type instructionLayer struct {
	Name string
	Text string
}

// globalInstructionLayers returns the non-empty parts of the managed
// instructions block in order: AGNOSTIC_AI.md, the rules, then
// local/AGNOSTIC_AI.md.
func globalInstructionLayers(source string, rules []spec.Entry) ([]instructionLayer, error) {
	var layers []instructionLayer
	for i, root := range []string{source, filepath.Join(source, "local")} {
		path := filepath.Join(root, "AGNOSTIC_AI.md")
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		if body := strings.TrimSpace(string(data)); body != "" {
			name := "AGNOSTIC_AI.md"
			if i == 1 {
				name = "local/AGNOSTIC_AI.md"
			}
			layers = append(layers, instructionLayer{Name: name, Text: body})
		}
		if i == 0 && len(rules) > 0 {
			sections := make([]string, 0, len(rules))
			for _, rule := range rules {
				sections = append(sections, "## "+rule.Name+"\n\n"+strings.TrimSpace(rule.Body))
			}
			layers = append(layers, instructionLayer{Name: "rules", Text: strings.Join(sections, "\n\n")})
		}
	}
	return layers, nil
}
