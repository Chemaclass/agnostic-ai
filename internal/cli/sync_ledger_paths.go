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

// mergeSpellings rewrites every output path the ledger holds with clean,
// the spelling the config's own paths get. Releases before #3329 stored
// `./AGENTS.md` as typed, so without this a renamed entry reads as a
// file sync stopped writing, and its merged keys are taken out or the
// file removed.
func (s *syncStateFile) mergeSpellings(projectRoot string, clean func(string) string) {
	s.Outputs = respelledList(s.Outputs, clean)
	s.Orphans = respelledList(s.Orphans, clean)
	s.Unledgered = respelledList(s.Unledgered, clean)
	s.OutputSums = respelled(s.OutputSums, clean, func(file string, sums []string) string {
		return matchingSum(projectRoot, file, sums)
	})
	s.Backups = respelled(s.Backups, clean, func(file string, sums []string) string {
		return matchingSum(projectRoot, file, sums)
	})
	s.Merged = respelled(s.Merged, clean, unionMerged)
}

func respelledList(paths []string, clean func(string) string) []string {
	out := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, p := range paths {
		if c := clean(p); !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return paths
	}
	return out
}

// matchingSum is the sum the bytes of file still match, so the ledger
// vouches for what sync last wrote there; else the first sum.
func matchingSum(projectRoot, file string, sums []string) string {
	data, err := os.ReadFile(filepath.Join(projectRoot, filepath.FromSlash(file)))
	if err == nil {
		for _, sum := range sums {
			if adapters.MatchesSum(data, sum) {
				return sum
			}
		}
	}
	return sums[0]
}

// unionMerged keeps every claim the spellings of one file hold, so a key
// sync set under either stays sync's to release.
func unionMerged(_ string, records []mergedOutput) mergedOutput {
	out := records[0]
	for _, m := range records[1:] {
		out.Keys = withMergedKeys(out.Keys, m.Keys)
		out.Created = out.Created || m.Created
		out.Unrecorded = out.Unrecorded || m.Unrecorded
	}
	return out
}

// respelled rekeys m by clean spelling. When several keys name one file,
// combine gets their values, the clean spelling's first.
func respelled[V any](m map[string]V, clean func(string) string, combine func(file string, values []V) V) map[string]V {
	if !slices.ContainsFunc(slices.Collect(maps.Keys(m)), func(p string) bool { return clean(p) != p }) {
		return m
	}
	byClean := map[string][]string{}
	for _, p := range slices.Sorted(maps.Keys(m)) {
		c := clean(p)
		if p == c {
			byClean[c] = append([]string{p}, byClean[c]...)
		} else {
			byClean[c] = append(byClean[c], p)
		}
	}
	out := make(map[string]V, len(byClean))
	for c, spellings := range byClean {
		values := make([]V, len(spellings))
		for i, p := range spellings {
			values[i] = m[p]
		}
		if len(values) == 1 {
			out[c] = values[0]
		} else {
			out[c] = combine(c, values)
		}
	}
	return out
}
