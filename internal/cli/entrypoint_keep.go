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
// already holds, is written as before. With backup, a file is replaced
// when its `<path>.bak` is free. --keep-edits keeps only a file the ledger
// or Git knows, so only those are exempt.
func keepHandWrittenInstructions(cfg *config.Config, b spec.Bundle, targets, ledgered []string, backup, keepEdits bool) error {
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
	held, err := heldInstructions(captured)
	if err != nil {
		return err
	}
	var lines []string
	for _, f := range files {
		if cfg.IsUnmanaged(f.Path) || slices.Contains(ledgered, filepath.ToSlash(f.Path)) {
			continue
		}
		// --keep-edits keeps a file Git tracks when it differs from HEAD.
		if keepEdits {
			if sum := committedSum(f.Path); sum != "" && fileSum(f.Path) != sum {
				continue
			}
		}
		uncaptured, err := handWrittenUncaptured(f.Path, held)
		if err != nil {
			return fmt.Errorf("%w; nothing was written", err)
		}
		if !uncaptured {
			continue
		}
		// --backup keeps the file as <path>.bak, unless that would replace
		// an earlier backup.
		if backup {
			if _, err := os.Lstat(f.Path + ".bak"); errors.Is(err, fs.ErrNotExist) {
				continue
			}
			lines = append(lines, fmt.Sprintf("%s holds instructions agnostic-ai did not write, and %s.bak already exists, so --backup would replace it.\n  move %s.bak away, then run agnostic-ai sync --backup", f.Path, f.Path, f.Path))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s holds instructions agnostic-ai did not write, and %s does not have them.\n  keep them:    agnostic-ai import %s\n  replace them: agnostic-ai sync --backup, which keeps the file as %s.bak",
			f.Path, adapters.AgnosticEntryPointPath, importSourceFor(f), f.Path))
	}
	if len(lines) == 0 {
		return nil
	}
	return fmt.Errorf("%s\nnothing was written", strings.Join(lines, "\n"))
}

// heldInstructions is captured, the AGNOSTIC_AI.md body, plus the local
// layer: personal text there reaches the same files, so it is held too,
// and import must not copy it into the shared body.
func heldInstructions(captured string) (string, error) {
	local, err := adapters.ReadLocalInstructions()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(local) == "" {
		return captured, nil
	}
	return captured + "\n\n" + local, nil
}

// importSourceFor names an import source that reads f: its first reader
// with an importer, else codex for the root AGENTS.md, which it reads.
func importSourceFor(f entryPointFile) string {
	for _, r := range f.Readers {
		if _, rulesDir := rulesDirImporters[r]; rulesDir || slices.Contains(importSourceNames, r) {
			return r
		}
	}
	if filepath.ToSlash(f.Path) == claudeAgentsMainFile {
		return "codex"
	}
	return "all"
}

// handWrittenUncaptured reports whether path holds hand-written text,
// without the generated header, whose sections captured, the
// AGNOSTIC_AI.md body, does not hold. A missing file holds none.
func handWrittenUncaptured(path, captured string) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	if strings.TrimSpace(string(data)) == "" || header.Has(string(data)) {
		return false, nil
	}
	body := uncapturedEntryBody(".", path, captured, string(data))
	// A CLAUDE.md that pulls AGENTS.md in with @AGENTS.md adds only
	// what follows that line.
	if rest, ok := adapters.SplitAgentsCompanion(body); ok {
		body = rest
	}
	if strings.TrimSpace(body) == "" {
		return false, nil
	}
	_, missing, _ := foldText(captured, captured, body)
	return len(missing) > 0, nil
}

// checkHandWrittenInstructions runs keepHandWrittenInstructions for a
// preview (--check, --plan, --json --dry-run), so it agrees with the sync
// it previews.
func checkHandWrittenInstructions(targets []string, backup bool) error {
	cfg, b, err := loadProject(".")
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		targets = cfg.Targets
	}
	return keepHandWrittenInstructions(cfg, b, targets, readStateFile(".").Outputs, backup, false)
}
