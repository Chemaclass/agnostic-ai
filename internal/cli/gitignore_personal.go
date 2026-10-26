package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/gitquery"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	personalExcludeStart = "# >>> agnostic-ai (personal) "
	personalExcludeEnd   = "# <<< agnostic-ai (personal) "
	personalExcludeNote  = "# Outputs only " + config.LocalOverrideFileName + " adds. Run `agnostic-ai sync` to refresh."
)

// personalSplitBlock is splitManagedBlock with the team's built-ins read
// from the shared config when agnostic-ai.local.yaml exists.
func personalSplitBlock(root string, cfg *config.Config, b spec.Bundle, targets, recorded []string) (block, personal []string, err error) {
	team := cfg.Builtins
	if fileExists(filepath.Join(root, config.LocalOverrideFileName)) {
		if team, err = config.TeamBuiltins(root); err != nil {
			return nil, nil, err
		}
	}
	return splitManagedBlock(root, cfg, b, team, previousPersonal(root), targets, recorded)
}

// previousPersonal returns the outputs the project's block in
// info/exclude lists.
func previousPersonal(root string) []string {
	ex, ok := personalExcludeFile(root)
	if !ok {
		return nil
	}
	data, err := os.ReadFile(ex.path)
	if err != nil {
		return nil
	}
	return personalPatternsOf(string(data), personalExcludeKey(root), ex.prefix)
}

// splitManagedBlock returns the managed block a sync with the team's
// built-ins writes, and the outputs only the personal built-ins add that
// the block leaves unignored. Outputs the personal config turns off stay
// in the block, so agnostic-ai.local.yaml never changes a committed file.
// A recorded path the last sync excluded as personal (prev) stays
// personal while the team does not write it, as an edited output
// --keep-edits leaves behind does.
func splitManagedBlock(root string, cfg *config.Config, b spec.Bundle, team, prev, targets, recorded []string) (block, personal []string, err error) {
	same := sameBuiltins(team, cfg.Builtins)
	if same && len(prev) == 0 {
		block, err := syncManagedBlock(root, cfg, b, targets, recorded)
		return block, nil, err
	}
	teamCfg, teamBundle := cfg, b
	if !same {
		copied := *cfg
		teamCfg = &copied
		teamCfg.Builtins = slices.Clone(team)
		layers, err := resolveLayers(root, teamCfg)
		if err != nil {
			return nil, nil, err
		}
		if teamBundle, err = spec.LoadLayered(layers); err != nil {
			return nil, nil, err
		}
		teamBundle.ApplyModelTiers(teamCfg.Models)
	}
	mine := ignoreCandidates(cfg, b)
	theirs := mine
	if !same {
		theirs = ignoreCandidates(teamCfg, teamBundle)
	}
	only := map[string]bool{}
	for e := range mine {
		if !theirs[e] {
			only[e] = true
		}
	}
	for _, e := range recorded {
		if n := normalizeGitignorePath(e); slices.Contains(prev, n) && !theirs[n] {
			only[n] = true
		}
	}
	var entries []string
	for _, e := range recorded {
		if !only[normalizeGitignorePath(e)] {
			entries = append(entries, e)
		}
	}
	for e := range theirs {
		if !mine[e] {
			entries = append(entries, e)
		}
	}
	block, err = syncManagedBlock(root, teamCfg, teamBundle, targets, entries)
	if err != nil {
		return nil, nil, err
	}
	for e := range only {
		if !blockCovers(block, e) {
			personal = append(personal, e)
		}
	}
	slices.Sort(personal)
	return block, personal, nil
}

func sameBuiltins(a, b []string) bool {
	return slices.Equal(slices.Compact(slices.Sorted(slices.Values(a))), slices.Compact(slices.Sorted(slices.Values(b))))
}

// ignoreCandidates returns the root-anchored paths a sync of every
// configured target writes or asks to ignore.
func ignoreCandidates(cfg *config.Config, b spec.Bundle) map[string]bool {
	var paths []string
	for _, p := range append(renderedOutputs(cfg, b, cfg.Targets), gitignoreHintsForTargets(cfg, cfg.Targets)...) {
		if !filepath.IsAbs(p) && !cfg.IsUnmanaged(p) {
			paths = append(paths, p)
		}
	}
	out := map[string]bool{}
	for _, p := range normalizeAndSort(dropTrackedEntries(paths)) {
		out[p] = true
	}
	return out
}

