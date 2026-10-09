package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func validateGuidedProjectConfigs(root string) error {
	if err := refuseGlobalHome(root, "upgrade global specs separately"); err != nil {
		return err
	}
	resolvedRoot, err := config.ResolveSourceAlias(root)
	if err != nil {
		return fmt.Errorf("resolve project %s: %w", root, err)
	}
	resolvedRoot, err = filepath.Abs(resolvedRoot)
	if err != nil {
		return fmt.Errorf("resolve project %s: %w", root, err)
	}
	for _, directory := range []string{filepath.Join(root, config.SourceBaseDir), filepath.Join(root, defaultProjectUser)} {
		if _, err := os.Lstat(directory); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return fmt.Errorf("%s: %w", directory, err)
		}
		target, err := guidedProjectPath(resolvedRoot, directory)
		if err != nil {
			return err
		}
		info, err := os.Stat(target)
		if err != nil {
			return fmt.Errorf("%s: %w", directory, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%s: expected a project source directory", directory)
		}
	}
	base, _, err := config.ResolveConfigPath(root)
	if err != nil {
		return err
	}
	paths := []string{base}
	local := filepath.Join(root, config.LocalOverrideFileName)
	if _, err := os.Lstat(local); err == nil {
		paths = append(paths, local)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s: %w", local, err)
	}
	for _, path := range paths {
		if _, err := guidedProjectPath(resolvedRoot, path); err != nil {
			return err
		}
	}
	return validateGuidedGlobalEntries(root, resolvedRoot)
}

func guidedProjectPath(resolvedRoot, path string) (string, error) {
	target, err := config.ResolveSourceAlias(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}
	if err := refuseGlobalHome(target, "upgrade global specs separately"); err != nil {
		return "", fmt.Errorf("%s points to global specs %s: %w", path, target, err)
	}
	inside, err := guidedPhysicalPathInside(resolvedRoot, target)
	if err != nil {
		return "", fmt.Errorf("check project path %s: %w", path, err)
	}
	if !inside {
		return "", fmt.Errorf("%s points to %s outside this project; update it separately", path, target)
	}
	return target, nil
}

func validateGuidedGlobalEntries(root, resolvedRoot string) error {
	source, err := globalSourceRoot()
	if err != nil {
		return fmt.Errorf("resolve global source root: %w", err)
	}
	home, err := config.ResolveSourceAlias(source)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("resolve global source %s: %w", source, err)
	}
	home, err = filepath.Abs(home)
	if err != nil {
		return fmt.Errorf("resolve global source root: %w", err)
	}
	nested, err := guidedPhysicalPathInside(resolvedRoot, home)
	if err != nil {
		return err
	}
	if !nested {
		return nil
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	layers := []spec.Layer{resolveProjectLayer(root, cfg)}
	if local, present := resolveProjectUserLayer(root); present {
		layers = append(layers, local)
	}
	for _, layer := range layers {
		bundle, err := spec.LoadLayered([]spec.Layer{layer})
		if err != nil {
			return err
		}
		for _, entry := range bundle.All() {
			target, err := config.ResolveSourceAlias(entry.Path)
			if err != nil {
				return fmt.Errorf("resolve %s: %w", entry.Path, err)
			}
			target, err = filepath.Abs(target)
			if err != nil {
				return fmt.Errorf("resolve %s: %w", entry.Path, err)
			}
			global, err := guidedPhysicalPathInside(home, target)
			if err != nil {
				return err
			}
			if global {
				return fmt.Errorf("%s points to global specs %s; upgrade global specs separately", entry.Path, target)
			}
		}
	}
	return nil
}

func guidedPhysicalPathInside(root, path string) (bool, error) {
	rootInfo, err := os.Stat(root)
	if err != nil {
		return false, fmt.Errorf("%s: %w", root, err)
	}
	for {
		info, err := os.Stat(path)
		if err != nil {
			return false, fmt.Errorf("%s: %w", path, err)
		}
		if os.SameFile(rootInfo, info) {
			return true, nil
		}
		parent := filepath.Dir(path)
		if parent == path {
			return false, nil
		}
		path = parent
	}
}
