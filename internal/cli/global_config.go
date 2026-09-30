package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// globalConfigPaths lists the home configs in load order: the source
// root's agnostic-ai.yaml, then local/agnostic-ai.yaml, whose keys
// replace the shared ones.
func globalConfigPaths(source string) []string {
	return []string{filepath.Join(source, config.ConfigFileName), filepath.Join(source, "local", config.ConfigFileName)}
}

// readGlobalConfig parses one home config by top-level key. A missing
// file reads as empty.
func readGlobalConfig(path string) (map[string]yaml.Node, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var doc map[string]yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, errs.Coded(errs.CodeConfigDecode, "parse %s: %w", path, err)
	}
	return doc, nil
}

// loadGlobalTargets reads the targets the home configs select. It
// returns nil when neither file sets targets. Global mode reads only
// targets, requires, lint, on-unsupported, and models, so each other key warns.
func loadGlobalTargets(source string, warn io.Writer) ([]string, error) {
	var targets []string
	for _, path := range globalConfigPaths(source) {
		doc, err := readGlobalConfig(path)
		if err != nil {
			return nil, err
		}
		var ignored []string
		for key := range doc {
			// Every agnostic-ai.yaml carries version, so it is not a surprise.
			if key != "targets" && key != "requires" && key != "lint" && key != "on-unsupported" && key != "models" && key != "version" {
				ignored = append(ignored, key)
			}
		}
		if len(ignored) > 0 {
			slices.Sort(ignored)
			if _, err := fmt.Fprintf(warn, "warning: %s: global mode reads only targets, requires, lint, on-unsupported, and models; ignoring %s\n", path, strings.Join(ignored, ", ")); err != nil {
				return nil, fmt.Errorf("write global config warning: %w", err)
			}
		}
		node, ok := doc["targets"]
		if !ok {
			continue
		}
		var list []string
		if err := node.Decode(&list); err != nil {
			return nil, errs.Coded(errs.CodeConfigDecode, "parse %s: targets: %w", path, err)
		}
		if len(list) == 0 {
			return nil, fmt.Errorf("%s: targets is empty; name at least one target or remove the key", path)
		}
		// A project-shaped config lists targets with no user-level
		// surface, such as aider; skipping them keeps it usable here.
		var kept, skipped []string
		for _, target := range list {
			switch {
			case slices.Contains(kept, target) || slices.Contains(skipped, target):
				// A repeated name counts once.
			case slices.Contains(globalTargetNames(), target):
				kept = append(kept, target)
			case slices.Contains(adapters.Names(), target):
				skipped = append(skipped, target)
			default:
				return nil, unsupportedGlobalTarget(path, target)
			}
		}
		if len(skipped) > 0 {
			if _, err := fmt.Fprintf(warn, "warning: %s: sync --global cannot write %s; skipping them\n", path, strings.Join(skipped, ", ")); err != nil {
				return nil, fmt.Errorf("write global config warning: %w", err)
			}
		}
		if len(kept) == 0 {
			return nil, fmt.Errorf("%s: no listed target has a user-level surface; supported: %s", path, strings.Join(globalTargetNames(), ", "))
		}
		targets = kept
	}
	return targets, nil
}

// loadGlobalModels reads the model tiers the home configs name. A tier in
// local/agnostic-ai.yaml replaces the same-name tier in the shared file.
func loadGlobalModels(source string) (map[string]config.ModelTier, error) {
	tiers := map[string]config.ModelTier{}
	for _, path := range globalConfigPaths(source) {
		doc, err := readGlobalConfig(path)
		if err != nil {
			return nil, err
		}
		node, ok := doc["models"]
		if !ok {
			continue
		}
		var layer map[string]config.ModelTier
		if err := node.Decode(&layer); err != nil {
			return nil, errs.Coded(errs.CodeConfigDecode, "parse %s: models: %w", path, err)
		}
		if err := config.ValidateModels(layer, path); err != nil {
			return nil, err
		}
		for name, tier := range layer {
			tiers[name] = tier
		}
	}
	return tiers, nil
}

