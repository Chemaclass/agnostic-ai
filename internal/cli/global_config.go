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

// loadGlobalTargets reads the targets a home config selects: the source
// root's agnostic-ai.yaml, then local/agnostic-ai.yaml, whose list
// replaces the shared one. It returns nil when neither file sets
// targets. Global mode reads no other key, so each one it ignores warns.
func loadGlobalTargets(source string, warn io.Writer) ([]string, error) {
	var targets []string
	for _, path := range []string{filepath.Join(source, config.ConfigFileName), filepath.Join(source, "local", config.ConfigFileName)} {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		var doc map[string]yaml.Node
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, errs.Coded(errs.CodeConfigDecode, "parse %s: %w", path, err)
		}
		var ignored []string
		for key := range doc {
			// Every agnostic-ai.yaml carries version, so it is not a surprise.
			if key != "targets" && key != "version" {
				ignored = append(ignored, key)
			}
		}
		if len(ignored) > 0 {
			slices.Sort(ignored)
			if _, err := fmt.Fprintf(warn, "warning: %s: global mode reads only targets; ignoring %s\n", path, strings.Join(ignored, ", ")); err != nil {
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

// Remedies refuseGlobalHome offers besides pointing AGNOSTIC_AI_HOME away.
const (
	globalHomeSyncRemedy  = "run `agnostic-ai sync --global`"
	globalHomeSpecsRemedy = "edit its specs by hand and run `agnostic-ai sync --global`"
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
