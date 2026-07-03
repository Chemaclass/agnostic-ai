package emit

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// KindDrop is one kind a target could not fully emit: Count specs of Kind,
// with Via naming the opt-in key (or reason) that would carry them when the
// drop is a downgrade rather than an outright capability gap.
type KindDrop struct {
	Kind  spec.Kind
	Count int
	Via   string
}

// TargetDrop summarizes, for one target, what the last run could not fully
// emit: Unsupported kinds (no native surface, dropped entirely) and
// Downgraded kinds (reach the target only behind an opt-in key or stay
// source-dir only). Built from the same buffers FlushCapabilityWarnings and
// FlushCoverageNotes consume.
type TargetDrop struct {
	Target      string
	Unsupported []KindDrop
	Downgraded  []KindDrop
}

// DroppedByTarget regroups the buffered capability warnings (unsupported)
// and coverage notes (downgraded) by target, sorted by target name and by
// kind within each. Read-only: it does not clear either buffer, so the
// existing kind-grouped flushes still render afterward. Returns nil when
// nothing is buffered.
func DroppedByTarget() []TargetDrop {
	capabilityWarnState.mu.Lock()
	warns := append([]pendingWarn(nil), capabilityWarnState.pending...)
	capabilityWarnState.mu.Unlock()

	coverageNoteState.mu.Lock()
	notes := append([]pendingNote(nil), coverageNoteState.pending...)
	coverageNoteState.mu.Unlock()

	byTarget := map[string]*TargetDrop{}
	get := func(target string) *TargetDrop {
		if d, ok := byTarget[target]; ok {
			return d
		}
		d := &TargetDrop{Target: target}
		byTarget[target] = d
		return d
	}

	seenWarn := map[string]bool{}
	for _, w := range warns {
		key := w.target + "\x00" + string(w.kind)
		if seenWarn[key] {
			continue
		}
		seenWarn[key] = true
		d := get(w.target)
		d.Unsupported = append(d.Unsupported, KindDrop{Kind: w.kind, Count: w.count})
	}
	seenNote := map[string]bool{}
	for _, n := range notes {
		key := n.target + "\x00" + string(n.kind) + "\x00" + n.via
		if seenNote[key] {
			continue
		}
		seenNote[key] = true
		d := get(n.target)
		d.Downgraded = append(d.Downgraded, KindDrop{Kind: n.kind, Count: n.count, Via: n.via})
	}

	if len(byTarget) == 0 {
		return nil
	}
	out := make([]TargetDrop, 0, len(byTarget))
	for _, d := range byTarget {
		sortKindDrops(d.Unsupported)
		sortKindDrops(d.Downgraded)
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Target < out[j].Target })
	return out
}

func sortKindDrops(ds []KindDrop) {
	sort.Slice(ds, func(i, j int) bool {
		if ds[i].Kind != ds[j].Kind {
			return ds[i].Kind < ds[j].Kind
		}
		return ds[i].Via < ds[j].Via
	})
}

// RenderDroppedSummary writes a concise per-target block of what each
// target could not fully emit. No-op (writes nothing) when no drops are
// buffered. The phrasing mirrors the kind-grouped warnings: unsupported
// kinds are "dropped", downgraded kinds name the key that would carry them.
func RenderDroppedSummary(w io.Writer) {
	drops := DroppedByTarget()
	if len(drops) == 0 {
		return
	}
	_, _ = fmt.Fprintln(w, "  dropped summary (per target):")
	for _, d := range drops {
		var parts []string
		for _, k := range d.Unsupported {
			parts = append(parts, fmt.Sprintf("%d %s dropped (unsupported)", k.Count, pluralizeKind(k.Kind, k.Count)))
		}
		for _, k := range d.Downgraded {
			// Mirror FlushCoverageNotes: a config-key hint reads "via
			// outputs.x.y"; a free-text reason reads "in the source dir (…)".
			tail := "via " + k.Via
			if !strings.HasPrefix(k.Via, "outputs.") {
				tail = "in the source dir (" + k.Via + ")"
			}
			parts = append(parts, fmt.Sprintf("%d %s %s", k.Count, pluralizeKind(k.Kind, k.Count), tail))
		}
		_, _ = fmt.Fprintf(w, "    %s: %s\n", d.Target, strings.Join(parts, "; "))
	}
}
