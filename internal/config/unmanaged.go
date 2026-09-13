package config

import (
	"path"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// MatchUnmanaged reports whether p matches one of patterns. Both sides
// are project-relative with forward slashes; a leading `./` or `/` is
// ignored, and a backslash in p counts as a separator. A pattern is an
// exact path, a path.Match glob (`*` never crosses `/`), or a directory
// prefix when it ends with `/`. Malformed globs are rejected at config
// load, so a Match error here never fires.
func MatchUnmanaged(patterns []string, p string) bool {
	if len(patterns) == 0 {
		return false
	}
	rel := normalizeUnmanaged(strings.ReplaceAll(p, `\`, "/"))
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		// Read the trailing slash before normalizing: Clean strips it.
		isDir := strings.HasSuffix(pattern, "/")
		pattern = normalizeUnmanaged(pattern)
		if isDir {
			if rel == pattern || strings.HasPrefix(rel, pattern+"/") {
				return true
			}
			continue
		}
		if rel == pattern {
			return true
		}
		if ok, _ := path.Match(pattern, rel); ok {
			return true
		}
	}
	return false
}

// MatchUnmanagedDir reports whether dir holds, or could hold, a path
// matching one of patterns. Unlike MatchUnmanaged it needs no concrete
// file: it compares dir's segments against each pattern's leading
// segments with path.Match. A file pattern must reach below dir; a
// directory pattern (trailing `/`) counts at any depth, since it owns
// everything under it and may itself sit under dir.
func MatchUnmanagedDir(patterns []string, dir string) bool {
	dirSegs := strings.Split(normalizeUnmanaged(strings.ReplaceAll(dir, `\`, "/")), "/")
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		isDir := strings.HasSuffix(pattern, "/")
		patSegs := strings.Split(normalizeUnmanaged(pattern), "/")
		n := len(dirSegs)
		if len(patSegs) <= n {
			if !isDir {
				continue // a file pattern this shallow cannot sit under dir
			}
			n = len(patSegs)
		}
		if segmentsMatch(patSegs[:n], dirSegs[:n]) {
			return true
		}
	}
	return false
}

// segmentsMatch reports whether every name matches the glob at the same
// position. Both slices have the same length.
func segmentsMatch(globs, names []string) bool {
	for i, g := range globs {
		if ok, _ := path.Match(g, names[i]); !ok {
			return false
		}
	}
	return true
}

// IsUnmanaged is MatchUnmanaged over c.Sync.Unmanaged. Nil-safe so
// callers holding a possibly-nil config need no guard.
func (c *Config) IsUnmanaged(p string) bool {
	return c != nil && MatchUnmanaged(c.Sync.Unmanaged, p)
}

// normalizeUnmanaged cleans a slash path and drops a leading `/`, so
// `./AGENTS.md`, `/AGENTS.md`, and `AGENTS.md` compare equal.
func normalizeUnmanaged(p string) string {
	return strings.TrimPrefix(path.Clean(p), "/")
}

// validateUnmanaged rejects a malformed glob in sync.unmanaged at load
// time, so a typo fails loudly instead of silently owning nothing.
func validateUnmanaged(patterns []string, source string) error {
	for _, p := range patterns {
		if _, err := path.Match(p, ""); err != nil {
			return errs.Coded(errs.CodeConfigDecode, "%s: sync.unmanaged: bad pattern %q: %w", source, p, err)
		}
	}
	return nil
}
