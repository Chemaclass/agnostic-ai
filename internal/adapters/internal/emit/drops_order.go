package emit

import "sort"

// OrderBufferedDropsByTarget stable-sorts the buffered capability warnings
// and coverage notes so their flush output follows the given target order,
// regardless of the order adapters appended them in.
//
// Serial emission already appends in target order, so this is a no-op
// there. Parallel emission (sync --jobs > 1) interleaves appends across
// goroutines; re-sorting by target restores the deterministic sequence.
// The sort is stable, so each target keeps its own append order (kinds in
// spec.AllKinds order for warnings, skip-site order for notes) — which
// makes the result byte-identical to the serial buffer. Targets absent
// from order sort to the end, keeping their relative order.
//
// It reorders only: it does not dedupe or clear. The digests already sort
// internally, so this changes only the human-facing flush and per-target
// summary output, never the sticky-suppression fingerprints.
func OrderBufferedDropsByTarget(order []string) {
	rank := targetRanker(order)

	capabilityWarnState.mu.Lock()
	sort.SliceStable(capabilityWarnState.pending, func(i, j int) bool {
		return rank(capabilityWarnState.pending[i].target) < rank(capabilityWarnState.pending[j].target)
	})
	capabilityWarnState.mu.Unlock()

	coverageNoteState.mu.Lock()
	sort.SliceStable(coverageNoteState.pending, func(i, j int) bool {
		return rank(coverageNoteState.pending[i].target) < rank(coverageNoteState.pending[j].target)
	})
	coverageNoteState.mu.Unlock()
}

// targetRanker returns a function mapping a target name to its index in
// order, or len(order) when the target is not listed (sorts last).
func targetRanker(order []string) func(string) int {
	idx := make(map[string]int, len(order))
	for i, t := range order {
		if _, ok := idx[t]; !ok {
			idx[t] = i
		}
	}
	return func(target string) int {
		if i, ok := idx[target]; ok {
			return i
		}
		return len(order)
	}
}
