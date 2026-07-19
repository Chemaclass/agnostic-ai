package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
)

// importSkillFolders copies each `<srcDir>/<name>/` directory tree that
// contains a SKILL.md into `<dstDir>/<name>/` byte-for-byte, so a
// round-trip preserves the full payload (scripts, references, assets).
// Folders without a SKILL.md are skipped; a missing srcDir imports
// nothing. Shared by every importer whose tool uses the Agent Skills
// folder layout (cursor, gemini, opencode, copilot).
func importSkillFolders(srcDir, dstDir string) (int, error) {
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
		if _, err := os.Stat(filepath.Join(skillSrc, "SKILL.md")); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return count, fmt.Errorf("stat skill %s: %w", e.Name(), err)
		}
		if err := copyDirTree(skillSrc, filepath.Join(dstDir, e.Name())); err != nil {
			return count, fmt.Errorf("copy skill %s: %w", e.Name(), err)
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
