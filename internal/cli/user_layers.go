package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func userSourceHome(home string) string {
	if source := os.Getenv("AGNOSTIC_AI_HOME"); source != "" {
		return source
	}
	return filepath.Join(home, ".agnostic-ai")
}

// userSources are the spec folders of the user source and of its
// local layer.
var userSources = config.Sources{Agents: "agents", Skills: "skills", Rules: "rules", Hooks: "hooks", Settings: "settings", MCPs: "mcps"}

func userLayers(source string, names []string) ([]spec.Layer, error) {
	layers, err := resolveBuiltinLayers(names, "")
	if err != nil {
		return nil, err
	}
	return append(layers, []spec.Layer{
		{Name: layerNameUser, Root: source, Sources: userSources},
		{Name: layerNameUserLocal, Root: filepath.Join(source, "local"), Sources: userSources, Extends: true},
	}...), nil
}

// loadUserBundle loads the user specs. sync --scope user writes one
// instructions block that every tool reads, so a built-in rule with a
// scope or target condition stays out instead of stopping the sync.
func loadUserBundle(source string, names []string) (spec.Bundle, error) {
	layers, err := userLayers(source, names)
	if err != nil {
		return spec.Bundle{}, err
	}
	b, err := spec.LoadLayered(layers)
	if err != nil {
		return spec.Bundle{}, err
	}
	b.Rules = slices.DeleteFunc(slices.Clone(b.Rules), func(r spec.Entry) bool {
		return r.Layer == layerNameBuiltin && (r.Scope != "" || hasUserRuleCondition(r.Meta))
	})
	return b, nil
}

// userSourceRoot resolves the source root as sync --scope user does,
// reading HOME only when AGNOSTIC_AI_HOME is unset.
func userSourceRoot() (string, error) {
	if source := os.Getenv(envUserRoot); source != "" {
		return source, nil
	}
	home, err := userHomeDir()
	if err != nil {
		return "", err
	}
	return userSourceHome(home), nil
}

// checkScope is the set of specs list, lint, and validate read, with the
// targets to check them against.
type checkScope struct {
	userLevel bool
	// source is the user source root; empty for a project.
	source string
	// cfg is nil for the user scope, which has no config.
	cfg *config.Config
	// models are the tiers specs name: the project's, or the home configs'.
	models  map[string]config.ModelTier
	targets []string
	// hookTargets narrows targets to those that write hooks, which for
	// a project is every enabled target.
	hookTargets []string
	// support maps each kind to the targets that write it: adapter
	// output for a project, user-level surfaces for the user scope.
	support kindSupport
	bundle  spec.Bundle
	// project is the loaded project that migration plans read; nil for
	// the user scope.
	project *loadedProject
}

// migrationScope is the project scope plans read, sharing what the check
// scope loaded.
func (s checkScope) migrationScope() migrationScope {
	if s.userLevel {
		panic("migrationScope: spec migrations plan the project, not the user scope")
	}
	return migrationScope{root: ".", project: s.project}
}

// loadCheckScope loads the project in the working directory, or the
// user-level and user-local layers sync --scope user reads. The user scope
// checks against the targets a default sync --scope user writes, from the
// home config or else every supported one, and against what each writes
// at user level.
func loadCheckScope(userLevel bool) (checkScope, error) {
	scope, _, err := loadCheckScopeAllHooks(userLevel)
	return scope, err
}

// loadCheckScopeAllHooks is loadCheckScope that also returns every loaded
// hook, built-in hooks for tools the scope does not write included.
func loadCheckScopeAllHooks(userLevel bool) (checkScope, []spec.Entry, error) {
	scope, err := loadSpecScope(userLevel, nil)
	if err != nil {
		return scope, nil, err
	}
	if userLevel {
		targets, err := loadUserTargets(scope.source, io.Discard)
		if err != nil {
			return checkScope{}, nil, err
		}
		if targets == nil {
			targets = userTargetNames()
		}
		scope.targets, scope.hookTargets = targets, userHookTargets(targets)
	}
	all := scope.bundle.Hooks
	scope.bundle.Hooks = slices.DeleteFunc(slices.Clone(all), func(hook spec.Entry) bool {
		return hook.Layer == layerNameBuiltin && !slices.ContainsFunc(scope.hookTargets, hook.EmitsTo)
	})
	return scope, all, nil
}

