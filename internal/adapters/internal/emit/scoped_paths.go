package emit

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
)

// CheckScopePath checks the nearest existing ancestor without scanning a tree.
// Capture and WASM may plan files in directories that do not exist yet.
func CheckScopePath(path string) error {
	if runtime.GOOS == "js" {
		return nil
	}
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("scope working directory: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("scope root: %w", err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for p := abs; ; p = filepath.Dir(p) {
		resolved, err := filepath.EvalSymlinks(p)
		if err == nil {
			rel, err := filepath.Rel(root, resolved)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("%s: scoped output escapes the project", path)
			}
			return nil
		}
		if info, statErr := os.Lstat(p); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s: cannot resolve scoped symlink: %w", p, err)
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("%s: %w", path, err)
		}
		if filepath.Dir(p) == p {
			return fmt.Errorf("%s: no existing ancestor for scoped output", path)
		}
	}
}

// CheckScopedDestination protects newly introduced instruction files and aliases
// that the same host would load instead of (or alongside) the generated file.
func CheckScopedDestination(path string) error {
	if runtime.GOOS == "js" {
		return nil
	}
	if err := CheckScopePath(path); err != nil {
		return err
	}
	candidates := []string{path}
	switch filepath.Base(path) {
	case "AGENTS.md":
		for _, name := range []string{"AGENTS.override.md", "WARP.md", "CLAUDE.md", ".goosehints"} {
			candidates = append(candidates, filepath.Join(filepath.Dir(path), name))
		}
	}
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		if p != path {
			return fmt.Errorf("%s: alternate instructions conflict with scoped %s; import or move the alternate file before syncing", p, filepath.Base(path))
		}
		if !header.Has(string(data)) {
			return fmt.Errorf("%s: hand-authored instructions conflict with scoped output; import or move their content before syncing", p)
		}
	}
	return nil
}
