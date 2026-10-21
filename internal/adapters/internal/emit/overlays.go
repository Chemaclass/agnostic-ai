package emit

import (
	"path"
	"slices"
	"strings"
)

// KnownOverlay is one path under `.agnostic-ai/overlays/` that a target
// reads at sync. Name is relative to that directory, in slash form; a
// directory name ends in "/" and covers every file directly inside it.
type KnownOverlay struct {
	Target string
	Name   string
}

// Dir reports whether the overlay covers a directory of helper files.
func (o KnownOverlay) Dir() bool { return strings.HasSuffix(o.Name, "/") }

// Covers reports whether the overlay is the file at rel, or the helper
// directory rel sits directly in.
func (o KnownOverlay) Covers(rel string) bool {
	if !o.Dir() {
		return rel == o.Name
	}
	rest, inside := strings.CutPrefix(rel, o.Name)
	return inside && rest != "" && !strings.Contains(rest, "/")
}

// knownOverlays is filled at package init by every Overlay and
// NewHelperOverlays call, so a reader cannot use a path it did not
// register. Never written after init.
var knownOverlays []KnownOverlay

func registerOverlay(o KnownOverlay) {
	if !slices.Contains(knownOverlays, o) {
		knownOverlays = append(knownOverlays, o)
	}
}

// Overlay registers name as an overlay target reads and returns its
// project-relative path. Adapters call it in a package-level var, so
// the unread-overlay check sees every file sync reads.
func Overlay(target, name string) string {
	registerOverlay(KnownOverlay{Target: target, Name: name})
	return path.Join(agnosticOverlayDir, name)
}

// KnownOverlays returns every overlay a target reads, sorted by name.
func KnownOverlays() []KnownOverlay {
	out := slices.Clone(knownOverlays)
	slices.SortFunc(out, func(a, b KnownOverlay) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// HelperOverlays names the `.agnostic-ai/overlays/<tool>/` directory a
// target restores helper files from.
type HelperOverlays struct{ tool string }

// NewHelperOverlays registers `<tool>/` as an overlay directory tool
// reads.
func NewHelperOverlays(tool string) HelperOverlays {
	registerOverlay(KnownOverlay{Target: tool, Name: tool + "/"})
	return HelperOverlays{tool: tool}
}
