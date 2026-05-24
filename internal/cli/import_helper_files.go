package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// helperFile names one tool-native file that lives outside the
// per-kind spec dirs (agents, skills, rules, hooks, mcps, commands)
// but must survive a `import` → `sync` round-trip. Examples: Claude's
// `CLAUDE.md` (project memory), operator `README.md` files,
// `statusline.sh` invoked from `settings.json` statusLine.command.
//
// Each helper is captured at import to
// `.agnostic-ai/overlays/<tool>/<basename>` (file mode preserved so an
// executable script stays executable) and restored at sync to
// `.<tool>/<basename>`.
type helperFile struct {
	tool, basename string
}

// helperFilesByTool lists every helper file each importer captures.
// Keep the list small: the goal is to preserve functional files the
// agent actually reads or executes, not every stray doc that happens
// to live under `.<tool>/`.
var helperFilesByTool = map[string][]helperFile{
	"claude": {
		{tool: "claude", basename: "CLAUDE.md"},
		{tool: "claude", basename: "README.md"},
		{tool: "claude", basename: "statusline.sh"},
	},
	"codex": {
		{tool: "codex", basename: "README.md"},
	},
}

// helperOverlayPath returns the captured overlay path for a helper.
func helperOverlayPath(root, tool, basename string) string {
	return filepath.Join(root, agnosticOverlayDir, tool, basename)
}

// captureHelperFiles copies the tool-native helper files declared in
// helperFilesByTool into the overlay tree, preserving file mode. A no-op
// per file when the source is missing — most projects do not ship every
// helper. Returns the basenames that were actually captured so the
// import summary can surface them.
func captureHelperFiles(root, tool string) ([]string, error) {
	var captured []string
	for _, h := range helperFilesByTool[tool] {
		src := filepath.Join(root, "."+h.tool, h.basename)
		info, err := os.Stat(src)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return captured, fmt.Errorf("stat %s: %w", src, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		body, err := os.ReadFile(src)
		if err != nil {
			return captured, fmt.Errorf("read %s: %w", src, err)
		}
		dst := helperOverlayPath(root, h.tool, h.basename)
		if err := importMkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return captured, fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
		}
		if err := importWriteFile(dst, body, info.Mode().Perm()); err != nil {
			return captured, fmt.Errorf("write %s: %w", dst, err)
		}
		captured = append(captured, h.basename)
	}
	return captured, nil
}
