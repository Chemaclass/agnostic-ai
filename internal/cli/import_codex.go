package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// importFromCodex reads existing Codex config (root AGENTS.md plus any
// nested <dir>/AGENTS.md) under root and writes specs into the configured
// source directories.
func importFromCodex(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules); err != nil {
		return err
	}
	n, err := importCodexRules(root, filepath.Join(root, src.Rules), src)
	if err != nil {
		return err
	}
	summaryf("imported %d rules\n", n)
	return nil
}

// codexWrapperHeadings are H2 sections produced by our codex emitter that
// wrap the real rules. Their bodies (the ### children) are what we want;
// the wrapper itself should not become a rule.
var codexWrapperHeadings = map[string]bool{
	"conventions": true,
	"agents":      true,
	"skills":      true,
}

// importCodexRules walks the project tree for AGENTS.md files and writes
// one rule per ## section found into dstDir. Rules from <dir>/AGENTS.md
// inherit "globs: <dir>/**". Slug collisions across files are deduplicated.
func importCodexRules(root, dstDir string, src config.Sources) (int, error) {
	files, err := findCodexFiles(root, src)
	if err != nil {
		return 0, err
	}
	if len(files) == 0 {
		return 0, nil
	}

	used := map[string]int{}
	count := 0
	for _, f := range files {
		data, err := os.ReadFile(f.path)
		if err != nil {
			return count, fmt.Errorf("read %s: %w", f.path, err)
		}
		sections := codexSectionsFrom(string(data))
		if len(sections) == 0 {
			body := strings.TrimSpace(string(data))
			if body == "" {
				continue
			}
			name := dedupSlug(used, projectSlug(root))
			if err := writeCodexRule(dstDir, name, "", f.globs, body); err != nil {
				return count, err
			}
			count++
			continue
		}
		for _, s := range sections {
			name := dedupSlug(used, s.slug)
			if err := writeCodexRule(dstDir, name, s.description, f.globs, s.body); err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}

type codexFile struct {
	path  string
	globs string // "" for root, "src/**" for src/AGENTS.md, etc.
}

// findCodexFiles walks root for AGENTS.md files. Hidden directories and
// the agnostic source dirs are skipped to avoid picking up unrelated
// AGENTS.md files (e.g. from vendored projects or our own scaffold).
func findCodexFiles(root string, src config.Sources) ([]codexFile, error) {
	var out []codexFile
	skipDirs := map[string]bool{
		"node_modules": true, "vendor": true,
	}
	for _, p := range []string{src.Agents, src.Skills, src.Rules, src.Hooks, src.MCPs} {
		if p != "" {
			skipDirs[firstSegment(p)] = true
		}
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || skipDirs[name]) {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() != "AGENTS.md" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		dir := filepath.ToSlash(filepath.Dir(rel))
		var globs string
		if dir != "." {
			globs = dir + "/**"
		}
		out = append(out, codexFile{path: path, globs: globs})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out, nil
}

// firstSegment returns the first path segment of p (e.g. ".agnostic-ai"
// from ".agnostic-ai/agents"). Used to skip the source tree when scanning
// for legacy AGENTS.md files.
func firstSegment(p string) string {
	p = filepath.ToSlash(p)
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return p
}

type codexSection struct {
	slug, description, body string
}

// codexSectionsFrom returns one section per real ## heading. Wrapper
// headings (Conventions/Agents/Skills) are unwrapped — their ### children
// become the actual sections. Italic-only first paragraphs are extracted
// as descriptions and stripped from the body.
func codexSectionsFrom(s string) []codexSection {
	h2 := splitH2Sections(s)
	var out []codexSection
	for _, sec := range h2 {
		if codexWrapperHeadings[sec.slug] {
			out = append(out, unwrapH3(sec.body)...)
			continue
		}
		desc, body := extractItalicDescription(stripHeadingLine(sec.body))
		out = append(out, codexSection{slug: sec.slug, description: desc, body: body})
	}
	return out
}

var h3HeadingRE = regexp.MustCompile(`(?m)^###[ \t]+(.+?)[ \t]*$`)

// unwrapH3 splits a wrapper's body by ### into one section per child.
func unwrapH3(body string) []codexSection {
	idx := h3HeadingRE.FindAllStringSubmatchIndex(body, -1)
	if len(idx) == 0 {
		return nil
	}
	out := make([]codexSection, 0, len(idx))
	for i, m := range idx {
		title := body[m[2]:m[3]]
		slug := slugify(title)
		if slug == "" {
			slug = fmt.Sprintf("section-%d", i+1)
		}
		bodyStart := m[1]
		bodyEnd := len(body)
		if i+1 < len(idx) {
			bodyEnd = idx[i+1][0]
		}
		secBody := strings.TrimSpace(body[bodyStart:bodyEnd])
		desc, secBody := extractItalicDescription(secBody)
		out = append(out, codexSection{slug: slug, description: desc, body: secBody})
	}
	return out
}

// stripHeadingLine drops a leading "# AGENTS.md" or "# AGENTS.md (dir)"
// header sometimes left over inside section bodies (defensive).
var leadingHeadingRE = regexp.MustCompile(`(?m)\A#[ \t]+[^\n]*\n+`)

func stripHeadingLine(s string) string {
	return leadingHeadingRE.ReplaceAllString(s, "")
}

// italicLineRE matches a single-line italic on its own line: "_text_".
// Multi-line italics are intentionally not matched — they're more likely
// to be content than metadata.
var italicLineRE = regexp.MustCompile(`(?m)^_([^_\n]+?)_\s*$`)

// extractItalicDescription is aggressive: if the body's first non-blank
// paragraph is a single-line italic, treat it as the description and
// remove it from the body. Anything else is left alone.
func extractItalicDescription(body string) (string, string) {
	trimmed := strings.TrimLeft(body, "\n\t ")
	m := italicLineRE.FindStringSubmatchIndex(trimmed)
	if m == nil || m[0] != 0 {
		return "", body
	}
	desc := trimmed[m[2]:m[3]]
	rest := strings.TrimLeft(trimmed[m[1]:], "\n")
	return strings.TrimSpace(desc), rest
}

// dedupSlug returns slug, or slug-2/-3/... if it has been used before.
// Mutates used in place.
func dedupSlug(used map[string]int, slug string) string {
	if n, exists := used[slug]; exists {
		used[slug] = n + 1
		return fmt.Sprintf("%s-%d", slug, n+1)
	}
	used[slug] = 1
	return slug
}

func writeCodexRule(dstDir, name, description, globs, body string) error {
	var fm strings.Builder
	fm.WriteString("---\nname: " + name + "\n")
	if description != "" {
		fm.WriteString("description: " + description + "\n")
	}
	if globs != "" {
		fm.WriteString("globs: " + globs + "\n")
	}
	fm.WriteString("---\n\n")
	fm.WriteString(strings.TrimRight(body, "\n"))
	fm.WriteString("\n")

	path := filepath.Join(dstDir, name+".md")
	if err := os.WriteFile(path, []byte(fm.String()), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
