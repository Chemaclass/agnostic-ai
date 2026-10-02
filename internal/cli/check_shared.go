package cli

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

// foldSharedDrift lists each drifted path under the first report that holds
// it with the same content. Several targets read one file, such as .agents/skills/<name>/SKILL.md,
// so a per-target listing repeats it. Later targets go into the first
// report's sharedWith, and a report left with no drift is dropped. The
// printers use the result; JSON and --fix keep the per-target reports.
func foldSharedDrift(reports []driftReport) []driftReport {
	owner := map[string]int{}
	var out []driftReport
	for _, r := range reports {
		if !r.hasDrift() {
			out = append(out, r)
			continue
		}
		idx := len(out)
		// variant tells apart renderings of one path that differ, which
		// sync.collision-policy: prefer-spec allows, so each stays visible.
		claim := func(path, variant string) bool {
			key := filepath.ToSlash(filepath.Clean(path))
			if i, ok := owner[key+"\x00"+variant]; ok {
				if i == idx {
					return false
				}
				if out[i].sharedWith == nil {
					out[i].sharedWith = map[string][]string{}
				}
				out[i].sharedWith[key] = append(out[i].sharedWith[key], r.Target)
				return false
			}
			owner[key+"\x00"+variant] = idx
			return true
		}
		files := func(in []adapters.CapturedFile) []adapters.CapturedFile {
			var kept []adapters.CapturedFile
			for _, f := range in {
				if claim(f.Path, f.Content) {
					kept = append(kept, f)
				}
			}
			return kept
		}
		paths := func(in []string) []string {
			var kept []string
			for _, p := range in {
				if claim(p, "") {
					kept = append(kept, p)
				}
			}
			return kept
		}
		folded := r
		folded.sharedWith = nil
		folded.Missing = files(r.Missing)
		folded.Stale = files(r.Stale)
		folded.Edited = files(r.Edited)
		folded.Orphaned = paths(r.Orphaned)
		folded.Leftover = paths(r.Leftover)
		folded.Unmanaged = nil
		for _, f := range r.Unmanaged {
			if claim(f.Path, "") {
				folded.Unmanaged = append(folded.Unmanaged, f)
			}
		}
		if folded.hasDrift() {
			out = append(out, folded)
		}
	}
	return out
}

// sharedNote names the other targets that read path, or "" when only this
// report's target does.
func (r driftReport) sharedNote(path string) string {
	others := r.sharedWith[filepath.ToSlash(filepath.Clean(path))]
	if len(others) == 0 {
		return ""
	}
	return " (shared with " + strings.Join(others, ", ") + ")"
}

// label is path in slash form, followed by the other targets that read it.
func (r driftReport) label(path string) string {
	return filepath.ToSlash(path) + r.sharedNote(path)
}
