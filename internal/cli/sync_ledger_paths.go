package cli

import (
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// ledgerWarnOut receives the warning for a ledger entry outside the project.
var ledgerWarnOut io.Writer = os.Stderr

// warnedLedgerEntries keeps each warning to one line per state file and
// entry, since sync and its hooks read the ledger many times.
var warnedLedgerEntries sync.Map

// dropEscapingEntries removes every path outside the project or in .git from the
// ledger fields sync writes to or removes through. The ledger is
// committed with the project, so such an entry is corrupt or crafted:
// sync names it and never touches the file it points at.
func (s *syncStateFile) dropEscapingEntries(projectRoot, statePath string) {
	var dropped []string
	keep := func(p string) bool {
		if config.UnsafeProjectPathIn(projectRoot, p) {
			dropped = append(dropped, p)
			return false
		}
		return true
	}
	filter := func(paths []string) []string {
		return slices.DeleteFunc(paths, func(p string) bool { return !keep(p) })
	}
	s.Outputs = filter(s.Outputs)
	s.Orphans = filter(s.Orphans)
	s.Unledgered = filter(s.Unledgered)
	maps.DeleteFunc(s.OutputSums, func(p, _ string) bool { return !keep(p) })
	maps.DeleteFunc(s.Backups, func(p, _ string) bool { return !keep(p) })
	maps.DeleteFunc(s.Merged, func(p string, _ mergedOutput) bool { return !keep(p) })
	if len(dropped) == 0 {
		return
	}
	slices.Sort(dropped)
	for _, p := range slices.Compact(dropped) {
		key := statePath
		if abs, err := filepath.Abs(statePath); err == nil {
			key = abs
		}
		if _, seen := warnedLedgerEntries.LoadOrStore(key+"\x00"+p, true); seen {
			continue
		}
		_, _ = fmt.Fprintf(ledgerWarnOut, "warning: %s: skipped an entry outside the project or inside .git, the file may be corrupt: %q\n", statePath, p)
	}
}

// mergeSpellings rewrites every output path the ledger holds in the
// spelling config.CleanOutputPath gives the config's own paths. Releases
// before #3329 stored `./AGENTS.md` as typed, so without this a renamed
// entry reads as a file sync stopped writing, and its merged keys are
// taken out or the file removed.
func (s *syncStateFile) mergeSpellings(projectRoot string) {
	s.Outputs = respelledList(s.Outputs)
	s.Orphans = respelledList(s.Orphans)
	s.Unledgered = respelledList(s.Unledgered)
	s.OutputSums = respelledSums(projectRoot, s.OutputSums)
	s.Backups = respelledSums(projectRoot, s.Backups)
	s.Merged = respelled(s.Merged, func(string, []string) string { return "" })
}

func respelledList(paths []string) []string {
	if !slices.ContainsFunc(paths, misspelled) {
		return paths
	}
	out := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, p := range paths {
		if c := config.CleanOutputPath(p); !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

// respelledSums keeps, for a file recorded under several spellings, the
// sum its bytes on disk still match, so the ledger vouches for what sync
// last wrote there.
func respelledSums(projectRoot string, sums map[string]string) map[string]string {
	return respelled(sums, func(clean string, spellings []string) string {
		data, err := os.ReadFile(filepath.Join(projectRoot, filepath.FromSlash(clean)))
		if err != nil {
			return ""
		}
		for _, p := range spellings {
			if adapters.MatchesSum(data, sums[p]) {
				return p
			}
		}
		return ""
	})
}

// respelled rekeys m by clean spelling. When several keys name one file,
// pick chooses among them; otherwise the clean spelling wins, then the
// first in sort order.
func respelled[V any](m map[string]V, pick func(clean string, spellings []string) string) map[string]V {
	if !slices.ContainsFunc(slices.Collect(maps.Keys(m)), misspelled) {
		return m
	}
	byClean := map[string][]string{}
	for _, p := range slices.Sorted(maps.Keys(m)) {
		c := config.CleanOutputPath(p)
		byClean[c] = append(byClean[c], p)
	}
	out := make(map[string]V, len(byClean))
	for c, spellings := range byClean {
		chosen := spellings[0]
		if len(spellings) > 1 {
			if p := pick(c, spellings); p != "" {
				chosen = p
			} else if slices.Contains(spellings, c) {
				chosen = c
			}
		}
		out[c] = m[chosen]
	}
	return out
}

func misspelled(p string) bool {
	return config.CleanOutputPath(p) != p
}
