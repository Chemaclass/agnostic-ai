package emit

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// ProjectLocalEntryPointPath is the project-relative path of the
// git-ignored instructions file in the project-user layer
// (`.agnostic-ai/local/`). Its text extends the shared
// AgnosticEntryPointPath body in every entry-point file sync writes,
// the same way `~/.agnostic-ai/local/AGNOSTIC_AI.md` extends the global
// instructions.
const ProjectLocalEntryPointPath = ".agnostic-ai/local/AGNOSTIC_AI.md"

// Sentinel markers delimiting the project-local instructions inside an
// entry-point file. Import strips the block (StripGeneratedAppendices)
// so private local text never flows back into the committed sources.
const (
	LocalStartMarker = "<!-- agnostic-ai:local:start -->"
	LocalEndMarker   = "<!-- agnostic-ai:local:end -->"
)

// ReadLocalInstructions returns the trimmed text of
// ProjectLocalEntryPointPath, or "" when the file is absent or blank.
// Any generated block pasted into it is dropped so the markers never
// nest.
func ReadLocalInstructions() (string, error) {
	data, err := os.ReadFile(ProjectLocalEntryPointPath)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", ProjectLocalEntryPointPath, err)
	}
	return strings.TrimSpace(StripGeneratedAppendices(StripHeader(string(data)))), nil
}

// AppendLocalInstructions returns body with local wrapped in the local
// sentinel markers and appended after one blank line, replacing any
// earlier block. Returns body unchanged when local is blank.
func AppendLocalInstructions(body, local string) string {
	local = strings.TrimSpace(local)
	if local == "" {
		return body
	}
	block := LocalStartMarker + "\n\n" + local + "\n\n" + LocalEndMarker + "\n"
	body = strings.TrimRight(StripLocalInstructions(body), "\n")
	if body == "" {
		return block
	}
	return body + "\n\n" + block
}

// StripLocalInstructions removes the sentinel-marked local block
// (markers included) from body. Returns body unchanged when no block is
// present.
func StripLocalInstructions(body string) string {
	return stripMarkedBlock(body, LocalStartMarker, LocalEndMarker)
}

// EntryPointView returns text as readers see it in one entry-point file:
// ::target fences resolved for readers, and `@path` lines rewritten per
// sync.resolve-imports unless every reader follows them. source names
// the file text came from, so an unresolvable import points at it.
func EntryPointView(cfg *config.Config, readers []string, source, text string) (string, error) {
	view := spec.FilterFences(text, readers)
	if allSupportFileImports(readers) {
		return view, nil
	}
	mode := ""
	if cfg != nil {
		mode = cfg.Sync.ResolveImports
	}
	resolved, err := ApplyImportMode(view, mode)
	if err != nil {
		return "", fmt.Errorf("resolve imports in %s: %w", source, err)
	}
	return resolved, nil
}

// LocalInstructionsView returns ProjectLocalEntryPointPath as readers see
// it (see EntryPointView), or "" when the file is absent or blank.
func LocalInstructionsView(cfg *config.Config, readers []string) (string, error) {
	local, err := ReadLocalInstructions()
	if err != nil || local == "" {
		return "", err
	}
	return EntryPointView(cfg, readers, ProjectLocalEntryPointPath, local)
}

// AppendLegacyEntryPointLocal appends the project-local block to content,
// the legacy concatenated rules document of target, when that document
// is target's own entry point. The central writer skips such a file (see
// LegacyRulesFileOwnsEntryPoint), so the adapter write is the only one
// that can carry the block. Returns content unchanged in any other
// layout.
func AppendLegacyEntryPointLocal(cfg *config.Config, target, content string) (string, error) {
	local, err := legacyEntryPointLocal(cfg, target)
	if err != nil {
		return "", err
	}
	return AppendLocalInstructions(content, local), nil
}

// legacyEntryPointLocal returns the project-local text as the readers of
// target's legacy entry-point document see it, or "" when target is on
// another layout. The readers are every configured target sharing that
// document, as the central writer filters a shared file, so two
// identical merged documents stay identical.
func legacyEntryPointLocal(cfg *config.Config, target string) (string, error) {
	if !LegacyRulesFileOwnsEntryPoint(cfg, target) {
		return "", nil
	}
	return LocalInstructionsView(cfg, legacyEntryPointReaders(cfg, target))
}

// legacyEntryPointReaders returns target plus every other configured
// target whose legacy rules document is the same entry-point file.
func legacyEntryPointReaders(cfg *config.Config, target string) []string {
	readers := []string{target}
	path := EntryPointPath(cfg, target)
	for _, t := range cfg.Targets {
		if t != target && LegacyRulesFileOwnsEntryPoint(cfg, t) && samePath(EntryPointPath(cfg, t), path) {
			readers = append(readers, t)
		}
	}
	return readers
}

// allSupportFileImports reports whether every reader resolves `@path`
// file-import lines natively.
func allSupportFileImports(readers []string) bool {
	for _, t := range readers {
		if !SupportsFileImports(t) {
			return false
		}
	}
	return len(readers) > 0
}
