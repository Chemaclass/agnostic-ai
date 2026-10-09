package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
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
	return nil
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
	rel, err := filepath.Rel(resolvedRoot, target)
	if err != nil {
		return "", fmt.Errorf("check project path %s: %w", path, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s points to %s outside this project; update it separately", path, target)
	}
	return target, nil
}
