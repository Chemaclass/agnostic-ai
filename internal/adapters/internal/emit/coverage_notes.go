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

// pendingSurfaceNote is a coverage note for a spec that emits in full
// and then lands differently across the target's own execution
// surfaces: the file is written, one surface honors it, another
// ignores it. Kept apart from pendingFieldNote, whose subject is inert
// everywhere on the target, so a note about one surface never reads as
// the whole target dropping the spec.
type pendingSurfaceNote struct {
	target string
	kind   spec.Kind
	count  int
	// surface is the vendor's own name for the surface that ignores the
	// spec, e.g. "Copilot cloud agent".
	surface string
	// reason is a short, user-facing phrase explaining the split and how
	// to land on the other side of it.
	reason string
}

var coverageNoteState struct {
	mu             sync.Mutex
	pending        []pendingNote
	pendingField   []pendingFieldNote
	pendingSurface []pendingSurfaceNote
	// pendingText holds notes about the project as a whole rather than
	// one target's emission, e.g. two targets' outputs overlapping in a
	// tree a third vendor reads. Already a full sentence; no grouping.
	pendingText []string
	// environmentFields holds, per target, the environment fields its
	// specs set, so a field one target ignores and another reads is not
	// reported as having no effect.
	environmentFields map[string]map[string]bool
	// omitted holds the specs a target's output left out whole, which
	// outlive a flush so the list of what each tool reads skips them.
	omitted map[omittedEntry]bool
}

type omittedEntry struct {
	target string
	kind   spec.Kind
	name   string
}

// NoteEntryOmitted records that target's output leaves out the spec of
// kind named name, so the list of what target reads does not name it.
// Call it next to the coverage note that says why.
func NoteEntryOmitted(target string, kind spec.Kind, name string) {
	coverageNoteState.mu.Lock()
	if coverageNoteState.omitted == nil {
		coverageNoteState.omitted = map[omittedEntry]bool{}
	}
	coverageNoteState.omitted[omittedEntry{target, kind, name}] = true
	coverageNoteState.mu.Unlock()
}

// OmittedEntry reports whether target's output left out the spec of
// kind named name since the last ResetCoverageNotes.
func OmittedEntry(target string, kind spec.Kind, name string) bool {
	coverageNoteState.mu.Lock()
	defer coverageNoteState.mu.Unlock()
	return coverageNoteState.omitted[omittedEntry{target, kind, name}]
}

// RecordEnvironmentFields records the fields that target's environment
// specs set: each top-level field with a value, and each dev-commands
// entry field as `dev-commands.<field>`. A no-effect note on one target
// is dropped when another target got the field and noted nothing about
// it, since one shared spec feeds every tool its own part.
func RecordEnvironmentFields(target string, envs []spec.Entry) {
	if len(envs) == 0 {
		return
	}
	fields := map[string]bool{}
	for _, e := range envs {
		for k, v := range ResolveMeta(e.Meta, target) {
			if hasValue(v) {
				fields[k] = true
			}
		}
		cmds, _ := ResolveMeta(e.Meta, target)["dev-commands"].([]any)
		for _, c := range cmds {
			m, _ := c.(map[string]any)
			for k, v := range m {
				if hasValue(v) {
					fields["dev-commands."+k] = true
				}
			}
		}
	}
	coverageNoteState.mu.Lock()
	if coverageNoteState.environmentFields == nil {
		coverageNoteState.environmentFields = map[string]map[string]bool{}
	}
	coverageNoteState.environmentFields[target] = fields
	coverageNoteState.mu.Unlock()
}

