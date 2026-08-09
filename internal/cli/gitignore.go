package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

const (
	gitignoreBlockStart = "# >>> agnostic-ai (managed) >>>"
	gitignoreBlockEnd   = "# <<< agnostic-ai (managed) <<<"
	gitignoreBlockNote  = "# Generated paths. Edit specs, not this block. Run `agnostic-ai sync` to refresh."
	gitignoreBlockHint  = "# Not committed: a fresh clone or `git worktree` lacks these until `agnostic-ai sync` runs (e.g. a post-checkout hook)."
)

// fixedManagedEntries are the always-ignored agnostic-ai paths that are
// not discovered from sync output: the local-override config, the sync
// state file, and the installed-packs dir. They live inside the managed
// block (not as loose lines) so one block owns every agnostic-ai
// gitignore entry: anchored, deduplicated, and refreshed on each write.
//
// Returned in bare (un-anchored) form; buildManagedBlock anchors and
// collapses them with the rest of the block.
func fixedManagedEntries() []string {
	return []string{
		config.LocalOverrideFileName,
		".agnostic-ai/.sync-state",
		packsDir + "/",
	}
}

// looseFixedDuplicates is the set of standalone lines older versions
// wrote outside the managed block for the fixed entries (both the bare
// form `init` emitted and the root-anchored form). The block now owns
// these, so the writer strips any such line found outside it, converging
// projects created before the consolidation (#401).
func looseFixedDuplicates() map[string]struct{} {
	set := make(map[string]struct{}, len(fixedManagedEntries())*2)
	for _, e := range fixedManagedEntries() {
		set[e] = struct{}{}
		set["/"+e] = struct{}{}
	}
	return set
}

// stripLooseFixedDuplicates drops every standalone line matching a fixed
// managed entry (bare or anchored) from text, so a stale loose copy left
// outside the managed block disappears on the next write. Other lines,
// blank lines, and comments are preserved verbatim.
func stripLooseFixedDuplicates(text string) string {
	if text == "" {
		return text
	}
	dups := looseFixedDuplicates()
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if _, ok := dups[strings.TrimSpace(line)]; ok {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// gitignoreHintsForTargets gathers the ignore-only paths every active
// target declares: local artifacts the tool creates that agnostic-ai
// never emits but that must stay out of version control. Riding inside
// the managed block, they survive each sync refresh instead of needing
// hand maintenance (#469). Hints are target-scoped, so a project that
// does not enable the target never sees them.
func gitignoreHintsForTargets(cfg *config.Config, targets []string) []string {
	var out []string
	for _, t := range targets {
		out = append(out, adapters.GitignoreHintsFor(t, cfg)...)
	}
	return out
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
	return sortedKeys(seen)
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

// collapseManagedEntries folds entries that live under a generated output
// subdirectory into a single `/dir/sub/` rule, so the managed block reads
// `/.claude/rules/` instead of one line per emitted file. Collapsing stops at
// the generated subdirectory rather than the tool's top-level dir, so a
// hand-authored sibling (e.g. `.claude/settings.json`, `.claude/hooks/`) is
// never swallowed by a `/.claude/` ignore (#414). Three kinds of entry are
// kept verbatim so collapsing never ignores a committed file:
//   - root-level files (no directory segment, e.g. `/AGENTS.md`);
//   - files sitting directly under a tool dir (e.g. `/.claude/CLAUDE.md`);
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
		segs := strings.SplitN(rel, "/", 3)
		switch {
		case len(segs) < 2:
			add(e) // root-level file: nothing to collapse into
		case len(segs) == 2:
			add(e) // file directly under a tool dir: keep precise
		default:
			if _, isProtected := protected[segs[0]]; isProtected {
				add(e) // source dir: keep the precise file
				continue
			}
			add("/" + segs[0] + "/" + segs[1] + "/")
		}
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
		cfg.Sources.Settings, cfg.Sources.Reviews, cfg.Sources.Environments,
		cfg.Sources.Ignore,
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
	entries = append(fixedManagedEntries(), dropSourceEntryPoint(entries)...)
	block := collapseManagedEntries(normalizeAndSort(entries), protectedSourceTopDirs(cfg))
	return append(block, normalizeAllowEntries(cfg.Gitignore.Allow)...)
}

// dropSourceEntryPoint removes AGNOSTIC_AI.md from the recorded emissions.
//
// The first sync in a fresh project writes it and records it like any other
// emission; later syncs read it from disk and skip the write, so it is absent
// from the second run's entries. That alone made the first .gitignore differ
// from every later one. The real damage is that it is a source file, the
// shared instruction body every target's entry point renders from, so
// `init && sync && git add -A && git commit` silently left it out of the
// repository (#580).
//
// It cannot be handled by protectedSourceTopDirs: that guards against
// collapsing entries into a directory-wide ignore, and `.agnostic-ai` legitimately
// holds two managed entries (`.sync-state`, `packs/`) that must stay listed.
func dropSourceEntryPoint(entries []string) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if normalizeGitignorePath(e) == normalizeGitignorePath(adapters.AgnosticEntryPointPath) {
			continue
		}
		out = append(out, e)
	}
	return out
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
	return sortedKeys(seen)
}

// ensureManagedGitignore guarantees a managed block exists at root with
// at least the fixed agnostic-ai entries (local-override config, sync
// state, packs dir). An existing block is left untouched, since it
// already carries the fixed entries and sync owns its generated lines;
// only an absent block is created. Used by `packs add`, which must
// ignore the packs dir but does not know the generated-output paths.
func ensureManagedGitignore(root string) error {
	path := filepath.Join(root, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if strings.Contains(string(data), gitignoreBlockStart) {
		return nil
	}
	cfg := &config.Config{}
	return updateGitignore(root, cfg, buildManagedBlock(cfg, nil))
}

// updateGitignore rewrites the managed block in `<root>/.gitignore` (or
// root-joined cfg.Gitignore.Path) with the provided entries. Lines
// outside the block are preserved, except stale loose copies of the
// fixed entries, which are stripped so the block becomes their single
// home. The file is created if missing. An empty entries list removes
// the block.
func updateGitignore(root string, cfg *config.Config, entries []string) error {
	path := ".gitignore"
	if cfg.Gitignore.Path != "" {
		path = cfg.Gitignore.Path
	}
	path = filepath.Join(root, path)
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	updated := replaceManagedBlock(stripLooseFixedDuplicates(string(existing)), entries)
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
	sb.WriteString(gitignoreBlockHint)
	sb.WriteString("\n")
	for _, e := range entries {
		sb.WriteString(e)
		sb.WriteString("\n")
	}
	sb.WriteString(gitignoreBlockEnd)
	return sb.String()
}
