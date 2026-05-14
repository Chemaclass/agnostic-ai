package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// shouldPromptTargetSelection reports whether sync should run the
// first-time target picker. True only when:
//   - no `.agnostic-ai/.sync-state` file exists (proxy for "first sync"),
//   - the config still has every supported target enabled (proxy for
//     "user hasn't already curated"),
//   - the caller hasn't opted out via --all, -t, --only, or --except
//     (handled upstream).
func shouldPromptTargetSelection(root string, cfg *config.Config) bool {
	if _, err := os.Stat(stateFilePath(root)); err == nil {
		return false
	}
	return targetsMatchDefault(cfg.Targets)
}

// targetsMatchDefault reports whether targets is set-equal to the
// canonical all-targets list. Order-insensitive; duplicates are
// tolerated (they collapse via the set).
func targetsMatchDefault(targets []string) bool {
	defaults := allTargetNames()
	set := make(map[string]bool, len(targets))
	for _, t := range targets {
		set[t] = true
	}
	if len(set) != len(defaults) {
		return false
	}
	for _, d := range defaults {
		if !set[d] {
			return false
		}
	}
	return true
}

// firstSyncTargetSelection drives the first-sync target picker.
// Returns the chosen subset and persists it to the base config file.
// Returns (nil, nil) on a silent fallback — non-TTY with no piped data
// — so the caller keeps the original targets.
func firstSyncTargetSelection(root string, in io.Reader, out io.Writer) ([]string, error) {
	picked, err := selectTargetsForSync(in, out, detectExistingTargets(root))
	if err != nil {
		return nil, err
	}
	if len(picked) == 0 {
		return nil, nil
	}
	if err := config.PersistTargets(root, picked); err != nil {
		return nil, fmt.Errorf("persist targets: %w", err)
	}
	_, _ = fmt.Fprintf(out, "→ saved %d target(s) to %s\n", len(picked), config.ConfigFileName)
	return picked, nil
}

// selectTargetsForSync returns the user's chosen target subset.
// Branches on input mode:
//   - TTY: run the same multi-select prompt as `init -i`, pre-ticking
//     preselected names.
//   - Piped (non-TTY with data on stdin): parse one comma-separated line.
//   - Neither (non-TTY, no data): return (nil, nil) for silent fallback.
//
// Differs from selectTargets (used by `init -i`), which errors instead
// of returning a silent-fallback signal.
func selectTargetsForSync(in io.Reader, stderr io.Writer, preselected []string) ([]string, error) {
	if f, ok := in.(*os.File); ok {
		if term.IsTerminal(f.Fd()) {
			return runInteractivePrompt(stderr, preselected)
		}
		if !hasPipedData(f) {
			return nil, nil
		}
	}
	br := bufio.NewReader(in)
	line, err := br.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, err
	}
	if strings.TrimSpace(line) == "" {
		return nil, nil
	}
	return parsePipedSelection(line)
}

// hasPipedData reports whether f is a pipe or regular file with bytes
// available. Used to distinguish "non-TTY with piped input" from
// "non-TTY with no input at all" (e.g. CI invocation with stdin closed).
func hasPipedData(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	if stat.Mode()&os.ModeNamedPipe != 0 {
		return true
	}
	return stat.Size() > 0
}