// pruneFieldNotesLocked drops each environment field note whose field
// another target got and noted nothing about, so the digest, the count,
// and the flush all see the notes that print. Caller holds
// coverageNoteState.mu.
func pruneFieldNotesLocked() {
	if len(coverageNoteState.environmentFields) == 0 {
		return
	}
	noted := map[string]bool{}
	for _, p := range coverageNoteState.pendingField {
		if p.kind == spec.KindEnvironment {
			noted[p.target+"\x00"+p.field] = true
		}
	}
	// A note on `dev-commands` as a whole covers each of its fields.
	notedBy := func(t, field string) bool {
		parent, _, _ := strings.Cut(field, ".")
		return noted[t+"\x00"+field] || noted[t+"\x00"+parent]
	}
	readElsewhere := func(p pendingFieldNote) bool {
		for t, fields := range coverageNoteState.environmentFields {
			if t != p.target && fields[p.field] && !notedBy(t, p.field) {
				return true
			}
		}
		return false
	}
	kept := coverageNoteState.pendingField[:0]
	for _, p := range coverageNoteState.pendingField {
		if p.kind != spec.KindEnvironment || !readElsewhere(p) {
			kept = append(kept, p)
		}
	}
	coverageNoteState.pendingField = kept
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

// NoteSurfaceGap records that count specs of kind emit to target and
// then run on some of its surfaces but not surface. Use this instead of
// NoteFieldNoOp when the target does honor the spec somewhere: a
// field-no-op note claims the whole target ignores it, which would be
// false. reason is a short, user-facing phrase, e.g. "a cloud agent job
// honors bash and command entries only". No-op when count is zero.
// Notes buffer until FlushCoverageNotes renders them.
func NoteSurfaceGap(target string, kind spec.Kind, count int, surface, reason string) {
	if count <= 0 {
		return
	}
	coverageNoteState.mu.Lock()
	coverageNoteState.pendingSurface = append(coverageNoteState.pendingSurface, pendingSurfaceNote{
		target: target, kind: kind, count: count, surface: surface, reason: reason,
	})
	coverageNoteState.mu.Unlock()
}

// NoteProject records a project-wide note that belongs to no single
// target, so none of the per-target shapes above fit. text is the full
// sentence after the `note: ` prefix. Buffering it here, instead of
// printing at the call site, gives it the same unchanged-since-last-sync
// suppression as every other note. No-op on empty text.
func NoteProject(text string) {
	if text == "" {
		return
	}
	coverageNoteState.mu.Lock()
	coverageNoteState.pendingText = append(coverageNoteState.pendingText, text)
	coverageNoteState.mu.Unlock()
}

// FlushCoverageNotes prints one line per buffered coverage gap (whole
// entries that reach a target only via a hint), one line per buffered
// field no-op (entries that reach a target in full, minus one inert
// attribute), and one line per buffered surface gap (entries that reach
// a target but not every surface it runs on), then clears all three
// buffers. Safe to call when empty.
func FlushCoverageNotes() {
	coverageNoteState.mu.Lock()
	defer coverageNoteState.mu.Unlock()
	flushGapNotesLocked()
	flushFieldNotesLocked()
	flushSurfaceNotesLocked()
	flushProjectNotesLocked()
}

// flushProjectNotesLocked prints each distinct project note once, in
// the order recorded. Caller holds coverageNoteState.mu.
func flushProjectNotesLocked() {
	seen := map[string]bool{}
	for _, text := range coverageNoteState.pendingText {
		if seen[text] {
			continue
		}
		seen[text] = true
		_, _ = fmt.Fprintf(Warner, "  note: %s\n", text)
	}
	coverageNoteState.pendingText = nil
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
		_, _ = fmt.Fprintf(Warner, "  note: %s\n", gapNoteText(k.kind, k.count, k.via, targets))
	}
	coverageNoteState.pending = nil
}

// flushFieldNotesLocked prints one line per (kind, field, count, reason)
// group across all buffered targets. Deliberately a different sentence
// shape from flushGapNotesLocked: "field has no effect on target", never
// "reaches target only ...", since the entry itself did reach the target.
// Caller holds coverageNoteState.mu.
func flushFieldNotesLocked() {
	pruneFieldNotesLocked()
	if len(coverageNoteState.pendingField) == 0 {
		coverageNoteState.environmentFields = nil
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
		_, _ = fmt.Fprintf(Warner, "  note: %s (%s)\n", fieldNoteText(k.kind, k.field, k.count, targets), k.reason)
	}
	coverageNoteState.pendingField = nil
	coverageNoteState.environmentFields = nil
}

// flushSurfaceNotesLocked prints one line per (kind, count, surface,
// reason) group across all buffered targets. A third sentence shape
// again: the entry reached the target, and every field on it is live,
// so neither of the other two sentences would be true. Caller holds
// coverageNoteState.mu.
func flushSurfaceNotesLocked() {
	if len(coverageNoteState.pendingSurface) == 0 {
		return
	}
	type key struct {
		kind    spec.Kind
		count   int
		surface string
		reason  string
	}
	order := []key{}
	groups := map[key][]string{}
	seen := map[string]bool{} // target+kind+surface+reason dedup within one flush
	for _, p := range coverageNoteState.pendingSurface {
		dedupKey := p.target + "\x00" + string(p.kind) + "\x00" + p.surface + "\x00" + p.reason
		if seen[dedupKey] {
			continue
		}
		seen[dedupKey] = true
		k := key{p.kind, p.count, p.surface, p.reason}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], p.target)
	}
	for _, k := range order {
		_, _ = fmt.Fprintf(Warner, "  note: %s (%s)\n", surfaceNoteText(k.kind, k.count, k.surface, groups[k]), k.reason)
	}
	coverageNoteState.pendingSurface = nil
}

