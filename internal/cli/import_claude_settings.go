package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	// claudeOverlayDir is the project-relative directory where importers
	// stash captured per-target settings so they survive a wipe of the
	// native config tree between `import` and `sync`.
	claudeOverlayDir = ".agnostic-ai/overlays"
	// claudeOverlayFile is the captured non-hooks portion of
	// `.claude/settings.json`. The emitter uses it as the base document
	// when writing settings.json, layering the spec-derived `hooks` key
	// on top.
	claudeOverlayFile = "claude.settings.json"
)

// claudeOverlayPath returns the project-relative path to the captured
// Claude settings overlay.
func claudeOverlayPath(root string) string {
	return filepath.Join(root, claudeOverlayDir, claudeOverlayFile)
}

// importClaudeSettingsOverlay reads `.claude/settings.json` under root,
// strips the `hooks` key (which is captured separately as yaml specs),
// and writes the remainder to `.agnostic-ai/overlays/claude.settings.json`.
//
// The overlay file becomes the authoritative source of non-hook settings
// (statusLine, enabledPlugins, model overrides, anything the user has
// configured). On `sync -t claude` the adapter loads the overlay, merges
// the hook output on top, and writes the result. Without the overlay,
// wiping `.claude/` between import and sync would lose every non-hook
// key.
//
// Returns nil when settings.json is missing or contains only `hooks`,
// so a fresh project does not get a surprise empty overlay file.
func importClaudeSettingsOverlay(root string) error {
	src := filepath.Join(root, claudeDir, "settings.json")
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", src, err)
	}
	delete(doc, "hooks")
	if len(doc) == 0 {
		return nil
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal overlay: %w", err)
	}
	dst := claudeOverlayPath(root)
	if err := importMkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	if err := importWriteFile(dst, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}
