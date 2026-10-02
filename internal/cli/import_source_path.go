package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

type importSourceCopy struct {
	original string
	shadow   string
	physical string
}

var importSourceCopies []importSourceCopy

func importSourcePath(root, source string) string {
	return importPreviewSourcePath(config.ResolveSourcePath(root, source))
}

func importPreviewSourcePath(path string) string {
	for _, copy := range importSourceCopies {
		if rel, ok := pathBelow(copy.original, path); ok {
			return filepath.Join(copy.shadow, rel)
		}
	}
	return path
}

func importOriginalSourcePath(path string) string {
	if !filepath.IsAbs(path) {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	for _, copy := range importSourceCopies {
		if rel, ok := pathBelow(copy.shadow, abs); ok {
			return filepath.Join(copy.original, rel)
		}
	}
	return path
}

func pathBelow(base, path string) (string, bool) {
	rel, err := filepath.Rel(base, path)
	return rel, err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func copyImportAbsoluteSources(project, shadow, previewRoot string, copied previewCopy) ([]importSourceCopy, error) {
	if copied.files == nil {
		copied.files = map[previewFileKey][]previewCopiedFile{}
	}
	if copied.dirs == nil {
		copied.dirs = previewDirectoryCopies{resolveImportSourceExisting(project): shadow}
	}
	cfg, err := config.Load(shadow)
	if err != nil {
		return nil, nil
	} // The importer reports config errors.
	var dirs []string
	for _, dir := range sourceDirsByKind(cfg.Sources) {
		if filepath.IsAbs(dir) {
			dirs = append(dirs, filepath.Clean(dir))
		}
	}
	slices.SortFunc(dirs, func(a, b string) int {
		ap, bp := resolveImportSourceExisting(a), resolveImportSourceExisting(b)
		if len(ap) != len(bp) {
			return len(ap) - len(bp)
		}
		if ap != bp {
			return strings.Compare(ap, bp)
		}
		return strings.Compare(a, b)
	})
	dirs = slices.Compact(dirs)
	projectPhysical := resolveImportSourceExisting(project)
	var copies []importSourceCopy
	for _, dir := range dirs {
		physical := resolveImportSourceExisting(dir)
		var target string
		for _, copy := range copies {
			if rel, nested := pathBelow(copy.original, dir); nested {
				target = filepath.Join(copy.shadow, rel)
				break
			}
			if rel, alias := pathBelow(copy.physical, physical); alias {
				target, err = newImportSourceAlias(previewRoot, filepath.Join(copy.shadow, rel))
				if err != nil {
					return nil, err
				}
				break
			}
		}
		if target == "" {
			if reused, ok := copied.dirs.shadow(physical); ok {
				_, exists := copied.dirs[physical]
				if _, statErr := os.Stat(physical); exists || errors.Is(statErr, fs.ErrNotExist) {
					if rel, inside := pathBelow(projectPhysical, physical); inside && reused == filepath.Join(shadow, rel) {
						target = reused
					} else {
						target, err = newImportSourceAlias(previewRoot, reused)
						if err != nil {
							return nil, err
						}
					}
				}
			}

			copyAncestor := false
			if target == "" {
				if rel, ancestor := pathBelow(physical, projectPhysical); ancestor {
					target = shadow
					for p := rel; p != "."; p = filepath.Dir(p) {
						target = filepath.Dir(target)
					}
					copyAncestor = true
				}
			}
			if target == "" || copyAncestor {
				if target == "" {
					target, err = os.MkdirTemp(previewRoot, "source-")
				}
				if err != nil {
					return nil, fmt.Errorf("create source preview: %w", err)
				}
				root, resolveErr := resolveImportSource(dir)
				if resolveErr != nil && !errors.Is(resolveErr, fs.ErrNotExist) {
					return nil, fmt.Errorf("%s: %w", dir, resolveErr)
				}
				if resolveErr != nil {
					if info, err := os.Lstat(dir); err == nil && (info.Mode()&os.ModeSymlink != 0 || importSourceDirectoryLink(info)) {
						return nil, fmt.Errorf("%s: %w", dir, resolveErr)
					}
				}
				if resolveErr == nil {
					var retained []string
					if copyAncestor {
						retained = []string{projectPhysical}
					}
					copier := previewCopier{root: root, dstRoot: target, detached: true,
						excluded: []string{resolveImportSourceExisting(previewRoot)}, retainedDirs: retained, visited: map[string]bool{root: true}, outsideFiles: map[string]bool{}, nested: map[string]bool{}, files: copied.files, dirs: copied.dirs, sourceCopy: true}
					if err := copier.copyDir(root, target); err != nil {
						return nil, fmt.Errorf("copy source %s for preview: %w", dir, err)
					}
				}
			}
		}
		copies = append(copies, importSourceCopy{original: dir, shadow: target, physical: physical})
	}
	// A nested source directory must match before its parent.
	slices.SortFunc(copies, func(a, b importSourceCopy) int { return len(b.original) - len(a.original) })
	return copies, nil
}

func importSourceRelativePath(root, source string) string {
	base, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	path, err := filepath.Abs(importSourcePath(root, source))
	if err != nil {
		return ""
	}
	if rel, inside := pathBelow(base, path); inside {
		return filepath.ToSlash(rel)
	}
	if rel, inside := pathBelow(resolveImportSourceExisting(base), resolveImportSourceExisting(path)); inside {
		return filepath.ToSlash(rel)
	}
	return ""
}

func importPreviewProjectPath(project, previewRoot string) string {
	cfg, err := config.Load(project)
	if err != nil {
		return filepath.Join(previewRoot, filepath.Base(project))
	}
	physical := resolveImportSourceExisting(project)
	ancestor := physical
	for _, source := range sourceDirsByKind(cfg.Sources) {
		if !filepath.IsAbs(source) {
			continue
		}
		dir := resolveImportSourceExisting(source)
		if _, inside := pathBelow(dir, physical); inside && len(dir) < len(ancestor) {
			ancestor = dir
		}
	}
	if ancestor == physical {
		return filepath.Join(previewRoot, filepath.Base(project))
	}
	rel, _ := filepath.Rel(ancestor, physical)
	return filepath.Join(previewRoot, "source-root", rel)
}

func importMappedSources(root string, sources config.Sources) config.Sources {
	mapped := func(source string) string {
		if source == "" {
			return ""
		}
		return resolveImportSourceExisting(importSourcePath(root, source))
	}
	return config.Sources{
		Agents: mapped(sources.Agents), Skills: mapped(sources.Skills), Rules: mapped(sources.Rules),
		Hooks: mapped(sources.Hooks), MCPs: mapped(sources.MCPs), Commands: mapped(sources.Commands),
		Settings: mapped(sources.Settings), Reviews: mapped(sources.Reviews),
		Environments: mapped(sources.Environments), Ignore: mapped(sources.Ignore),
	}
}
