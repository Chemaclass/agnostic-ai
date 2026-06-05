package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

const (
	gitignoreBlockStart = "# >>> agnostic-ai (managed) >>>"
	gitignoreBlockEnd   = "# <<< agnostic-ai (managed) <<<"
	gitignoreBlockNote  = "# Generated paths. Edit specs, not this block. Run `agnostic-ai sync` to refresh."
)

// ensureLineInGitignore appends entry to .gitignore at root so it is
// never committed. The file is created if missing. A no-op when the
// entry already appears verbatim on its own line. Used by `init` to
// gitignore the local-override config; lives here (next to the
// managed-block writer) rather than in init.go so all gitignore
// mutations share one home.
func ensureLineInGitignore(root, entry string) error {
	path := filepath.Join(root, ".gitignore")
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == entry {
			return nil
		}
	}
	var buf strings.Builder
	if len(existing) > 0 {
		buf.Write(existing)
		if !strings.HasSuffix(string(existing), "\n") {
			buf.WriteString("\n")
		}
	}
	buf.WriteString(entry)
	buf.WriteString("\n")
	if err := os.WriteFile(path, []byte(buf.String()), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// normalizeAndSort converts a slice of filesystem paths to gitignore
// entries: forward slashes, leading `./` trimmed, deduplicated, sorted.
func normalizeAndSort(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		if n := normalizeGitignorePath(p); n != "" {
			seen[n] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// normalizeGitignorePath converts a filesystem path to a gitignore entry:
// always forward-slashed, leading "./" stripped, and root-anchored with a
// leading "/". Emitted paths are relative to the project root, so anchoring
// them keeps the pattern from matching a same-named file nested elsewhere
// (e.g. a golden `AGENTS.md` under internal/adapters/*/testdata/). Without
// the anchor an unanchored basename like `.rules` or `CONVENTIONS.md` would
// match at any depth and silently ignore those fixtures.
func normalizeGitignorePath(p string) string {
	p = filepath.ToSlash(p)
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	if p == "" || p == "." {
		return ""
	}
	return "/" + p
}

// collapseManagedEntries folds every entry that lives under a generated
// output directory into a single `/dir/` rule, so the managed block reads
// `/.claude/` instead of one line per emitted file. Two kinds of entry are
// kept verbatim so collapsing never ignores a committed file:
//   - root-level files (no directory segment, e.g. `/AGENTS.md`);
//   - entries under a protected source directory, where tracked specs live
//     alongside generated state (e.g. `/.agnostic-ai/.sync-state`).
//
// Input entries are already root-anchored and sorted (normalizeAndSort).
func collapseManagedEntries(entries, protectedTopDirs []string) []string {
	protected := make(map[string]struct{}, len(protectedTopDirs))
	for _, d := range protectedTopDirs {
		protected[d] = struct{}{}
	}
	seen := make(map[string]struct{}, len(entries))
	out := make([]string, 0, len(entries))
	add := func(e string) {
		if _, ok := seen[e]; ok {
			return
		}
		seen[e] = struct{}{}
		out = append(out, e)
	}
	for _, e := range entries {
		rel := strings.TrimPrefix(e, "/")
		i := strings.Index(rel, "/")
		if i < 0 {
			add(e) // root-level file: nothing to collapse into
			continue
		}
		if _, isProtected := protected[rel[:i]]; isProtected {
			add(e) // source dir: keep the precise file
			continue
		}
		add("/" + rel[:i] + "/")
	}
	sort.Strings(out)
	return out
}

// protectedSourceTopDirs returns the top-level directory of every spec
// source plus the `.agnostic-ai` layer root. collapseManagedEntries leaves
// entries under these untouched so a tracked spec is never swallowed by a
// directory-wide ignore.
func protectedSourceTopDirs(cfg *config.Config) []string {
	set := map[string]struct{}{".agnostic-ai": {}}
	for _, s := range []string{
		cfg.Sources.Agents, cfg.Sources.Skills, cfg.Sources.Rules,
		cfg.Sources.Hooks, cfg.Sources.MCPs, cfg.Sources.Commands,
	} {
		if top := gitignoreTopSegment(s); top != "" {
			set[top] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for d := range set {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

func gitignoreTopSegment(p string) string {
	p = strings.TrimPrefix(filepath.ToSlash(p), "./")
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return ""
	}
	if i := strings.Index(p, "/"); i >= 0 {
		return p[:i]
	}
	return p
}

// buildManagedBlock assembles the managed-block lines: collapsed,
// root-anchored ignores first, then the configured re-allow exceptions as
// `!`-prefixed lines. Allows are emitted last so they override any broader
// ignore above them, letting a project keep a tracked fixture (e.g.
// `internal/adapters/**/testdata/**`) without hand-editing the block (#388).
func buildManagedBlock(cfg *config.Config, entries []string) []string {
	block := collapseManagedEntries(normalizeAndSort(entries), protectedSourceTopDirs(cfg))
	return append(block, normalizeAllowEntries(cfg.Gitignore.Allow)...)
}

// normalizeAllowEntries turns the configured allow patterns into `!`-prefixed
// gitignore lines: forward slashes, leading `./` and `!` trimmed before the
// canonical `!` is re-added, blanks dropped, deduplicated, sorted. Patterns
// are gitignore globs and stay unanchored so a re-allow like `**/AGENTS.md`
// keeps matching at any depth.
func normalizeAllowEntries(patterns []string) []string {
	seen := make(map[string]struct{}, len(patterns))
	for _, p := range patterns {
		p = strings.TrimSpace(filepath.ToSlash(p))
		p = strings.TrimPrefix(p, "./")
		p = strings.TrimPrefix(p, "!")
		if p == "" {
			continue
		}
		seen["!"+p] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// updateGitignore rewrites the managed block in `.gitignore` (or
// cfg.Gitignore.Path) with the provided entries. Lines outside the block
// are preserved as-is. The file is created if missing. An empty entries
// list removes the block.
func updateGitignore(cfg *config.Config, entries []string) error {
	path := ".gitignore"
	if cfg.Gitignore.Path != "" {
		path = cfg.Gitignore.Path
	}
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	updated := replaceManagedBlock(string(existing), entries)
	if updated == string(existing) {
		return nil
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// replaceManagedBlock returns content with the agnostic-ai managed block
// rewritten to list entries. If entries is empty, the block is removed.
// If no prior block exists, a new one is appended (with a leading blank
// line when the file is non-empty).
func replaceManagedBlock(content string, entries []string) string {
	startIdx := strings.Index(content, gitignoreBlockStart)
	if startIdx >= 0 {
		endIdx := strings.Index(content[startIdx:], gitignoreBlockEnd)
		if endIdx < 0 {
			// Truncated block; treat the rest of the file as the block to
			// avoid duplicating markers on the next run.
			endIdx = len(content) - startIdx
		} else {
			endIdx += len(gitignoreBlockEnd)
		}
		before := content[:startIdx]
		after := strings.TrimLeft(content[startIdx+endIdx:], "\n")
		newBlock := renderBlock(entries)
		if newBlock == "" {
			before = strings.TrimRight(before, "\n")
			if before != "" && after != "" {
				return before + "\n\n" + after
			}
			if before != "" {
				return before + "\n"
			}
			return after
		}
		joined := strings.TrimRight(before, "\n") + "\n\n" + newBlock
		if after != "" {
			joined += "\n" + after
		} else {
			joined += "\n"
		}
		if before == "" {
			joined = strings.TrimLeft(joined, "\n")
		}
		return joined
	}

	newBlock := renderBlock(entries)
	if newBlock == "" {
		return content
	}
	if content == "" {
		return newBlock + "\n"
	}
	return strings.TrimRight(content, "\n") + "\n\n" + newBlock + "\n"
}

func renderBlock(entries []string) string {
	if len(entries) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(gitignoreBlockStart)
	sb.WriteString("\n")
	sb.WriteString(gitignoreBlockNote)
	sb.WriteString("\n")
	for _, e := range entries {
		sb.WriteString(e)
		sb.WriteString("\n")
	}
	sb.WriteString(gitignoreBlockEnd)
	return sb.String()
}
