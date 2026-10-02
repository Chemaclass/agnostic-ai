package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// keepHandWrittenInstructions stops a sync before it writes over an
// instructions file, such as CLAUDE.md, that holds text agnostic-ai did
// not write and AGNOSTIC_AI.md does not have (#1611). The first sync
// after `init` used to replace it with the placeholder, which is what an
// agent asked to set agnostic-ai up runs. A file with the generated
// header, one an earlier sync wrote (the ledger lists it, and a hand edit
// to it is kept as `<path>.bak`), or one whose sections AGNOSTIC_AI.md
// already holds, is written as before.
func keepHandWrittenInstructions(cfg *config.Config, b spec.Bundle, targets, ledgered []string) error {
	captured := ""
	if data, err := os.ReadFile(adapters.AgnosticEntryPointPath); err == nil {
		captured = header.Strip(string(data))
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%s: %w", adapters.AgnosticEntryPointPath, err)
	}
	files, err := renderEntryPointFiles(cfg, b, targets, captured)
	if err != nil {
		return err
	}
	var lines []string
	for _, f := range files {
		if cfg.IsUnmanaged(f.Path) || slices.Contains(ledgered, filepath.ToSlash(f.Path)) {
			continue
		}
		data, err := os.ReadFile(f.Path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("%s: %w; nothing was written", f.Path, err)
		}
		if strings.TrimSpace(string(data)) == "" || header.Has(string(data)) {
			continue
		}
		body := uncapturedEntryBody(".", f.Path, captured, string(data))
		// A CLAUDE.md that pulls AGENTS.md in with @AGENTS.md adds only
		// what follows that line.
		if rest, ok := adapters.SplitAgentsCompanion(body); ok {
			body = rest
		}
		if strings.TrimSpace(body) == "" {
			continue
		}
		if _, missing, _ := foldText(captured, captured, body); len(missing) == 0 {
			continue
		}
		source := f.Path
		if len(f.Readers) > 0 {
			source = f.Readers[0]
		}
		lines = append(lines, fmt.Sprintf("%s holds instructions agnostic-ai did not write, and %s does not have them.\n  keep them:    agnostic-ai import %s\n  replace them: move %s away, then run agnostic-ai sync",
			f.Path, adapters.AgnosticEntryPointPath, source, f.Path))
	}
	if len(lines) == 0 {
		return nil
	}
	return fmt.Errorf("%s\nnothing was written", strings.Join(lines, "\n"))
}
