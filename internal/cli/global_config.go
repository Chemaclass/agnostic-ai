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
		for _, target := range list {
			if err := unsupportedGlobalTarget(path, target); err != nil {
				return nil, err
			}
		}
		targets = list
	}
	return targets, nil
}

// refuseGlobalHome stops a project sync in the global source root, whose
// agnostic-ai.yaml is the home config: read as a project, it would write
// native files into the home. os.SameFile sees through symlinks and case.
func refuseGlobalHome(dir string) error {
	source, err := globalSourceRoot()
	if err != nil {
		return nil
	}
	here, err := os.Stat(dir)
	if err != nil {
		return nil
	}
	root, err := os.Stat(source)
	if err != nil || !os.SameFile(here, root) {
		return nil
	}
	if os.Getenv(envUserGlobalRoot) != "" {
		return fmt.Errorf("%s is the global home (%s); run `agnostic-ai sync --global`, or unset %s if this is a project", source, envUserGlobalRoot, envUserGlobalRoot)
	}
	return fmt.Errorf("%s is the global home (default %s); run `agnostic-ai sync --global`, or set %s to another root if this is a project", source, envUserGlobalRoot, envUserGlobalRoot)
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