// loadSpecScope is loadCheckScope without the user-level targets, for list,
// which reads only requires from the home config. With skipBroken set, a
// home config that does not parse warns there instead of stopping.
func loadSpecScope(userLevel bool, skipBroken io.Writer) (checkScope, error) {
	if !userLevel {
		cfg, b, err := loadProject(".")
		if err != nil {
			return checkScope{}, err
		}
		return loadedProjectCheckScope(cfg, b), nil
	}
	source, err := userSourceRoot()
	if err != nil {
		return checkScope{}, err
	}
	if err := requireUserVersion(source, skipBroken); err != nil {
		return checkScope{}, err
	}
	names, err := loadUserBuiltins(source, skipBroken)
	if err != nil {
		return checkScope{}, err
	}
	b, err := loadUserBundle(source, names)
	if err != nil {
		return checkScope{}, err
	}
	tiers, unloaded, err := loadUserModels(source, skipBroken)
	if err != nil {
		return checkScope{}, err
	}
	warn := skipBroken
	if warn == nil {
		warn = io.Discard
	}
	if err := applyUserTiers(&b, tiers, unloaded, warn); err != nil {
		return checkScope{}, err
	}
	return checkScope{userLevel: true, source: source, models: tiers, support: userKindSupport(), bundle: b}, nil
}

// loadedProjectCheckScope is the check scope of the project in the
// working directory, already loaded as cfg and b.
func loadedProjectCheckScope(cfg *config.Config, b spec.Bundle) checkScope {
	return checkScope{cfg: cfg, models: cfg.Models, targets: cfg.Targets, hookTargets: cfg.Targets, support: projectKindSupport(cfg), bundle: b, project: loadedMigrationScope(".", cfg, b).project}
}

func (s checkScope) emptyHint() string {
	if !s.userLevel {
		return emptySpecsHint
	}
	return fmt.Sprintf("no user specs found in %s. add files under its "+
		"{agents,skills,rules,hooks,settings,mcps}/ or local/ directories, or set "+
		"AGNOSTIC_AI_HOME to another root.", s.source)
}

// lintUserRules reports each rule sync --scope user would refuse, so
// lint and validate catch it before a sync does.
func lintUserRules(rules []spec.Entry, targets []string) []validationIssue {
	var out []validationIssue
	for _, rule := range rules {
		if !userRuleConditional(rule, targets) {
			continue
		}
		out = append(out, validationIssue{
			Path:    rule.Path,
			Message: fmt.Sprintf("user-level rule %q is scoped or conditional; user-level rules must apply unconditionally", rule.Name),
		})
	}
	return out
}

// lintUserRuleFindings reports the same rules as lint findings
// (LINT010, error).
func lintUserRuleFindings(rules []spec.Entry, targets []string) []lintFinding {
	var out []lintFinding
	for _, issue := range lintUserRules(rules, targets) {
		out = append(out, lintFinding{Code: "LINT010", Severity: lintError, Path: issue.Path, Message: issue.Message})
	}
	return out
}

func userInstructions(source string, tiers map[string]config.ModelTier, rules []spec.Entry) ([]byte, error) {
	layers, err := userInstructionLayers(source, tiers, rules)
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
type instructionRendering struct {
	Text   string
	Copied []header.CopiedRange
}

type instructionLayer struct {
	Rendering *instructionRendering
	Name      string
	Text      string
	Sources   []instructionSource
}

type instructionSource struct {
	Imports []adapters.ImportSpan
	Path    string
	Text    string
}

// userInstructionLayers returns the non-empty parts of the managed
// instructions block in order: AGNOSTIC_AI.md, the model tiers, the
// rules, then local/AGNOSTIC_AI.md. The tiers line carries no markers:
// sync owns the whole block.
func userInstructionLayers(source string, tiers map[string]config.ModelTier, rules []spec.Entry) ([]instructionLayer, error) {
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
		if line := adapters.ModelTiersLine(tiers); i == 0 && line != "" {
			layers = append(layers, instructionLayer{Name: "model tiers", Text: line})
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
