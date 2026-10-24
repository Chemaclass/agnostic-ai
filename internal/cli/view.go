package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// authoritySource is what serve reads: a project, entered
// so that paths read as sync reads them, or the user-level
// configuration.
type authoritySource struct {
	root   string
	global bool
	// close returns to the directory the command started in.
	close func()
}

// openAuthoritySource enters dir for a project, since sync and the
// generated files it compares name paths from the project root.
func openAuthoritySource(dir string, global bool) (*authoritySource, error) {
	if global {
		root, err := globalSourceRoot()
		if err != nil {
			return nil, err
		}
		return &authoritySource{root: root, global: true, close: func() {}}, nil
	}
	src := &authoritySource{root: ".", close: func() {}}
	if dir == "" || filepath.Clean(dir) == "." {
		return src, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve working directory: %w", err)
	}
	if err := os.Chdir(dir); err != nil {
		return nil, fmt.Errorf("enter %s: %w", dir, err)
	}
	src.close = func() { _ = os.Chdir(cwd) }
	return src, nil
}

// load reads the configuration and specs. The global configuration has
// no project config: its targets come from the home config, or are
// every target sync --global writes.
func (s *authoritySource) load() (*config.Config, spec.Bundle, error) {
	if !s.global {
		return loadProject(s.root)
	}
	scope, err := loadCheckScope(true)
	if err != nil {
		return nil, spec.Bundle{}, err
	}
	return &config.Config{Targets: scope.targets}, scope.bundle, nil
}

// configFiles lists the config files the source reads, relative to its
// root.
func (s *authoritySource) configFiles() []string {
	var out []string
	if s.global {
		for _, path := range globalConfigPaths(s.root) {
			if _, err := os.Stat(path); err == nil {
				if rel, err := filepath.Rel(s.root, path); err == nil {
					out = append(out, filepath.ToSlash(rel))
				}
			}
		}
		return out
	}
	if path, _, err := config.ResolveConfigPath(s.root); err == nil {
		out = append(out, filepath.Base(path))
	}
	if _, err := os.Stat(filepath.Join(s.root, config.LocalOverrideFileName)); err == nil {
		out = append(out, config.LocalOverrideFileName)
	}
	return out
}