// blockCovers reports whether a line of the managed block ignores entry.
func blockCovers(block []string, entry string) bool {
	for _, line := range block {
		if line == entry || strings.HasSuffix(line, "/") && strings.HasPrefix(entry, line) {
			return true
		}
	}
	return false
}

// gitExclude is the repository's info/exclude, in the common Git
// directory, and the project's path inside its work tree, which anchors
// the patterns. A linked worktree shares the file with every checkout.
type gitExclude struct {
	path, prefix string
	linked       bool
}

// personalExcludeFile returns the info/exclude for the project at root;
// ok is false outside a work tree.
func personalExcludeFile(root string) (gitExclude, bool) {
	layout, err := gitquery.LayoutAt(root)
	if err != nil || !layout.HasToplevel || layout.CommonDir == "" {
		return gitExclude{}, false
	}
	return gitExclude{
		path:   filepath.Join(layout.CommonDir, "info", "exclude"),
		prefix: filepath.ToSlash(layout.Prefix),
		linked: filepath.Clean(layout.GitDir) != filepath.Clean(layout.CommonDir),
	}, true
}

// personalExcludeKey names the project a personal block belongs to, so
// projects sharing one info/exclude keep their own.
func personalExcludeKey(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	return filepath.ToSlash(canonicalDir(abs))
}

// personalSpan is one complete personal block: its begin marker, the
// note, and the end marker of the same key, with no begin marker between.
// Anything else, even a line that looks like a marker, is the user's.
type personalSpan struct {
	key        string
	start, end int
}

func personalSpans(lines []string) []personalSpan {
	var spans []personalSpan
	for i := 0; i < len(lines); i++ {
		key, ok := personalBeginKey(lines[i])
		if !ok || i+1 >= len(lines) || lines[i+1] != personalExcludeNote {
			continue
		}
		for j := i + 2; j < len(lines); j++ {
			if _, nested := personalBeginKey(lines[j]); nested {
				break
			}
			if lines[j] == personalExcludeEnd+key+" <<<" {
				spans = append(spans, personalSpan{key, i, j})
				i = j
				break
			}
		}
	}
	return spans
}

func personalBeginKey(line string) (string, bool) {
	rest, ok := strings.CutPrefix(line, personalExcludeStart)
	if !ok {
		return "", false
	}
	key, ok := strings.CutSuffix(rest, " >>>")
	return key, ok && key != ""
}

// excludeLines splits content into lines without their line endings and
// returns the ending the file uses.
func excludeLines(content string) (lines []string, newline string) {
	newline = "\n"
	if strings.Contains(content, "\r\n") {
		newline = "\r\n"
	}
	text := strings.TrimSuffix(strings.TrimSuffix(content, "\n"), "\r")
	if text == "" {
		return nil, newline
	}
	lines = strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	return lines, newline
}

// personalBlockLines renders the block for key, nil when there are no
// patterns.
func personalBlockLines(key, prefix string, patterns []string) []string {
	if len(patterns) == 0 {
		return nil
	}
	block := []string{personalExcludeStart + key + " >>>", personalExcludeNote}
	for _, p := range patterns {
		block = append(block, "/"+strings.TrimPrefix(prefix+strings.TrimPrefix(p, "/"), "/"))
	}
	return append(block, personalExcludeEnd+key+" <<<")
}

// withPersonalExcludes returns info/exclude content whose block for key
// lists patterns, or holds no block for key when patterns is empty.
func withPersonalExcludes(content, key, prefix string, patterns []string) string {
	return withPersonalBlock(content, key, personalBlockLines(key, prefix, patterns))
}

