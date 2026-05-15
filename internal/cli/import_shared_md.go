package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// extractLeadingItalic returns the inner text of a `_..._` paragraph
// at the start of body (no surrounding whitespace) and the body with
// that paragraph plus its trailing blank line stripped. ok is false
// when body does not begin with a single-line italic paragraph.
//
// Several adapters render `description:` as a leading italic paragraph
// in the emitted markdown. Importers use this helper to reverse that
// transformation so a description round-trips through frontmatter
// rather than accumulating as body content.
func extractLeadingItalic(body string) (desc, stripped string, ok bool) {
	trimmed := strings.TrimLeft(body, "\n")
	nl := strings.IndexByte(trimmed, '\n')
	var first string
	if nl < 0 {
		first = trimmed
	} else {
		first = trimmed[:nl]
	}
	first = strings.TrimSpace(first)
	if len(first) < 3 || !strings.HasPrefix(first, "_") || !strings.HasSuffix(first, "_") {
		return "", body, false
	}
	if strings.Count(first, "_") != 2 {
		return "", body, false
	}
	desc = first[1 : len(first)-1]
	rest := ""
	if nl >= 0 {
		rest = strings.TrimLeft(trimmed[nl+1:], "\n")
	}
	return desc, rest, true
}

// sliceMainFileByH2 splits <root>/<srcName> on `## ` headings into one
// rule per section in dstDir. Without headings it writes a single rule
// named after the project directory. No-op when the source file is
// absent or empty. Reused by every importer whose target keeps rules
// in a single concatenated markdown file (CONVENTIONS.md, AGENTS.md,
// GEMINI.md, .opencode/AGENTS.md, .rules, etc.).
func sliceMainFileByH2(root, srcName, dstDir string) (int, error) {
	src := filepath.Join(root, srcName)
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

	used := map[string]int{}
	for _, s := range sections {
		used[s.slug] = 1
	}
	count := 0
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
			return count, err
		}
		count++
	}
	return count, nil
}

// writeAgentMD writes an agent spec to path with a name + optional
// description and tags frontmatter, followed by body.
func writeAgentMD(path, name, description string, tags []string, body string) error {
	var sb strings.Builder
	sb.WriteString("---\nname: " + name + "\n")
	if description != "" {
		sb.WriteString("description: " + description + "\n")
	}
	if len(tags) > 0 {
		sb.WriteString("tags: [" + strings.Join(tags, ", ") + "]\n")
	}
	sb.WriteString("---\n\n")
	sb.WriteString(strings.TrimRight(body, "\n"))
	sb.WriteString("\n")
	if err := importWriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
