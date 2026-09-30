package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// importedNestedClaudeFile is a nested CLAUDE.md whose text a rule now
// holds, so Claude Code would load that text twice once sync writes it.
type importedNestedClaudeFile struct{ path, rule string }

// importClaudeRules imports rules from a Claude Code project. Prefers
// `.claude/rules/*.md` (each file becomes one rule, byte-identical
// copy). Without that directory, only a rules block sync wrote into the
// root CLAUDE.md becomes rules, and each nested `<dir>/CLAUDE.md`
// becomes one rule scoped to its directory.
func importClaudeRules(root, dstDir string, src config.Sources, layout claudeLayout) (int, []importedNestedClaudeFile, error) {
	rulesDir := filepath.Join(root, layout.rules)
	if dirExists(rulesDir) {
		n, err := copyMarkdownTree(rulesDir, dstDir)
		return n, nil, err
	}
	count, err := importRootClaudeRules(root, dstDir)
	if err != nil {
		return count, nil, err
	}
	nested, leftBehind, err := importNestedClaudeRules(root, dstDir, src)
	return count + nested, leftBehind, err
}

func importRootClaudeRules(root, dstDir string) (int, error) {
	// A CLAUDE.md that imports AGENTS.md feeds AGNOSTIC_AI.md instead: its
	// own text is for Claude only, so it must not become a rule for all.
	if _, companion, err := claudeCompanionBody(root); err != nil || companion {
		return 0, err
	}
	return sliceMirroredMainFile(root, claudeMainFile, dstDir)
}

// importNestedClaudeRules writes each nested CLAUDE.md as one rule with
// the whole file, scoped to its directory and named after that scope.
func importNestedClaudeRules(root, dstDir string, src config.Sources) (int, []importedNestedClaudeFile, error) {
	files, err := findHierarchicalMainFiles(root, claudeMainFile, src)
	if err != nil {
		return 0, nil, err
	}
	names := wholeFileRuleNames(root, files)
	used := map[string]int{}
	var leftBehind []importedNestedClaudeFile
	count := 0
	for _, f := range files {
		if f.globs == "" {
			continue
		}
		body, companionOnly, besideCodexRule, err := nestedClaudeRuleBody(root, f.path)
		if err != nil {
			return count, leftBehind, err
		}
		if body == "" {
			continue
		}
		base := names[f.globs]
		if besideCodexRule {
			base += "-claude"
		}
		name := dedupSlug(used, base)
		// A rule sliced from the root CLAUDE.md in this run may hold the name.
		for importWritten[filepath.Join(dstDir, name+".md")] {
			name = dedupSlug(used, base)
		}
		if err := writeScopedRule(dstDir, name, f.globs, body); err != nil {
			return count, leftBehind, err
		}
		count++
		if !companionOnly {
			rel, _ := filepath.Rel(root, f.path)
			leftBehind = append(leftBehind, importedNestedClaudeFile{path: filepath.ToSlash(rel), rule: name})
		}
	}
	return count, leftBehind, nil
}

// nestedClaudeRuleBody returns the rule text of the nested CLAUDE.md at
// path, "" when it holds none. A companion that imports the AGENTS.md
// beside it reads as that file, the way Claude Code loads it, with its
// own text in a claude fence. When codex imports that AGENTS.md in the
// same run, only the fenced text is left, and besideCodexRule is true.
func nestedClaudeRuleBody(root, path string) (body string, companionOnly, besideCodexRule bool, err error) {
	raw, err := readEntryFile(root, path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, false, nil
	}
	if err != nil {
		return "", false, false, fmt.Errorf("read %s: %w", path, err)
	}
	text := string(raw)
	if header.Has(text) {
		return "", false, false, nil
	}
	rest, companion := adapters.SplitAgentsCompanion(text)
	if !companion {
		return strings.TrimSpace(text), false, false, nil
	}
	var parts []string
	peerOwnsAgents := slices.Contains(importRunSources, "codex")
	if !peerOwnsAgents {
		agents, err := nestedAgentsText(root, filepath.Join(filepath.Dir(path), claudeAgentsMainFile))
		if err != nil {
			return "", false, false, err
		}
		if agents != "" {
			parts = append(parts, agents)
		}
	}
	if rest != "" {
		parts = append(parts, "::target claude\n"+rest+"\n::end")
	}
	return strings.Join(parts, "\n\n"), rest == "", peerOwnsAgents && rest != "", nil
}

// nestedAgentsText returns a hand-written AGENTS.md, "" when it is absent
// or sync wrote it from specs the project already has.
func nestedAgentsText(root, path string) (string, error) {
	raw, err := readEntryFile(root, path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	if header.Has(string(raw)) {
		return "", nil
	}
	return strings.TrimSpace(string(raw)), nil
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
// -2, -3 suffixes. Headings inside code and raw HTML are ignored so example
// markdown does not fragment the output.
func splitH2Sections(s string) (string, []h2Section) {
	lines := strings.Split(s, "\n")
	type head struct {
		line  int
		title string
	}
	var heads []head
	for _, i := range markdownHeadingLines(lines, 2) {
		if m := h2HeadingRE.FindStringSubmatch(lines[i]); m != nil {
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
	if err := importWriteFile(path, []byte(fm+body+"\n"), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
