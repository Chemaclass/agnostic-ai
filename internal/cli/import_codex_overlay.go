package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
)

const (
	// codexOverlayFile is the captured non-hooks/non-mcp-servers portion
	// of `.codex/config.toml`. The emitter loads it and concatenates it
	// before the spec-derived hooks + mcp_servers sections so a re-sync
	// from a fresh checkout still produces a config.toml with model,
	// sandbox, profiles, model_providers, history, notify, and any
	// other top-level keys the user had configured.
	codexOverlayFile = "codex.config.toml"

	// codexSettingsSpec is the settings spec a promoted
	// model_reasoning_effort lands in.
	codexSettingsSpec = "codex.yaml"
)

// codexOverlayDir is an alias for the shared overlay directory. Kept
// as a separate identifier so call sites read codex-scoped.
const codexOverlayDir = agnosticOverlayDir

// codexOverlayPath returns the project-relative path to the captured
// Codex config overlay.
func codexOverlayPath(root string) string {
	return filepath.Join(root, codexOverlayDir, codexOverlayFile)
}

// codexOverlayRelPath returns the overlay path relative to the project
// root, suitable for printing in import summary lines.
func codexOverlayRelPath() string {
	return filepath.Join(codexOverlayDir, codexOverlayFile)
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
// A promotable top-level `model_reasoning_effort` moves to the portable
// settings `effort` in settingsDir instead, so every target syncs it;
// promoted reports that.
//
// Returns (false, nil) when config.toml is missing or contains only
// `hooks` and/or `mcp_servers`, so a fresh project does not get a
// surprise empty overlay file. Returns (true, nil) when the overlay
// was actually written.
func importCodexConfigOverlay(root, settingsDir string) (seeded, promoted bool, err error) {
	src := filepath.Join(root, codexConfigTOML)
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("read %s: %w", src, err)
	}

	// Validate that the input is parseable as TOML before trying to strip
	// the spec-managed sections; surfacing a parse error here points the
	// user at the real config.toml rather than the filtered overlay.
	raw := header.Strip(string(data))
	doc := map[string]any{}
	if _, err := toml.Decode(raw, &doc); err != nil {
		return false, false, fmt.Errorf("parse %s: %w", src, err)
	}
	delete(doc, "hooks")
	delete(doc, "mcp_servers")
	if raw, promoted, err = promoteCodexEffort(raw, doc, settingsDir); err != nil {
		return false, false, err
	}
	if len(doc) == 0 {
		return false, promoted, nil
	}

	// Text-level strip preserves multi-line string literals, comments,
	// blank lines, and key ordering that a decode/encode round-trip
	// would otherwise normalize away. Falling back to encoding via the
	// TOML library only happens when the text strip cannot produce a
	// valid overlay (e.g. malformed input the parser still accepted).
	filtered := stripCodexSpecManagedSections(raw)
	if filtered = strings.TrimSpace(filtered); filtered != "" {
		filtered += "\n"
	}
	if filtered == "" {
		var buf bytes.Buffer
		if err := toml.NewEncoder(&buf).Encode(doc); err != nil {
			return false, promoted, fmt.Errorf("encode overlay: %w", err)
		}
		filtered = buf.String()
	}

	dst := codexOverlayPath(root)
	if err := importMkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return false, promoted, fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	if err := importWriteFile(dst, []byte(filtered), 0o644); err != nil {
		return false, promoted, fmt.Errorf("write %s: %w", dst, err)
	}
	return true, promoted, nil
}

// promoteCodexEffort moves a top-level model_reasoning_effort out of raw
// and doc into the portable settings `effort`. A value another settings
// spec shadows stays, since the overlay outranks portable settings. The
// text edit is kept only when raw still decodes to doc without the key,
// so an unusual layout stays in the overlay untouched.
func promoteCodexEffort(raw string, doc map[string]any, settingsDir string) (string, bool, error) {
	const key = "model_reasoning_effort"
	plan, level, err := planSettingsEffort("codex", doc[key], settingsDir, codexSettingsSpec)
	if err != nil || plan != effortPromote {
		return raw, false, err
	}
	stripped := stripTopLevelTOMLKey(raw, key)
	got := map[string]any{}
	if _, err := toml.Decode(stripped, &got); err != nil {
		return raw, false, nil
	}
	delete(got, "hooks")
	delete(got, "mcp_servers")
	want := maps.Clone(doc)
	delete(want, key)
	if !reflect.DeepEqual(got, want) {
		return raw, false, nil
	}
	if err := writeSettingsSpec(settingsDir, codexSettingsSpec, settingsEffortSpec(plan, "codex", key, level)); err != nil {
		return raw, false, err
	}
	delete(doc, key)
	return stripped, true, nil
}

