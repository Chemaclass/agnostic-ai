package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// importClaudeRules splits CLAUDE.md on `## ` headings into one rule file
// per section in dstDir. Without headings it writes a single rule named
// after the project directory.
func importClaudeRules(root, dstDir string) (int, error) {
	src := filepath.Join(root, "CLAUDE.md")
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}

	sections := splitH2Sections(string(data))
	if len(sections) == 0 {
		body := strings.TrimSpace(string(data))
		if body == "" {
			return 0, nil
		}
		name := projectSlug(root)
		path := filepath.Join(dstDir, name+".md")
		if err := writeRule(path, name, body); err != nil {
			return 0, err
		}
		return 1, nil
	}
	for _, s := range sections {
		path := filepath.Join(dstDir, s.slug+".md")
		if err := writeRule(path, s.slug, s.body); err != nil {
			return 0, err
		}
	}
	return len(sections), nil
}

type h2Section struct{ slug, body string }

var h2HeadingRE = regexp.MustCompile(`(?m)^##[ \t]+(.+?)[ \t]*$`)

// splitH2Sections returns one section per `## heading`. Slug collisions
// are deduplicated with -2, -3 suffixes. Content before the first heading
// is discarded.
func splitH2Sections(s string) []h2Section {
	idx := h2HeadingRE.FindAllStringSubmatchIndex(s, -1)
	if len(idx) == 0 {
		return nil
	}
	out := make([]h2Section, 0, len(idx))
	used := map[string]int{}
	for i, m := range idx {
		title := s[m[2]:m[3]]
		base := slugify(title)
		if base == "" {
			base = fmt.Sprintf("section-%d", i+1)
		}
		slug := base
		if n, exists := used[base]; exists {
			used[base] = n + 1
			slug = fmt.Sprintf("%s-%d", base, n+1)
		} else {
			used[base] = 1
		}
		bodyStart := m[1]
		bodyEnd := len(s)
		if i+1 < len(idx) {
			bodyEnd = idx[i+1][0]
		}
		body := strings.TrimSpace(s[bodyStart:bodyEnd])
		out = append(out, h2Section{slug: slug, body: body})
	}
	return out
}

var nonAlphaNumRE = regexp.MustCompile(`[^a-z0-9]+`)

// slugify lowercases and collapses non-alphanumeric runs into single
// hyphens. Leading/trailing hyphens trimmed.
func slugify(s string) string {
	s = strings.ToLower(s)
	s = nonAlphaNumRE.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// projectSlug returns the basename of root, slugified. Falls back to
// "project" for unresolvable paths.
func projectSlug(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "project"
	}
	s := slugify(filepath.Base(abs))
	if s == "" {
		return "project"
	}
	return s
}

func writeRule(path, name, body string) error {
	fm := fmt.Sprintf("---\nname: %s\n---\n\n", name)
	if err := os.WriteFile(path, []byte(fm+body+"\n"), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
