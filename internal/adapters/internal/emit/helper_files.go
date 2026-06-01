// Package emit helper_files restores tool-native helper files captured
// at import time so a fresh sync rebuilds a self-contained `.<tool>/`
// tree without re-running import.
//
// Captured layout (set by `agnostic-ai import <tool>`):
//
//	.agnostic-ai/overlays/<tool>/<basename>
//
// Restored layout (set by each adapter's Emit()):
//
//	.<tool>/<basename>
//
// File mode is preserved so an executable helper (e.g. statusline.sh)
// stays executable on restore.
package emit

import (
	"fmt"
	"os"
	"path/filepath"
)

// agnosticOverlayDir is the project-relative root for captured tool-native
// files outside the spec dirs. Mirrors the constant in the cli package
// so emit and import agree.
const agnosticOverlayDir = ".agnostic-ai/overlays"

// RestoreHelperFiles copies every `.agnostic-ai/overlays/<tool>/*` file
// back to `.<tool>/<basename>` with its mode bits preserved. No-op when
// the overlay tree is missing or empty — projects that did not capture
// any helpers have nothing to restore.
func RestoreHelperFiles(tool string, dryRun bool) error {
	srcDir := filepath.Join(agnosticOverlayDir, tool)
	entries, err := os.ReadDir(srcDir)
	if IsAbsent(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", srcDir, err)
	}
	for _, e := range entries {
		if e.IsDir() || e.Name() == "" {
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
		dst := filepath.Join("."+tool, e.Name())
		if dryRun {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
		}
		if err := os.WriteFile(dst, body, info.Mode().Perm()); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
	}
	return nil
}
