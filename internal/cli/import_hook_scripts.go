package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// agnosticScriptsDir is the project-relative root for stashed hook
// script bodies. Mirrors the constant in the emit package so import
// and sync agree on where the bodies live.
const agnosticScriptsDir = ".agnostic-ai/scripts"

// captureHookScripts copies every regular file under `<root>/.<tool>/hooks/`
// into `<root>/.agnostic-ai/scripts/<tool>/`, preserving the file mode so
// executable scripts stay executable. A no-op when the source directory
// is missing — projects that author hook commands inline (e.g. `gofmt`)
// have nothing to stash.
//
// Per-tool subdirectories let cross-tool emit fall back to the spec
// origin's body when the target has no variant of its own. Unified
// scripts can be promoted to `.agnostic-ai/scripts/<basename>` by hand
// later; this helper never collapses tools to keep import lossless.
func captureHookScripts(root, tool string) error {
	srcDir := filepath.Join(root, "."+tool, "hooks")
	dstDir := filepath.Join(root, agnosticScriptsDir, tool)
	return copyHookScriptsTree(srcDir, dstDir)
}

func copyHookScriptsTree(srcDir, dstDir string) error {
	entries, err := os.ReadDir(srcDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", srcDir, err)
	}
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		src := filepath.Join(srcDir, e.Name())
		info, err := e.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", src, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		body, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("read %s: %w", src, err)
		}
		if err := importMkdirAll(dstDir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dstDir, err)
		}
		dst := filepath.Join(dstDir, e.Name())
		if err := importWriteFile(dst, body, info.Mode().Perm()); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
	}
	return nil
}