// gapNoteText is the sentence for a coverage gap. A config-key hint
// reads as "only via outputs.x.y"; a free-text reason (no materialize
// key) reads as "only in the source dir (reason)".
func gapNoteText(kind spec.Kind, count int, via string, targets []string) string {
	tail := "via " + via
	if !strings.HasPrefix(via, "outputs.") {
		tail = "in the source dir (" + via + ")"
	}
	return fmt.Sprintf("%d %s %s %s only %s", count, pluralizeKind(kind, count), reachVerb(count), strings.Join(targets, ", "), tail)
}

// fieldNoteText is the sentence for a field no-op, before its reason.
func fieldNoteText(kind spec.Kind, field string, count int, targets []string) string {
	return fmt.Sprintf("`%s` on %d %s has no effect on %s", field, count, pluralizeKind(kind, count), strings.Join(targets, ", "))
}

// surfaceNoteText is the sentence for a surface gap, before its reason.
func surfaceNoteText(kind spec.Kind, count int, surface string, targets []string) string {
	return fmt.Sprintf("%d %s %s %s but not %s", count, pluralizeKind(kind, count), reachVerb(count), strings.Join(targets, ", "), surface)
}

// reachVerb agrees the verb with the subject count: "reaches" for a
// single spec, "reach" for many.
func reachVerb(n int) string {
	if n == 1 {
		return "reaches"
	}
	return "reach"
}

// ResetCoverageNotes clears buffered coverage gaps, field no-ops,
// surface gaps, project notes, and omitted entries without printing.
// Used by tests and by `sync --watch` between runs.
func ResetCoverageNotes() {
	coverageNoteState.mu.Lock()
	discardCoverageNotesLocked()
	coverageNoteState.environmentFields = nil
	coverageNoteState.omitted = nil
	coverageNoteState.mu.Unlock()
}

// DiscardCoverageNotes clears the buffered notes without printing, as a
// flush would, and keeps the omitted entries for the list of what each
// tool reads.
func DiscardCoverageNotes() {
	coverageNoteState.mu.Lock()
	discardCoverageNotesLocked()
	coverageNoteState.mu.Unlock()
}

func discardCoverageNotesLocked() {
	coverageNoteState.pending = nil
	coverageNoteState.pendingField = nil
	coverageNoteState.pendingSurface = nil
	coverageNoteState.pendingText = nil
}

// CoverageNotesDigest returns a stable hex digest of the buffered
// coverage gaps, field no-ops, and surface gaps, suitable for comparing
// across sync runs to suppress unchanged repeats. Returns "" when
// nothing is pending.
func CoverageNotesDigest() string {
	coverageNoteState.mu.Lock()
	defer coverageNoteState.mu.Unlock()
	pruneFieldNotesLocked()
	if len(coverageNoteState.pending) == 0 && len(coverageNoteState.pendingField) == 0 &&
		len(coverageNoteState.pendingSurface) == 0 && len(coverageNoteState.pendingText) == 0 {
		return ""
	}
	seen := map[string]bool{}
	keys := make([]string, 0, len(coverageNoteState.pending)+
		len(coverageNoteState.pendingField)+len(coverageNoteState.pendingSurface)+
		len(coverageNoteState.pendingText))
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
	for _, p := range coverageNoteState.pendingSurface {
		k := fmt.Sprintf("surface\x00%s\x00%s\x00%s\x00%d\x00%s", p.target, p.kind, p.surface, p.count, p.reason)
		if seen[k] {
			continue
		}
		seen[k] = true
		keys = append(keys, k)
	}
	for _, text := range coverageNoteState.pendingText {
		k := "project\x00" + text
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
// (target, kind, via), field-no-op (target, kind, field, reason), and
// surface-gap (target, kind, surface, reason) notes are currently
// buffered. Used to size the suppression notice when
// sticky-suppressing unchanged repeats.
func PendingCoverageNotesCount() int {
	coverageNoteState.mu.Lock()
	defer coverageNoteState.mu.Unlock()
	pruneFieldNotesLocked()
	seen := targetNoteKeysLocked()
	for _, text := range coverageNoteState.pendingText {
		seen["project\x00"+text] = true
	}
	return len(seen)
}

// targetNoteKeysLocked returns one key per distinct coverage gap
// (target, kind, via), field no-op (target, kind, field, reason), and
// surface gap (target, kind, surface, reason). Caller holds
// coverageNoteState.mu.
func targetNoteKeysLocked() map[string]bool {
	seen := map[string]bool{}
	for _, p := range coverageNoteState.pending {
		seen["gap\x00"+p.target+"\x00"+string(p.kind)+"\x00"+p.via] = true
	}
	for _, p := range coverageNoteState.pendingField {
		seen["field\x00"+p.target+"\x00"+string(p.kind)+"\x00"+p.field+"\x00"+p.reason] = true
	}
	for _, p := range coverageNoteState.pendingSurface {
		seen["surface\x00"+p.target+"\x00"+string(p.kind)+"\x00"+p.surface+"\x00"+p.reason] = true
	}
	return seen
}
