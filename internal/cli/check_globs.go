package cli

import (
	"errors"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/spec"
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
func reportUnmatchedGlobs(cmd *cobra.Command, root string) (int, error) {
	_, b, err := loadProject(root)
	if err != nil {
		return 0, err
	}
	unmatched, err := collectUnmatchedGlobs(root, b)
	if err != nil {
		return 0, err
	}
	if len(unmatched) == 0 {
		cmd.Println("  ✓ every rule's `globs:` pattern matches at least one file")
		return 0, nil
	}
	cmd.Printf("  ! %d rule(s) with no matching files in repo (rule will never load):\n", len(unmatched))
	for _, u := range unmatched {
		if u.Invalid != "" {
			cmd.Printf("      %s (globs: %s; %s is not a pattern this check can read)\n", u.Rule, u.Globs, u.Invalid)
			continue
		}
		cmd.Printf("      %s (globs: %s)\n", u.Rule, u.Globs)
	}
	cmd.Println("    fix: update the rule's `globs:` or remove the rule")
	return len(unmatched), nil
}

// collectUnmatchedGlobs lists, by rule name, the rules in b whose
// `globs:` match no path under root or hold a pattern this check cannot read.
func collectUnmatchedGlobs(root string, b spec.Bundle) ([]unmatchedGlob, error) {
	unmatched := []unmatchedGlob{}
	for _, r := range b.Rules {
		raw := strings.TrimSpace(r.Globs())
		if raw == "" {
			continue
		}
		patterns := splitGlobPatterns(raw)
		if len(patterns) == 0 {
			continue
		}
		// A pattern that cannot compile names its rule; the rest still run.
		if bad := invalidGlob(patterns); bad != "" {
			unmatched = append(unmatched, unmatchedGlob{Rule: r.Name, Globs: raw, Invalid: bad})
			continue
		}
		matched, err := anyGlobMatches(root, patterns)
		if err != nil {
			return nil, err
		}
		if !matched {
			unmatched = append(unmatched, unmatchedGlob{Rule: r.Name, Globs: raw})
		}
	}
	sort.Slice(unmatched, func(i, j int) bool { return unmatched[i].Rule < unmatched[j].Rule })
	return unmatched, nil
}

// unmatchedGlob is a rule that never loads. Invalid names the pattern
// this check cannot read, when that is the reason.
type unmatchedGlob struct {
	Rule    string `json:"rule"`
	Globs   string `json:"globs"`
	Invalid string `json:"invalid,omitempty"`
}

// invalidGlob returns the first pattern compileGlob cannot compile, or "".
func invalidGlob(patterns []string) string {
	for _, p := range patterns {
		if _, err := compileGlob(p); err != nil {
			return p
		}
	}
	return ""
}

// splitGlobPatterns splits a `globs:` value on the commas outside brace
// sets (the convention adopted by Cursor / Cline / Windsurf when listing
// multiple patterns inline). Whitespace around each pattern is trimmed.
func splitGlobPatterns(raw string) []string {
	return spec.GlobList(raw)
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
// segments), `*` (anything except `/`), and `{a,b}` brace sets into a
// regex anchored to the full path. As in picomatch and minimatch, a
// brace pair is a set only when it holds a comma at its own depth, so
// `{x}`, `{}`, `{{name}}`, and an unpaired brace match themselves.
// Reserved regex metacharacters are escaped.
func compileGlob(pattern string) (*regexp.Regexp, error) {
	sets, alternates := braceSets(pattern)
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
		case pattern[i] == '{' && sets[i]:
			sb.WriteString("(?:")
			i++
		case pattern[i] == '}' && sets[i]:
			sb.WriteString(")")
			i++
		case pattern[i] == ',' && alternates[i]:
			sb.WriteString("|")
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

// braceSets marks, by byte index, the braces of each pair in pattern
// that holds a comma at its own depth, and those commas. Pairs match
// left to right; a brace with no partner is in no pair.
func braceSets(pattern string) (sets, alternates map[int]bool) {
	closeOf := map[int]int{}
	var open []int
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '{':
			open = append(open, i)
		case '}':
			if len(open) > 0 {
				closeOf[open[len(open)-1]] = i
				open = open[:len(open)-1]
			}
		}
	}
	sets, alternates = map[int]bool{}, map[int]bool{}
	var pairs []int
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '{':
			if _, ok := closeOf[i]; ok {
				pairs = append(pairs, i)
			}
		case '}':
			if len(pairs) > 0 && closeOf[pairs[len(pairs)-1]] == i {
				pairs = pairs[:len(pairs)-1]
			}
		case ',':
			if len(pairs) > 0 {
				o := pairs[len(pairs)-1]
				alternates[i] = true
				sets[o], sets[closeOf[o]] = true, true
			}
		}
	}
	return sets, alternates
}