// withPersonalBlock returns content with key's block set to block, in
// place, or removed when block is nil. Blocks of projects that no longer
// exist go too. Every other line stays as the file holds it.
func withPersonalBlock(content, key string, block []string) string {
	lines, newline := excludeLines(content)
	spans := personalSpans(lines)
	var out []string
	placed, changed := false, false
	for i := 0; i < len(lines); i++ {
		span := slices.IndexFunc(spans, func(s personalSpan) bool { return s.start == i })
		if span < 0 {
			out = append(out, lines[i])
			continue
		}
		s := spans[span]
		switch {
		case s.key == key:
			if !placed {
				out = append(out, block...)
				placed = true
			}
			changed = true
		case projectGone(s.key):
			changed = true
		default:
			out = append(out, lines[s.start:s.end+1]...)
		}
		i = s.end
	}
	if !placed && len(block) > 0 {
		out = append(out, block...)
		changed = true
	}
	if !changed {
		return content
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, newline) + newline
}

// personalBlockOf returns the lines of key's block in content, nil when
// it has none.
func personalBlockOf(content, key string) []string {
	lines, _ := excludeLines(content)
	for _, s := range personalSpans(lines) {
		if s.key == key {
			return slices.Clone(lines[s.start : s.end+1])
		}
	}
	return nil
}

// personalPatternsOf returns the root-anchored outputs key's block lists.
func personalPatternsOf(content, key, prefix string) []string {
	block := personalBlockOf(content, key)
	if len(block) < 3 {
		return nil
	}
	var out []string
	for _, line := range block[2 : len(block)-1] {
		if rest, ok := strings.CutPrefix(line, "/"+prefix); ok {
			out = append(out, "/"+rest)
		}
	}
	return out
}

func projectGone(dir string) bool {
	dir = filepath.FromSlash(dir)
	if !filepath.IsAbs(dir) {
		return false
	}
	_, err := os.Stat(dir)
	return errors.Is(err, fs.ErrNotExist)
}

// excludeLockWait bounds how long a sync waits for another one to finish
// its info/exclude update, as a worktree and its main checkout may.
const excludeLockWait = 10 * time.Second

// updateGitExclude rewrites the info/exclude at path with edit, holding a
// lock beside it so concurrent syncs never lose each other's blocks.
func updateGitExclude(path string, edit func(string) string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	lockPath := filepath.Join(dir, "agnostic-ai-exclude.lock")
	lock, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("%s: %w", lockPath, err)
	}
	defer func() { _ = lock.Close() }()
	deadline := time.Now().Add(excludeLockWait)
	for {
		err := tryLockProjectFile(lock)
		if err == nil {
			break
		}
		if !projectLockContended(err) || time.Now().After(deadline) {
			return fmt.Errorf("lock %s: %w", lockPath, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	updated := edit(string(existing))
	if updated == string(existing) {
		return nil
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// writePersonalExcludes updates the project's block in info/exclude.
// Outside a work tree there is nothing to ignore. A linked worktree's
// patterns would hide those paths in every checkout, so its personal
// outputs stay untracked and sync says so.
func writePersonalExcludes(root string, patterns []string) error {
	ex, ok := personalExcludeFile(root)
	if !ok {
		return nil
	}
	if ex.linked && len(patterns) > 0 {
		names := make([]string, len(patterns))
		for i, p := range patterns {
			names[i] = strings.TrimPrefix(p, "/")
		}
		summaryf("%s untracked in this linked worktree: %s. Its info/exclude is shared with every checkout, so sync ignores personal outputs only in the main checkout.\n", bang(), strings.Join(names, ", "))
		patterns = nil
	}
	key := personalExcludeKey(root)
	return updateGitExclude(ex.path, func(content string) string {
		return withPersonalExcludes(content, key, ex.prefix, patterns)
	})
}

// priorPersonalExclude saves the project's block so a failed sync puts
// back that block alone, keeping what other projects wrote meanwhile.
func priorPersonalExclude(root string) (priorFile, bool) {
	ex, ok := personalExcludeFile(root)
	if !ok {
		return priorFile{}, false
	}
	data, err := os.ReadFile(ex.path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return priorFile{}, false
	}
	key := personalExcludeKey(root)
	saved := personalBlockOf(string(data), key)
	return priorFile{path: ex.path, restore: func() error {
		return updateGitExclude(ex.path, func(content string) string { return withPersonalBlock(content, key, saved) })
	}}, true
}
