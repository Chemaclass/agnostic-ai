package cli

import (
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"

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
