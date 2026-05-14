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

// importClaudeRules imports rules from a Claude Code project. Prefers
// `.claude/rules/*.md` when present (each file becomes one rule,
// byte-identical copy). Falls back to splitting CLAUDE.md on `## `
// headings into one rule per section. Without headings it writes a
// single rule named after the project directory.
func importClaudeRules(root, dstDir string) (int, error) {
	if n, ok, err := importClaudeRulesDir(root, dstDir); ok || err != nil {
		return n, err
	}
	src := filepath.Join(root, "CLAUDE.md")
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}

	preamble, sections := splitH2Sections(string(data))
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
	count := 0
	used := map[string]int{}
	for _, s := range sections {
		used[s.slug] = 1
	}
	if preamble != "" {
		slug := preambleSlug(preamble, used)
		path := filepath.Join(dstDir, slug+".md")
		if err := writeRule(path, slug, preamble); err != nil {
			return 0, err
		}
		count++
	}
	for _, s := range sections {
		path := filepath.Join(dstDir, s.slug+".md")
		if err := writeRule(path, s.slug, s.body); err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}

// importClaudeRulesDir copies each `.claude/rules/*.md` byte-for-byte
// to dstDir. Returns (count, true, nil) when the directory exists,
// regardless of how many .md files it contains. Returns
// (0, false, nil) when the directory is absent so the caller can fall
// back to slicing CLAUDE.md.
func importClaudeRulesDir(root, dstDir string) (int, bool, error) {
	src := filepath.Join(root, ".claude", "rules")
	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, true, fmt.Errorf("read %s: %w", src, err)
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			return count, true, fmt.Errorf("read rule %s: %w", e.Name(), err)
		}
		dst := filepath.Join(dstDir, e.Name())
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return count, true, fmt.Errorf("write %s: %w", dst, err)
		}
		count++
	}
	return count, true, nil
}

var h1HeadingRE = regexp.MustCompile(`(?m)^#[ \t]+(.+?)[ \t]*$`)

// preambleSlug picks a slug for content that precedes the first H2. Uses the
// first H1 title if present, falls back to "intro". Disambiguates against
// slugs already used by section headings.
func preambleSlug(preamble string, used map[string]int) string {
	base := "intro"
	if m := h1HeadingRE.FindStringSubmatch(preamble); m != nil {
		if s := slugify(m[1]); s != "" {
			base = s
		}
	}
	slug := base
	for i := 2; used[slug] > 0; i++ {
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	used[slug] = 1
	return slug
}

type h2Section struct{ slug, body string }

var (
	h2HeadingRE = regexp.MustCompile(`^##[ \t]+(.+?)[ \t]*$`)
	fenceRE     = regexp.MustCompile("^[ \\t]*(```|~~~)")
)

// splitH2Sections returns the preamble (content before the first `## heading`)
// and one section per `## heading`. Slug collisions are deduplicated with
// -2, -3 suffixes. Headings inside fenced code blocks are ignored so example
// markdown does not fragment the output.
func splitH2Sections(s string) (string, []h2Section) {
	lines := strings.Split(s, "\n")
	type head struct {
		line  int
		title string
	}
	var heads []head
	inFence := false
	for i, line := range lines {
		if fenceRE.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := h2HeadingRE.FindStringSubmatch(line); m != nil {
			heads = append(heads, head{i, m[1]})
		}
	}
	if len(heads) == 0 {
		return strings.TrimSpace(s), nil
	}
	preamble := strings.TrimSpace(strings.Join(lines[:heads[0].line], "\n"))
	out := make([]h2Section, 0, len(heads))
	used := map[string]int{}
	for i, h := range heads {
		base := slugify(h.title)
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
		bodyStart := h.line + 1
		bodyEnd := len(lines)
		if i+1 < len(heads) {
			bodyEnd = heads[i+1].line
		}
		body := strings.TrimSpace(strings.Join(lines[bodyStart:bodyEnd], "\n"))
		out = append(out, h2Section{slug: slug, body: body})
	}
	return preamble, out
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
