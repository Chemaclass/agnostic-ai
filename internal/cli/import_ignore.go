package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// ignoreFileByTarget maps an import source to the ignore file its
// adapter emits. These seven are the targets with a documented
// ignore-file convention; every other target reports ignore specs as
// unsupported and has nothing to read back.
var ignoreFileByTarget = map[string]string{
	"aider":    ".aiderignore",
	"cursor":   ".cursorignore",
	"gemini":   ".geminiignore",
	"junie":    ".aiignore",
	"kiro":     ".kiroignore",
	"trae":     ".trae/.ignore",
	"windsurf": ".devinignore",
}

// importIgnoreFile reads the target's hand-authored ignore file and
// writes its patterns into `<src.Ignore>/<target>.md`, so a project
// that kept credentials out of agent context by hand keeps doing so
// once agnostic-ai owns the file. Without this there was no read-back
// path at all and the emit side had nothing to point users at (#754).
//
// A file carrying the agnostic-ai provenance header imports nothing:
// the specs that produced it are already the source of truth, and
// reading it back would emit every pattern twice on the next sync.
// A missing or empty file imports nothing either.
//
// The imported spec is unscoped, so its patterns reach every
// ignore-capable target rather than only the one they came from. That
// widens what agents may not read, never what they may.
func importIgnoreFile(root, target string, src config.Sources) (int, error) {
	name, ok := ignoreFileByTarget[target]
	if !ok || src.Ignore == "" {
		return 0, nil
	}
	path := filepath.Join(root, filepath.FromSlash(name))
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	if header.Has(string(data)) {
		return 0, nil
	}
	body := strings.TrimSpace(string(data))
	if body == "" {
		return 0, nil
	}
	if err := importMkdirAll(filepath.Join(root, src.Ignore), 0o755); err != nil {
		return 0, fmt.Errorf("mkdir %s: %w", src.Ignore, err)
	}
	out := filepath.Join(root, src.Ignore, target+".md")
	specFile := fmt.Sprintf("---\nname: %s\ndescription: Imported from %s.\n---\n\n%s\n", target, name, body)
	if err := importWriteFile(out, []byte(specFile), 0o644); err != nil {
		return 0, fmt.Errorf("write %s: %w", out, err)
	}
	return 1, nil
}
