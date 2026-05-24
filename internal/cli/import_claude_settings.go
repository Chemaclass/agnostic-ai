package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

const (
	// claudeOverlayFile is the captured non-hooks portion of
	// `.claude/settings.json`. The emitter uses it as the base document
	// when writing settings.json, layering the spec-derived `hooks` key
	// on top.
	claudeOverlayFile = "claude.settings.json"

	// claudeHookOrderFile is a sidecar capturing the order of hook
	// event keys (`PreToolUse`, `PostToolUse`, ...) as they appeared in
	// the source settings.json. The claude adapter reads it on emit so
	// the user's authored event order survives a round-trip instead of
	// being normalized to the canonical lifecycle order.
	claudeHookOrderFile = "claude.settings.hook-events.json"
)

// claudeOverlayDir is an alias for the shared overlay directory.
// Kept as a separate identifier so call sites read claude-scoped.
const claudeOverlayDir = agnosticOverlayDir

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
// Returns (false, nil) when settings.json is missing or contains only
// `hooks`, so a fresh project does not get a surprise empty overlay
// file. Returns (true, nil) when the overlay was actually written.
func importClaudeSettingsOverlay(root string) (bool, error) {
	src := filepath.Join(root, claudeDir, "settings.json")
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", src, err)
	}
	doc := adapters.NewOrderedJSON()
	if err := json.Unmarshal(data, doc); err != nil {
		return false, fmt.Errorf("parse %s: %w", src, err)
	}
	hadHooks := false
	if rawHooks, ok := doc.Get("hooks"); ok {
		hadHooks = true
		if err := captureClaudeHookEventOrder(root, rawHooks); err != nil {
			return false, err
		}
		doc.SetRaw("hooks", json.RawMessage(`null`))
	}
	if doc.Len() == 0 || (hadHooks && doc.Len() == 1) {
		return false, nil
	}
	indent := adapters.DetectJSONIndent(data)
	raw, err := adapters.MarshalJSONIndentWith(doc, indent)
	if err != nil {
		return false, fmt.Errorf("marshal overlay: %w", err)
	}
	dst := claudeOverlayPath(root)
	if err := importMkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return false, fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	if err := importWriteFile(dst, append(raw, '\n'), 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", dst, err)
	}
	return true, nil
}

// claudeOverlayRelPath returns the overlay path relative to the project
// root, suitable for printing in import summary lines.
func claudeOverlayRelPath() string {
	return filepath.Join(claudeOverlayDir, claudeOverlayFile)
}

// captureClaudeHookEventOrder scans the raw `hooks` value from the
// source settings.json and writes the event keys in source order to
// `.agnostic-ai/overlays/claude.settings.hook-events.json`. The claude
// adapter reads that file on emit and uses the captured order in
// preference to the canonical lifecycle order, so a user authored as
// `PostToolUse` first stays that way across a round-trip.
func captureClaudeHookEventOrder(root string, rawHooks json.RawMessage) error {
	if len(rawHooks) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(rawHooks))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		// non-object hook value — nothing to record.
		return nil
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil
	}
	var events []string
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil
		}
		name, _ := key.(string)
		if name != "" {
			events = append(events, name)
		}
		// Skip the matcher-group value; we only care about key order.
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil
		}
	}
	if len(events) == 0 {
		return nil
	}
	dst := filepath.Join(root, claudeOverlayDir, claudeHookOrderFile)
	if err := importMkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	body, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal hook order: %w", err)
	}
	if err := importWriteFile(dst, append(body, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}
