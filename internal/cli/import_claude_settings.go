package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
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

// importClaudeSettingsOverlay reads `.claude/settings.json` under root
// and writes it to `.agnostic-ai/overlays/claude.settings.json` with the
// `hooks` value replaced by a null sentinel.
//
// The overlay file becomes the authoritative source of non-hook settings
// (statusLine, enabledPlugins, model overrides, anything the user has
// configured). On `sync -t claude` the adapter loads the overlay, merges
// the hook output on top, and writes the result. Without the overlay,
// wiping `.claude/` between import and sync would lose every non-hook
// key.
//
// The hooks key is kept as a `null` sentinel rather than deleted so the
// overlay preserves the author's original key position. `writeSettings`
// overwrites the sentinel with the spec-derived hook map on every sync,
// keeping hooks at the position the user authored (#227).
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
	doc := adapters.NewOrderedJSON()
	if err := json.Unmarshal(data, doc); err != nil {
		return fmt.Errorf("parse %s: %w", src, err)
	}
	hadHooks := false
	if _, ok := doc.Get("hooks"); ok {
		hadHooks = true
		doc.SetRaw("hooks", json.RawMessage(`null`))
	}
	if doc.Len() == 0 || (hadHooks && doc.Len() == 1) {
		return nil
	}
	raw, err := adapters.MarshalJSONIndent(doc)
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
