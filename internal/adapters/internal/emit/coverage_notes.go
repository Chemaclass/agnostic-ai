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

// pendingFieldNote is a coverage note for one attribute on a spec whose
// target has no home for it: the entry itself still emits in full, only
// this field goes in inert (the target's file format has no key for it,
// or a same-named key means something unrelated there). Kept as a
// distinct case from pendingNote, whose subject never reaches the target
// at all, so the rendered sentence never implies the entry itself is
// missing when only one of its fields is.
type pendingFieldNote struct {
	target string
	kind   spec.Kind
	field  string
	count  int
	// reason is a short, user-facing phrase explaining why the field has
	// no effect, e.g. "no file-based way to pre-disable a project-scoped
	// MCP server".
	reason string
}

var coverageNoteState struct {
	mu           sync.Mutex
	pending      []pendingNote
	pendingField []pendingFieldNote
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

// NoteFieldNoOp records that count specs of kind reach target in full,
// but field has no effect once they land there: the target's config
// format has no key for it, or a same-named key means something
// unrelated. Use this instead of NoteCoverageGap when the entry itself is
// not downgraded and only one of its attributes is silently inert — the
// two render as different sentences so neither implies the other's
// failure mode. reason is a short, user-facing phrase, e.g. "no
// file-based way to pre-disable a project-scoped MCP server". No-op when
// count is zero. Notes buffer until FlushCoverageNotes renders them.
func NoteFieldNoOp(target string, kind spec.Kind, field string, count int, reason string) {
	if count <= 0 {
		return
	}
	coverageNoteState.mu.Lock()
	coverageNoteState.pendingField = append(coverageNoteState.pendingField, pendingFieldNote{
		target: target, kind: kind, field: field, count: count, reason: reason,
	})
	coverageNoteState.mu.Unlock()
}

// FlushCoverageNotes prints one line per buffered coverage gap (whole
// entries that reach a target only via a hint) and one line per buffered
// field no-op (entries that reach a target in full, minus one inert
// attribute), then clears both buffers. Safe to call when empty.
func FlushCoverageNotes() {
	coverageNoteState.mu.Lock()
	defer coverageNoteState.mu.Unlock()
	flushGapNotesLocked()
	flushFieldNotesLocked()
}

// flushGapNotesLocked prints one line per (kind, count, via) group across
// all buffered targets. Targets that share the same (kind, count, via)
// join on one line. Caller holds coverageNoteState.mu.
func flushGapNotesLocked() {
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

// flushFieldNotesLocked prints one line per (kind, field, count, reason)
// group across all buffered targets. Deliberately a different sentence
// shape from flushGapNotesLocked: "field has no effect on target", never
// "reaches target only ...", since the entry itself did reach the target.
// Caller holds coverageNoteState.mu.
func flushFieldNotesLocked() {
	if len(coverageNoteState.pendingField) == 0 {
		return
	}
	type key struct {
		kind   spec.Kind
		field  string
		count  int
		reason string
	}
	order := []key{}
	groups := map[key][]string{}
	seen := map[string]bool{} // target+kind+field+reason dedup within one flush
	for _, p := range coverageNoteState.pendingField {
		dedupKey := p.target + "\x00" + string(p.kind) + "\x00" + p.field + "\x00" + p.reason
		if seen[dedupKey] {
			continue
		}
		seen[dedupKey] = true
		k := key{p.kind, p.field, p.count, p.reason}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], p.target)
	}
	for _, k := range order {
		targets := groups[k]
		_, _ = fmt.Fprintf(Warner, "  note: `%s` on %d %s has no effect on %s (%s)\n",
			k.field, k.count, pluralizeKind(k.kind, k.count), strings.Join(targets, ", "), k.reason)
	}
	coverageNoteState.pendingField = nil
}

// reachVerb agrees the verb with the subject count: "reaches" for a
// single spec, "reach" for many.
func reachVerb(n int) string {
	if n == 1 {
		return "reaches"
	}
	return "reach"
}

// ResetCoverageNotes clears buffered coverage gaps and field no-ops
// without printing. Used by tests and by `sync --watch` between runs.
func ResetCoverageNotes() {
	coverageNoteState.mu.Lock()
	coverageNoteState.pending = nil
	coverageNoteState.pendingField = nil
	coverageNoteState.mu.Unlock()
}

// CoverageNotesDigest returns a stable hex digest of the buffered
// coverage gaps and field no-ops, suitable for comparing across sync
// runs to suppress unchanged repeats. Returns "" when nothing is
// pending.
func CoverageNotesDigest() string {
	coverageNoteState.mu.Lock()
	defer coverageNoteState.mu.Unlock()
	if len(coverageNoteState.pending) == 0 && len(coverageNoteState.pendingField) == 0 {
		return ""
	}
	seen := map[string]bool{}
	keys := make([]string, 0, len(coverageNoteState.pending)+len(coverageNoteState.pendingField))
	for _, p := range coverageNoteState.pending {
		k := fmt.Sprintf("gap\x00%s\x00%s\x00%d\x00%s", p.target, p.kind, p.count, p.via)
		if seen[k] {
			continue
		}
		seen[k] = true
		keys = append(keys, k)
	}
	for _, p := range coverageNoteState.pendingField {
		k := fmt.Sprintf("field\x00%s\x00%s\x00%s\x00%d\x00%s", p.target, p.kind, p.field, p.count, p.reason)
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

// PendingCoverageNotesCount returns how many distinct coverage-gap
// (target, kind, via) and field-no-op (target, kind, field, reason)
// notes are currently buffered. Used to size the suppression notice when
// sticky-suppressing unchanged repeats.
func PendingCoverageNotesCount() int {
	coverageNoteState.mu.Lock()
	defer coverageNoteState.mu.Unlock()
	seen := map[string]bool{}
	for _, p := range coverageNoteState.pending {
		seen["gap\x00"+p.target+"\x00"+string(p.kind)+"\x00"+p.via] = true
	}
	for _, p := range coverageNoteState.pendingField {
		seen["field\x00"+p.target+"\x00"+string(p.kind)+"\x00"+p.field+"\x00"+p.reason] = true
	}
	return len(seen)
}
