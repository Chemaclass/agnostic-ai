package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
)

// warnUncapturedEntryPoints prints a warning for every target root
// entry-point file that exists in the project, is hand-authored (no
// agnostic-ai header), and holds content different from the body just
// captured into AGNOSTIC_AI.md from mirroredSrc.
//
// import mirrors a single source entry-point into the shared body. A
// sibling entry-point with unique content (e.g. a project keeping both a
// rich CLAUDE.md and a distinct AGENTS.md) would otherwise be overwritten
// by the next sync with no notice, silently discarding real instructions
// (#415). Surfacing it lets the user merge that content into
// AGNOSTIC_AI.md before syncing.
//
// Generated siblings (carrying the agnostic-ai header) are skipped: sync
// rewrites them with the same body, so nothing is lost. Under `import
// all` the root AGENTS.md is skipped too: foldRootAgentsMainFile merges
// it once every importer has run.
func warnUncapturedEntryPoints(root, capturedBody string, mirroredSrcs ...string) {
	for _, rel := range adapters.ConventionalEntryPointPaths() {
		if slices.Contains(mirroredSrcs, rel) || rel == claudeAgentsMainFile && importingAll() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			continue
		}
		if uncapturedEntryBody(root, rel, capturedBody, string(data)) == "" {
			continue
		}
		summaryf("  ! %s has unique content not captured by this import — sync will overwrite it with the shared body. Merge it into %s first to keep it.\n",
			rel, agnosticMainFile)
	}
}

func uncapturedEntryBody(root, rel, capturedBody, raw string) string {
	if header.Has(raw) {
		return ""
	}
	want := strings.TrimSpace(capturedBody)
	body := strings.TrimSpace(adapters.StripGeneratedAppendices(raw))
	if body == "" || body == want || rel == claudeMainFile && adapters.IsAgentsCompanion(body) {
		return ""
	}
	if strings.Contains(want, "::target") && matchesRenderedView(root, rel, capturedBody, body) {
		return ""
	}
	return body
}

func importingAll() bool {
	return importAllSkippedEntryFiles != nil
}

// foldRootAgentsMainFile appends to AGNOSTIC_AI.md the sections of a
// hand-written root AGENTS.md that it does not hold yet, seeding it when
// absent, and reports whether it wrote. `import all` never detects codex
// from AGENTS.md alone, since many tools read that file, so without this
// its text would be left for sync to overwrite.
func foldRootAgentsMainFile(root string) (bool, error) {
	data, err := readEntryFile(root, filepath.Join(root, claudeAgentsMainFile))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", claudeAgentsMainFile, err)
	}
	if header.Has(string(data)) {
		return false, nil
	}
	dst := filepath.Join(root, agnosticMainFile)
	existing, err := os.ReadFile(dst)
	if errors.Is(err, fs.ErrNotExist) {
		result, err := mirrorMainFile(root, claudeAgentsMainFile)
		if result == mirrorWritten {
			summaryf("  → %s seeded from %s\n", agnosticMainFile, claudeAgentsMainFile)
		}
		return result == mirrorWritten, err
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", dst, err)
	}
	captured := string(existing)
	body := uncapturedEntryBody(root, claudeAgentsMainFile, captured, string(data))
	if body == "" {
		return false, nil
	}
	var added, titles []string
	have := collapseSpace(captured)
	for _, section := range markdownH2Sections(body) {
		if strings.Contains(have, collapseSpace(section)) {
			continue
		}
		added = append(added, section)
		titles = append(titles, fmt.Sprintf("%q", sectionTitle(section)))
	}
	if len(added) == 0 {
		return false, nil
	}
	merged := strings.TrimRight(captured, "\n") + "\n\n" + strings.Join(added, "\n\n") + "\n"
	if err := importWriteFile(dst, []byte(merged), 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", dst, err)
	}
	noun := "sections"
	if len(added) == 1 {
		noun = "section"
	}
	summaryf("  → merged %d %s from %s into %s: %s\n",
		len(added), noun, claudeAgentsMainFile, agnosticMainFile, strings.Join(titles, ", "))
	return true, nil
}

func markdownH2Sections(body string) []string {
	var sections []string
	var current []string
	flush := func() {
		if s := strings.TrimSpace(strings.Join(current, "\n")); s != "" {
			sections = append(sections, s)
		}
		current = nil
	}
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		if fenceRE.MatchString(line) {
			inFence = !inFence
		} else if !inFence && h2HeadingRE.MatchString(line) {
			flush()
		}
		current = append(current, line)
	}
	flush()
	return sections
}

func sectionTitle(section string) string {
	first, _, _ := strings.Cut(section, "\n")
	return strings.TrimSpace(strings.TrimLeft(first, "#"))
}

// collapseSpace reduces every run of whitespace to one space, so a section
// that differs from the body only in line wrapping still counts as held.
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
