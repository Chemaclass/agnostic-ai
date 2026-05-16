package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	// codexOverlayDir is the project-relative directory where importers
	// stash captured per-target settings so they survive a wipe of the
	// native config tree between `import` and `sync`.
	codexOverlayDir = ".agnostic-ai/overlays"
	// codexOverlayFile is the captured non-hooks/non-mcp-servers portion
	// of `.codex/config.toml`. The emitter loads it and concatenates it
	// before the spec-derived hooks + mcp_servers sections so a re-sync
	// from a fresh checkout still produces a config.toml with model,
	// sandbox, profiles, model_providers, history, notify, and any
	// other top-level keys the user had configured.
	codexOverlayFile = "codex.config.toml"
)

// codexOverlayPath returns the project-relative path to the captured
// Codex config overlay.
func codexOverlayPath(root string) string {
	return filepath.Join(root, codexOverlayDir, codexOverlayFile)
}

// importCodexConfigOverlay reads `.codex/config.toml` under root, drops
// the `hooks` and `mcp_servers` keys (the spec-derived sections), and
// writes the remainder to `.agnostic-ai/overlays/codex.config.toml`.
//
// The overlay file becomes the authoritative source of every other
// `.codex/config.toml` key the user has configured (`model`, `sandbox`,
// `approval_policy`, `notify`, `[history]`, `[profiles.*]`,
// `[model_providers.*]`, and any future Codex keys). On `sync -t codex`
// the adapter prepends the overlay body before the generated hooks +
// MCP sections. Without the overlay, a wipe of `.codex/` between
// import and sync would destroy every non-managed key.
//
// Returns nil when config.toml is missing or contains only `hooks`
// and/or `mcp_servers`, so a fresh project does not get a surprise
// empty overlay file.
func importCodexConfigOverlay(root string) error {
	src := filepath.Join(root, codexConfigTOML)
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	doc := map[string]any{}
	if _, err := toml.Decode(string(data), &doc); err != nil {
		return fmt.Errorf("parse %s: %w", src, err)
	}
	delete(doc, "hooks")
	delete(doc, "mcp_servers")
	if len(doc) == 0 {
		return nil
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(doc); err != nil {
		return fmt.Errorf("encode overlay: %w", err)
	}
	dst := codexOverlayPath(root)
	if err := importMkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	if err := importWriteFile(dst, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}
