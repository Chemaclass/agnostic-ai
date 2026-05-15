package cli

import (
	"errors"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// reportUnmatchedGlobs walks loaded rules and reports any whose `globs:`
// frontmatter matches no path in the working tree. Off-by-default in
// doctor (some monorepos legitimately ship globs for paths that will
// exist later); enabled via `doctor --check-globs`.
//
// `**` matches any number of path segments (including zero); `*`
// matches anything except `/`. Multiple comma-separated patterns are
// split and checked independently; a rule passes when ANY pattern
// matches at least one file in the working tree.
func reportUnmatchedGlobs(cmd *cobra.Command, root string) error {
	_, b, err := loadProject(root)
	if err != nil {
		return err
	}
	var unmatched []ruleGlob
	for _, r := range b.Rules {
		raw := strings.TrimSpace(r.Globs())
		if raw == "" {
			continue
		}
		patterns := splitGlobPatterns(raw)
		if len(patterns) == 0 {
			continue
		}
		matched, err := anyGlobMatches(root, patterns)
		if err != nil {
			return err
		}
		if !matched {
			unmatched = append(unmatched, ruleGlob{name: r.Name, globs: raw})
		}
	}
	if len(unmatched) == 0 {
		cmd.Println("  ✓ every rule's `globs:` pattern matches at least one file")
		return nil
	}
	sort.Slice(unmatched, func(i, j int) bool { return unmatched[i].name < unmatched[j].name })
	cmd.Printf("  ! %d rule(s) with no matching files in repo (rule will never load):\n", len(unmatched))
	for _, u := range unmatched {
		cmd.Printf("      %s (globs: %s)\n", u.name, u.globs)
	}
	cmd.Println("    fix: update the rule's `globs:` or remove the rule")
	return nil
}

type ruleGlob struct {
	name  string
	globs string
}

// splitGlobPatterns splits a `globs:` value on commas (the convention
// adopted by Cursor / Cline / Windsurf when listing multiple patterns
// inline). Whitespace around each pattern is trimmed.
func splitGlobPatterns(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// anyGlobMatches returns true when at least one pattern matches at
// least one path under root. Walks the filesystem once per call.
// `.git`, `.agnostic-ai`, `node_modules`, and `vendor` are pruned so
// the walk stays bounded.
func anyGlobMatches(root string, patterns []string) (bool, error) {
	skip := map[string]bool{".git": true, ".agnostic-ai": true, "node_modules": true, "vendor": true}
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := compileGlob(p)
		if err != nil {
			return false, err
		}
		compiled = append(compiled, re)
	}
	matched := false
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if d.IsDir() {
			if path != root && skip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		for _, re := range compiled {
			if re.MatchString(rel) {
				matched = true
				return filepath.SkipAll
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, filepath.SkipAll) {
		return false, err
	}
	return matched, nil
}

// compileGlob translates a glob pattern with `**` (any number of path
// segments) and `*` (anything except `/`) into a regex anchored to the
// full path. Reserved regex metacharacters are escaped.
func compileGlob(pattern string) (*regexp.Regexp, error) {
	var sb strings.Builder
	sb.WriteString("^")
	i := 0
	for i < len(pattern) {
		switch {
		case strings.HasPrefix(pattern[i:], "/**/"):
			sb.WriteString("(?:/.*/|/)")
			i += 4
		case strings.HasPrefix(pattern[i:], "**/"):
			sb.WriteString("(?:.*/)?")
			i += 3
		case strings.HasPrefix(pattern[i:], "/**"):
			sb.WriteString("(?:/.*)?")
			i += 3
		case strings.HasPrefix(pattern[i:], "**"):
			sb.WriteString(".*")
			i += 2
		case pattern[i] == '*':
			sb.WriteString("[^/]*")
			i++
		case pattern[i] == '?':
			sb.WriteString("[^/]")
			i++
		case pattern[i] == '.':
			sb.WriteString("\\.")
			i++
		case pattern[i] == '+' || pattern[i] == '(' || pattern[i] == ')' ||
			pattern[i] == '|' || pattern[i] == '^' || pattern[i] == '$' ||
			pattern[i] == '{' || pattern[i] == '}' || pattern[i] == '\\':
			sb.WriteByte('\\')
			sb.WriteByte(pattern[i])
			i++
		default:
			sb.WriteByte(pattern[i])
			i++
		}
	}
	sb.WriteString("$")
	return regexp.Compile(sb.String())
}