// stripTopLevelTOMLKey removes the single-line `key = value` assignment
// that precedes the first table header in raw.
func stripTopLevelTOMLKey(raw, key string) string {
	lines := strings.Split(raw, "\n")
	inMultiline := ""
	for i, line := range lines {
		if inMultiline != "" {
			if strings.Contains(line, inMultiline) {
				inMultiline = ""
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if isCodexSectionHeader(trimmed) {
			break
		}
		if name, _, found := strings.Cut(trimmed, "="); found && strings.TrimSpace(name) == key {
			if openMultilineDelimiter(line) != "" {
				return raw
			}
			return strings.Join(append(lines[:i:i], lines[i+1:]...), "\n")
		}
		inMultiline = openMultilineDelimiter(line)
	}
	return raw
}

// stripCodexSpecManagedSections removes every `[[hooks.<event>]]` and
// `[mcp_servers.<name>]` table from raw TOML while leaving every other
// byte intact. This preserves multi-line string literals, comments, and
// the author's original key ordering — qualities the BurntSushi encoder
// normalizes away on a decode/encode round-trip.
//
// A section runs from its `[…]` header line up to (but not including)
// the next header line or end-of-file. Lines inside a multi-line string
// (triple-double or triple-single-quote literals) are never treated as
// section headers, so a `[bracketed]` example inside a docstring will
// not split the value.
func stripCodexSpecManagedSections(raw string) string {
	var out strings.Builder
	out.Grow(len(raw))

	lines := strings.Split(raw, "\n")
	skipping := false
	inMultiline := ""
	for i, line := range lines {
		// Track whether we're inside a triple-quoted block.
		// Section-header detection must not fire on lines that are
		// part of a string value.
		if inMultiline != "" {
			if strings.Contains(line, inMultiline) {
				inMultiline = ""
			}
		} else {
			inMultiline = openMultilineDelimiter(line)
		}

		trimmed := strings.TrimSpace(line)
		if inMultiline == "" && isCodexSectionHeader(trimmed) {
			skipping = isManagedSectionHeader(trimmed)
			if skipping {
				continue
			}
		}
		if skipping {
			continue
		}
		out.WriteString(line)
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return collapseBlankLineRuns(out.String())
}

// openMultilineDelimiter returns the triple-quote delimiter that opens
// line when it does not close on the same line; otherwise it returns
// the empty string. The check is deliberately simple: TOML multi-line
// strings rarely appear in the codex overlay (the only common case is
// developer_instructions).
func openMultilineDelimiter(line string) string {
	for _, delim := range []string{`"""`, `'''`} {
		first := strings.Index(line, delim)
		if first < 0 {
			continue
		}
		last := strings.LastIndex(line, delim)
		if last == first {
			return delim
		}
	}
	return ""
}

// isCodexSectionHeader reports whether a trimmed line opens a new TOML
// section (either a regular `[name]` table or an `[[name]]` array of
// tables). Multi-line strings are out-of-scope because they live inside
// a value, not at column zero after a trim.
func isCodexSectionHeader(trimmed string) bool {
	if !strings.HasPrefix(trimmed, "[") {
		return false
	}
	return strings.HasSuffix(trimmed, "]")
}

// isManagedSectionHeader reports whether a section header opens a
// spec-managed table that the overlay should drop. The patterns mirror
// the keys agnostic-ai owns: hooks (lifecycle hooks) and mcp_servers
// (MCP server registrations).
func isManagedSectionHeader(trimmed string) bool {
	switch {
	case strings.HasPrefix(trimmed, "[[hooks."),
		strings.HasPrefix(trimmed, "[[hooks]]"),
		strings.HasPrefix(trimmed, "[hooks."),
		trimmed == "[hooks]",
		strings.HasPrefix(trimmed, "[mcp_servers."),
		trimmed == "[mcp_servers]":
		return true
	}
	return false
}

// collapseBlankLineRuns collapses runs of two or more blank lines down
// to a single blank line so the overlay file stays compact after
// stripping managed sections.
func collapseBlankLineRuns(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	blank := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return strings.TrimRight(out.String(), "\n")
}
