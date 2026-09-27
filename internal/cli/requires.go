package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// loadRequiredProject is loadProject for the commands that honor
// `requires`. The check runs before specs load, so a spec only a newer
// release reads cannot fail ahead of the upgrade hint.
func loadRequiredProject(cmd *cobra.Command, root string) (*config.Config, spec.Bundle, error) {
	cfg, sources, err := config.LoadWithSources(root)
	if err != nil {
		return nil, spec.Bundle{}, err
	}
	if err := requireVersion(cmd, strings.Join(sources, " + "), cfg.Requires); err != nil {
		return nil, spec.Bundle{}, err
	}
	b, err := loadProjectSpecs(root, cfg, sources)
	if err != nil {
		return nil, spec.Bundle{}, err
	}
	return cfg, b, nil
}

// requireGlobalVersion is requireVersion for the home config, where a
// requires in local/agnostic-ai.yaml replaces the shared one.
func requireGlobalVersion(cmd *cobra.Command, source string) error {
	var requires, path string
	for _, p := range globalConfigPaths(source) {
		doc, err := readGlobalConfig(p)
		if err != nil {
			return err
		}
		node, ok := doc["requires"]
		if !ok {
			continue
		}
		if err := node.Decode(&requires); err != nil {
			return errs.Coded(errs.CodeConfigDecode, "%s: requires: %w", p, err)
		}
		path = p
	}
	return requireVersion(cmd, path, requires)
}

// requireVersion stops the command when the running binary is older
// than requires, which source sets. A build that is not a release has
// no place in the order, so it warns and runs: contributors and CI
// build from source.
func requireVersion(cmd *cobra.Command, source, requires string) error {
	if requires == "" {
		return nil
	}
	req, err := config.ParseRequirement(requires)
	if err != nil {
		return errs.Coded(errs.CodeConfigDecode, "%s: requires: %w", source, err)
	}
	running := cmd.Root().Version
	allowed, release := req.Allows(running)
	if !release {
		if verbosity < levelDefault {
			return nil
		}
		if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s: requires %s, but %s is not a release build; not checked\n", source, req, running); err != nil {
			return fmt.Errorf("write requires warning: %w", err)
		}
		return nil
	}
	if !allowed {
		return errs.Coded(errs.CodeRequiresUnmet, "%s requires agnostic-ai %s, but %s is installed; run `agnostic-ai upgrade`", source, req, running)
	}
	return nil
}
