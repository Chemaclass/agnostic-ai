package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
)

// importScopedSkillFolders discovers the target's native skill directory at
// the repository root and below every project subdirectory. The prefix before
// the native directory becomes the canonical skill scope.
type scopedSkillDir struct {
	path  string
	scope string
}

func findScopedSkillDirs(root, nativeDir string) ([]scopedSkillDir, error) {
	nativeDir = filepath.ToSlash(filepath.Clean(nativeDir))
	var found []scopedSkillDir
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		if path != root && (entry.Name() == ".git" || entry.Name() == ".agnostic-ai") {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel != nativeDir && !strings.HasSuffix(rel, "/"+nativeDir) {
			return nil
		}
		scope := strings.TrimSuffix(rel, nativeDir)
		scope = strings.TrimSuffix(scope, "/")
		found = append(found, scopedSkillDir{path: path, scope: scope})
		return filepath.SkipDir
	})
	if err != nil {
		return nil, fmt.Errorf("discover scoped skills under %s: %w", root, err)
	}
	return found, nil
}

func importScopedSkillFolders(root, nativeDir, dstDir string) (int, error) {
	return importScopedSkillFoldersFrom(root, []string{nativeDir}, dstDir)
}

// importScopedSkillFoldersFrom scans every directory in nativeDirs, at
// the repository root and below every project subdirectory. Earlier
// entries win: when the same skill name appears at the same scope under
// two of them, the first is imported and the rest are skipped. Each
// target lists its own emit path first so an emit then import reads back
// the tree the adapter wrote. Precedence is tracked per scope, since the
// same name at two different scopes is two different skills.
func importScopedSkillFoldersFrom(root string, nativeDirs []string, dstDir string) (int, error) {
	count := 0
	seen := map[string]map[string]bool{}
	for _, nativeDir := range nativeDirs {
		dirs, err := findScopedSkillDirs(root, nativeDir)
		if err != nil {
			return count, err
		}
		for _, dir := range dirs {
			if seen[dir.scope] == nil {
				seen[dir.scope] = map[string]bool{}
			}
			imported, err := importSkillFoldersWith(
				dir.path,
				filepath.Join(dstDir, filepath.FromSlash(dir.scope)),
				skillFolderImportOpts{SkipNames: seen[dir.scope]},
			)
			if err != nil {
				return count, err
			}
			count += imported
		}
	}
	return count, nil
}

// importSkillFolders copies each `<srcDir>/<name>/` directory tree that
// contains a SKILL.md into `<dstDir>/<name>/` byte-for-byte, so a
// round-trip preserves the full payload (scripts, references, assets).
// Folders without a SKILL.md are skipped; a missing srcDir imports
// nothing. Shared by every importer whose tool uses the Agent Skills
// folder layout (cursor, gemini, opencode, copilot).
func importSkillFolders(srcDir, dstDir string) (int, error) {
	return importSkillFoldersWith(srcDir, dstDir, skillFolderImportOpts{})
}

type skillFolderImportOpts struct {
	SkipNames      map[string]bool
	TransformSkill func([]byte) ([]byte, error)
}

// importSkillFoldersWith imports a native skill tree with optional
// collision handling and SKILL.md normalization. Candidate lists share a
// SkipNames map so the documented path order becomes explicit precedence
// without treating a pre-existing destination spec as a native collision.
func importSkillFoldersWith(srcDir, dstDir string, opts skillFolderImportOpts) (int, error) {
	entries, err := os.ReadDir(srcDir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", srcDir, err)
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillSrc := filepath.Join(srcDir, e.Name())
		skillDst := filepath.Join(dstDir, e.Name())
		if _, err := os.Stat(filepath.Join(skillSrc, "SKILL.md")); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return count, fmt.Errorf("stat skill %s: %w", e.Name(), err)
		}
		if opts.SkipNames[e.Name()] {
			continue
		}
		if err := copyDirTreeWith(skillSrc, skillDst, opts.TransformSkill); err != nil {
			return count, fmt.Errorf("copy skill %s: %w", e.Name(), err)
		}
		if opts.SkipNames != nil {
			opts.SkipNames[e.Name()] = true
		}
		count++
	}
	return count, nil
}

// copyDirTree walks srcDir recursively and writes every regular file
// byte-for-byte into the matching location under dstDir, recreating
// the directory layout as it goes. File mode bits are preserved so an
// executable script remains executable on the destination. Symlinks
// are not followed; if they appear inside a skill folder they are
// silently skipped (skills are documented to be plain files +
// directories — symlinks would not survive a tar/zip release anyway).
func copyDirTree(srcDir, dstDir string) error {
	return copyDirTreeWith(srcDir, dstDir, nil)
}

func copyDirTreeWith(srcDir, dstDir string, transformSkill func([]byte) ([]byte, error)) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return importMkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		// Strip the agnostic-ai provenance header from SKILL.md so a
		// roundtrip (claude / codex emit -> import) does not bake the
		// header into the source spec. Sibling assets pass through
		// byte-for-byte because they are user-authored.
		if filepath.Base(path) == "SKILL.md" {
			data = []byte(header.Strip(string(data)))
			if transformSkill != nil {
				data, err = transformSkill(data)
				if err != nil {
					return fmt.Errorf("transform %s: %w", path, err)
				}
			}
		}
		if err := importMkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(target), err)
		}
		if err := importWriteFile(target, data, info.Mode().Perm()); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
		return nil
	})
}
