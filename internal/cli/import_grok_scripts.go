package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
)

func captureGrokHookScripts(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", src, err)
	}
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("%s: %w", path, walkErr)
		}
		if path == src {
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 || importLocal.leavesHookScript(entry.Name()) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if header.Leads(path, string(data)) {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		out := filepath.Join(dst, rel)
		if err := importMkdirAll(filepath.Dir(out), 0755); err != nil {
			return fmt.Errorf("%s: %w", out, err)
		}
		if err := importWriteFile(out, data, info.Mode().Perm()); err != nil {
			return fmt.Errorf("%s: %w", out, err)
		}
		return nil
	})
}
