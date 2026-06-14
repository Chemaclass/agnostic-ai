package emit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Coverage notes surface "opt-in gaps": a spec kind an adapter declares
// support for (so no "unsupported" warning fires) but does not emit by
// default. The content only materializes behind an `outputs.<target>.*`
// key, or stays source-dir only. Without a note these kinds are silently
// skipped and the user has no signal that content did not reach a target.
//
// The store mirrors the capability-warning buffer: adapters call
// NoteCoverageGap at their skip site, sync flushes once via
// FlushCoverageNotes, and a digest feeds the same sticky-suppression as
// the capability warnings so unchanged notes do not repeat across runs.

type pendingNote struct {
	target string
	kind   spec.Kind
	count  int
	// via is the user-facing hint: the config key to set so the content
	// reaches the target, or a phrase like "source-dir only" when there
	// is no key.
	via string
}

var coverageNoteState struct {
	mu      sync.Mutex
	pending []pendingNote
}

// NoteCoverageGap records that count specs of kind reach target only via
// the hint `via` (a config key such as
// `outputs.gemini.emit-skills-as-commands`, or a phrase like
// "source-dir only"). No-op when count is zero so a note never fires for
// an absent kind. Notes buffer until FlushCoverageNotes renders them.
func NoteCoverageGap(target string, kind spec.Kind, count int, via string) {
	if count <= 0 {
		return
	}
	coverageNoteState.mu.Lock()
	coverageNoteState.pending = append(coverageNoteState.pending, pendingNote{
		target: target, kind: kind, count: count, via: via,
	})
	coverageNoteState.mu.Unlock()
}

// FlushCoverageNotes prints one line per (kind, count, via) group across
// all buffered targets and clears the buffer. Targets that share the same
// (kind, count, via) join on one line. Safe to call when empty.
func FlushCoverageNotes() {
	coverageNoteState.mu.Lock()
	defer coverageNoteState.mu.Unlock()
	if len(coverageNoteState.pending) == 0 {
		return
	}
	type key struct {
		kind  spec.Kind
		count int
		via   string
	}
	order := []key{}
	groups := map[key][]string{}
	seen := map[string]bool{} // target+kind+via dedup within one flush
	for _, p := range coverageNoteState.pending {
		dedupKey := p.target + "\x00" + string(p.kind) + "\x00" + p.via
		if seen[dedupKey] {
			continue
		}
		seen[dedupKey] = true
		k := key{p.kind, p.count, p.via}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], p.target)
	}
	for _, k := range order {
		targets := groups[k]
		// A config-key hint reads as "only via outputs.x.y"; a free-text
		// reason (no materialize key) reads as "only in the source dir (reason)".
		tail := "via " + k.via
		if !strings.HasPrefix(k.via, "outputs.") {
			tail = "in the source dir (" + k.via + ")"
		}
		_, _ = fmt.Fprintf(Warner, "  note: %d %s %s %s only %s\n",
			k.count, pluralizeKind(k.kind, k.count), reachVerb(k.count),
			strings.Join(targets, ", "), tail)
	}
	coverageNoteState.pending = nil
}

// reachVerb agrees the verb with the subject count: "reaches" for a
// single spec, "reach" for many.
func reachVerb(n int) string {
	if n == 1 {
		return "reaches"
	}
	return "reach"
}

// ResetCoverageNotes clears buffered notes without printing. Used by
// tests and by `sync --watch` between runs.
func ResetCoverageNotes() {
	coverageNoteState.mu.Lock()
	coverageNoteState.pending = nil
	coverageNoteState.mu.Unlock()
}

// CoverageNotesDigest returns a stable hex digest of the buffered notes,
// suitable for comparing across sync runs to suppress unchanged repeats.
// Returns "" when no notes are pending.
func CoverageNotesDigest() string {
	coverageNoteState.mu.Lock()
	defer coverageNoteState.mu.Unlock()
	if len(coverageNoteState.pending) == 0 {
		return ""
	}
	seen := map[string]bool{}
	keys := make([]string, 0, len(coverageNoteState.pending))
	for _, p := range coverageNoteState.pending {
		k := fmt.Sprintf("%s\x00%s\x00%d\x00%s", p.target, p.kind, p.count, p.via)
		if seen[k] {
			continue
		}
		seen[k] = true
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sum := sha256.Sum256([]byte(strings.Join(keys, "\n")))
	return hex.EncodeToString(sum[:])
}

// PendingCoverageNotesCount returns how many distinct (target, kind, via)
// notes are currently buffered. Used to size the suppression notice when
// sticky-suppressing unchanged repeats.
func PendingCoverageNotesCount() int {
	coverageNoteState.mu.Lock()
	defer coverageNoteState.mu.Unlock()
	seen := map[string]bool{}
	for _, p := range coverageNoteState.pending {
		seen[p.target+"\x00"+string(p.kind)+"\x00"+p.via] = true
	}
	return len(seen)
}