// loadGlobalOnUnsupported reads the on-unsupported policy the home
// configs set. A key in local/agnostic-ai.yaml replaces the same key in
// the shared file. It returns "" when neither sets it, which reads as warn.
func loadGlobalOnUnsupported(source string) (string, error) {
	var mode string
	for _, path := range globalConfigPaths(source) {
		doc, err := readGlobalConfig(path)
		if err != nil {
			return "", err
		}
		node, ok := doc["on-unsupported"]
		if !ok {
			continue
		}
		if err := node.Decode(&mode); err != nil {
			return "", errs.Coded(errs.CodeConfigDecode, "parse %s: on-unsupported: %w", path, err)
		}
	}
	return mode, nil
}

// loadGlobalLint reads the lint budgets the home configs set. A key in
// local/agnostic-ai.yaml replaces the same key in the shared file, as
// the project's agnostic-ai.local.yaml does.
func loadGlobalLint(source string) (config.LintConfig, error) {
	var lint config.LintConfig
	var sources []string
	for _, path := range globalConfigPaths(source) {
		doc, err := readGlobalConfig(path)
		if err != nil {
			return config.LintConfig{}, err
		}
		node, ok := doc["lint"]
		if !ok {
			continue
		}
		if err := node.Decode(&lint); err != nil {
			return config.LintConfig{}, errs.Coded(errs.CodeConfigDecode, "parse %s: lint: %w", path, err)
		}
		sources = append(sources, path)
	}
	if err := lint.Validate(strings.Join(sources, " + ")); err != nil {
		return config.LintConfig{}, err
	}
	return lint, nil
}

// Remedies refuseGlobalHome offers besides pointing AGNOSTIC_AI_HOME away.
const (
	globalHomeSyncRemedy  = "run `agnostic-ai sync --global`"
	globalHomeSpecsRemedy = "edit its specs by hand and run `agnostic-ai sync --global`"
	globalHomeHookRemedy  = "run `agnostic-ai install-hook --global`"
)

// refuseGlobalHome stops a project command in the global source root or
// any directory inside it. Read as a project, the home config would send
// native files and project scaffolding into the home or its local/ layer.
// Walking the resolved path and comparing with os.SameFile sees through
// symlinks and path case.
func refuseGlobalHome(dir, remedy string) error {
	source, err := globalSourceRoot()
	if err != nil {
		return nil
	}
	root, err := os.Stat(source)
	if err != nil {
		return nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	current, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil
	}
	var inside []string
	for {
		if info, err := os.Stat(current); err == nil && os.SameFile(info, root) {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
		inside = append([]string{filepath.Base(current)}, inside...)
		current = parent
	}
	// A home kept in HOME itself would otherwise refuse every project
	// under HOME, so there only the root is guarded.
	if len(inside) > 0 && isUserHome(root) {
		return nil
	}
	where := source + " is the global home"
	if len(inside) > 0 {
		where = filepath.Join(append([]string{source}, inside...)...) + " is inside the global home " + source
	}
	if os.Getenv(envUserGlobalRoot) != "" {
		return fmt.Errorf("%s (%s); %s, or unset %s if this is a project", where, envUserGlobalRoot, remedy, envUserGlobalRoot)
	}
	return fmt.Errorf("%s (%s is unset); %s, or set %s to another root if this is a project", where, envUserGlobalRoot, remedy, envUserGlobalRoot)
}

func isUserHome(dir os.FileInfo) bool {
	home, err := globalUserHome()
	if err != nil {
		return false
	}
	info, err := os.Stat(home)
	return err == nil && os.SameFile(info, dir)
}

// unsupportedGlobalTarget rejects a target sync --global cannot write,
// suggesting the closest supported name. where leads the message.
func unsupportedGlobalTarget(where, target string) error {
	if _, ok := globalTargets[target]; ok {
		return nil
	}
	if s := adapters.SuggestName(target, globalTargetNames()); s != "" {
		return fmt.Errorf("%s: unsupported target %q (did you mean %s?)", where, target, s)
	}
	return fmt.Errorf("%s: unsupported target %q (supported: %s)", where, target, strings.Join(globalTargetNames(), ", "))
}
