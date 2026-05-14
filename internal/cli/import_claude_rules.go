package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// importClaudeRules imports rules from a Claude Code project. Prefers
// `.claude/rules/*.md` (each file becomes one rule, byte-identical
// copy). Falls back to slicing CLAUDE.md on `## ` headings when the
// directory is absent. Without headings the slicer writes a single
// rule named after the project directory.
func importClaudeRules(root, dstDir string) (int, error) {
	rulesDir := filepath.Join(root, claudeDir, "rules")
	if dirExists(rulesDir) {
		return copyMarkdownDir(rulesDir, dstDir)
	}
	return sliceMainFileByH2(root, claudeMainFile, dstDir)
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
