package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

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
	return splitManagedBlock(root, cfg, b, team, targets, recorded)
}

// splitManagedBlock returns the managed block a sync with the team's
// built-ins writes, and the outputs only the personal built-ins add that
// the block leaves unignored. Outputs the personal config turns off stay
// in the block, so agnostic-ai.local.yaml never changes a committed file.
func splitManagedBlock(root string, cfg *config.Config, b spec.Bundle, team, targets, recorded []string) (block, personal []string, err error) {
	if sameBuiltins(team, cfg.Builtins) {
		block, err := syncManagedBlock(root, cfg, b, targets, recorded)
		return block, nil, err
	}
	teamCfg := *cfg
	teamCfg.Builtins = slices.Clone(team)
	layers, err := resolveLayers(root, &teamCfg)
	if err != nil {
		return nil, nil, err
	}
	teamBundle, err := spec.LoadLayered(layers)
	if err != nil {
		return nil, nil, err
	}
	teamBundle.ApplyModelTiers(teamCfg.Models)
	mine := ignoreCandidates(cfg, b)
	theirs := ignoreCandidates(&teamCfg, teamBundle)
	var entries []string
	for _, e := range recorded {
		if n := normalizeGitignorePath(e); !mine[n] || theirs[n] {
			entries = append(entries, e)
		}
	}
	for e := range theirs {
		if !mine[e] {
			entries = append(entries, e)
		}
	}
	block, err = syncManagedBlock(root, &teamCfg, teamBundle, targets, entries)
	if err != nil {
		return nil, nil, err
	}
	for e := range mine {
		if !theirs[e] && !blockCovers(block, e) {
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

// personalExcludeFile returns the repository's info/exclude, in the
// common Git directory a linked worktree shares, and the project's path
// inside its work tree, which anchors the patterns. ok is false outside
// a work tree.
func personalExcludeFile(root string) (path, prefix string, ok bool) {
	layout, err := gitquery.LayoutAt(root)
	if err != nil || !layout.HasToplevel || layout.CommonDir == "" {
		return "", "", false
	}
	return filepath.Join(layout.CommonDir, "info", "exclude"), filepath.ToSlash(layout.Prefix), true
}

// personalExcludeKey names the project a personal block belongs to, so
// projects and worktrees sharing one info/exclude keep their own.
func personalExcludeKey(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	return filepath.ToSlash(canonicalDir(abs))
}

// withPersonalExcludes returns info/exclude content whose block for key
// lists patterns, or holds no block for key when patterns is empty.
// Blocks of projects that no longer exist go too. Other lines stay.
func withPersonalExcludes(content, key, prefix string, patterns []string) string {
	var block []string
	if len(patterns) > 0 {
		block = append(block, personalExcludeStart+key+" >>>", personalExcludeNote)
		for _, p := range patterns {
			block = append(block, "/"+strings.TrimPrefix(prefix+strings.TrimPrefix(p, "/"), "/"))
		}
		block = append(block, personalExcludeEnd+key+" <<<")
	}
	text := strings.TrimSuffix(content, "\n")
	var lines []string
	if text != "" {
		lines = strings.Split(text, "\n")
	}
	var out []string
	placed := false
	for i := 0; i < len(lines); i++ {
		owner, isStart := strings.CutPrefix(lines[i], personalExcludeStart)
		owner, isStart = strings.CutSuffix(owner, " >>>")
		if !isStart {
			out = append(out, lines[i])
			continue
		}
		end := i
		for end < len(lines)-1 && lines[end] != personalExcludeEnd+owner+" <<<" {
			end++
		}
		switch {
		case owner == key:
			if !placed {
				out = append(out, block...)
				placed = true
			}
		case projectGone(owner):
		default:
			out = append(out, lines[i:end+1]...)
		}
		i = end
	}
	if !placed {
		out = append(out, block...)
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}

func projectGone(dir string) bool {
	_, err := os.Stat(filepath.FromSlash(dir))
	return errors.Is(err, fs.ErrNotExist)
}

// writePersonalExcludes updates the project's block in info/exclude.
// Outside a work tree there is nothing to ignore.
func writePersonalExcludes(root string, patterns []string) error {
	path, prefix, ok := personalExcludeFile(root)
	if !ok {
		return nil
	}
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	updated := withPersonalExcludes(string(existing), personalExcludeKey(root), prefix, patterns)
	if updated == string(existing) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
