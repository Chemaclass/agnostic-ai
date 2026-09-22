package cli

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// referenceFinding is one relative Markdown link in an emitted skill
// document whose destination is missing on disk. Source is the canonical
// spec file the document came from, empty when it cannot be attributed.
type referenceFinding struct {
	Target      string `json:"target"`
	Source      string `json:"source,omitempty"`
	Path        string `json:"path"`
	Line        int    `json:"line"`
	Destination string `json:"destination"`
}

// collectReferenceFindings checks every local-file Markdown link in the
// skill documents each selected target emits. A document counts as
// skill-owned when it disappears from the capture once the bundle has no
// skills, so the check follows the adapters' real output layout and never
// scans unrelated repository files. Read-only: adapters run in capture
// mode and only emitted documents already on disk are read.
func collectReferenceFindings(targets []string) (findings []referenceFinding, docs int, err error) {
	cfg, b, err := loadProject(".")
	if err != nil {
		return nil, 0, err
	}
	if len(targets) == 0 {
		targets = cfg.Targets
	}
	adapters.SetWarner(io.Discard)
	defer adapters.SetWarner(os.Stderr)

	sess := adapters.NewSession()
	for _, t := range targets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			continue // collectDrift already warned about the unknown target
		}
		owned, err := skillOwnedDocs(sess, adapter, b, cfg)
		if err != nil {
			return nil, 0, fmt.Errorf("%s: %w", t, err)
		}
		skills := b.For(t).Skills
		for _, p := range owned {
			data, err := os.ReadFile(p)
			if err != nil {
				continue // an absent document is a drift finding, not ours
			}
			docs++
			source, attributed := "", false
			for _, l := range localMarkdownLinks(string(data)) {
				dest := filepath.Join(filepath.Dir(p), filepath.FromSlash(l.Dest))
				if _, err := os.Stat(dest); err == nil {
					continue
				}
				if !attributed {
					source = skillSourceFor(p, skills)
					if source == "" {
						source = skillSourceByRender(sess, adapter, b, cfg, p)
					}
					attributed = true
				}
				findings = append(findings, referenceFinding{
					Target:      t,
					Source:      source,
					Path:        filepath.ToSlash(p),
					Line:        l.Line,
					Destination: l.Raw,
				})
			}
		}
	}
	return findings, docs, nil
}

// skillOwnedDocs returns the Markdown files a target writes only because
// the bundle has skills, sorted by path. Absolute paths (user-scope
// output) and user-owned paths are left out.
func skillOwnedDocs(sess *adapters.Session, adapter adapters.Adapter, b spec.Bundle, cfg *config.Config) ([]string, error) {
	full, err := captureAdapterFiles(sess, adapter, b, cfg)
	if err != nil {
		return nil, err
	}
	noSkills := b
	noSkills.Skills = nil
	without, err := captureAdapterFiles(sess, adapter, noSkills, cfg)
	if err != nil {
		return nil, err
	}
	kept := make(map[string]bool, len(without))
	for _, f := range without {
		kept[f.Path] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, f := range full {
		if kept[f.Path] || seen[f.Path] || filepath.IsAbs(f.Path) || cfg.IsUnmanaged(f.Path) {
			continue
		}
		switch strings.ToLower(filepath.Ext(f.Path)) {
		case ".md", ".mdc", ".markdown":
		default:
			continue
		}
		seen[f.Path] = true
		out = append(out, f.Path)
	}
	sort.Strings(out)
	return out, nil
}

// skillSourceFor attributes an emitted document to its canonical source.
// The first path segment (directory, or file stem for a flattened skill)
// naming a skill picks it; for a folder skill the rest of the path is
// looked up beside its SKILL.md. Best effort: returns "" on no match.
func skillSourceFor(emitted string, skills []spec.Entry) string {
	segs := strings.Split(filepath.ToSlash(emitted), "/")
	last := len(segs) - 1
	for i, seg := range segs {
		if i == last {
			seg = strings.TrimSuffix(seg, filepath.Ext(seg))
		}
		for _, sk := range skills {
			if sk.Name != seg || sk.Path == "" {
				continue
			}
			rel := strings.Join(segs[i+1:], "/")
			if rel != "" && filepath.Base(sk.Path) == "SKILL.md" {
				candidate := filepath.Join(filepath.Dir(sk.Path), filepath.FromSlash(rel))
				if _, err := os.Stat(candidate); err == nil {
					return filepath.ToSlash(candidate)
				}
			}
			return filepath.ToSlash(sk.Path)
		}
	}
	return ""
}

// skillSourceByRender attributes a document whose path names no skill
// (e.g. Continue's `skill-<name>.md` rule) by re-rendering the target
// without each skill in turn until the document disappears. Runs only for
// a document that has a finding, so a clean project pays nothing.
func skillSourceByRender(sess *adapters.Session, adapter adapters.Adapter, b spec.Bundle, cfg *config.Config, emitted string) string {
	for i, sk := range b.Skills {
		minus := b
		minus.Skills = append(append([]spec.Entry{}, b.Skills[:i]...), b.Skills[i+1:]...)
		files, err := captureAdapterFiles(sess, adapter, minus, cfg)
		if err != nil {
			return ""
		}
		if !slices.ContainsFunc(files, func(f adapters.CapturedFile) bool { return f.Path == emitted }) {
			return filepath.ToSlash(sk.Path)
		}
	}
	return ""
}

// reportBrokenReferences prints the doctor section for
// --check-references and returns the number of broken links.
func reportBrokenReferences(cmd *cobra.Command, targets []string) (int, error) {
	cmd.Println()
	cmd.Println("Skill references:")
	findings, docs, err := collectReferenceFindings(targets)
	if err != nil {
		return 0, err
	}
	if len(findings) == 0 {
		cmd.Printf("  ✓ every local link in %d emitted skill document(s) resolves\n", docs)
		return 0, nil
	}
	for _, f := range findings {
		cmd.Printf("  ✗ %s: %s:%d links to missing %s\n", f.Target, f.Path, f.Line, f.Destination)
		if f.Source != "" {
			cmd.Printf("      source: %s\n", f.Source)
		}
	}
	cmd.Println("    fix: add the file to the skill folder under .agnostic-ai/ and run `agnostic-ai sync`, or correct the link in the source")
	return len(findings), nil
}

// markdownLink is one local-file link destination found in a document.
// Dest is the decoded path with any fragment or query removed; Raw keeps
// the destination as written so diagnostics quote the author's text.
type markdownLink struct {
	Line int
	Dest string
	Raw  string
}

var (
	// referenceDefinition matches `[label]: dest` at the start of a line,
	// indented at most three spaces. Footnotes (`[^x]:`) are excluded later.
	referenceDefinition = regexp.MustCompile(`^ {0,3}\[([^\]]+)\]:[ \t]*(<[^>]*>|\S+)`)
	// urlScheme matches `http:`, `mailto:`, `c:` and other schemes.
	urlScheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
	// listItem matches bullet and ordered list markers.
	listItem = regexp.MustCompile(`^ {0,3}([-*+]|\d{1,9}[.)])[ \t]`)
)

// localMarkdownLinks returns the relative local-file destinations of the
// inline links, images, and reference definitions in doc, in document
// order. Parsing is bounded to that documented syntax: YAML frontmatter,
// fenced and indented code blocks, and inline code spans are skipped, and
// external URLs, absolute paths, and fragment-only links are dropped.
func localMarkdownLinks(doc string) []markdownLink {
	lines := strings.Split(strings.ReplaceAll(doc, "\r\n", "\n"), "\n")
	start := 0
	if len(lines) > 0 && lines[0] == "---" {
		for i := 1; i < len(lines); i++ {
			if lines[i] == "---" {
				start = i + 1
				break
			}
		}
	}

	var out []markdownLink
	add := func(line int, raw string) {
		if dest, ok := localLinkPath(raw); ok {
			out = append(out, markdownLink{Line: line, Dest: dest, Raw: trimLinkSuffix(raw)})
		}
	}
	var fence string
	prevBlank, inList, inIndented := true, false, false
	for i := start; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimLeft(line, " ")
		indent := len(line) - len(trimmed)
		if fence != "" {
			if indent < 4 && strings.HasPrefix(trimmed, fence) && strings.Trim(trimmed, fence[:1]+" \t") == "" {
				fence = ""
			}
			continue
		}
		if indent < 4 {
			if f := openingFence(trimmed); f != "" {
				fence = f
				continue
			}
		}
		blank := strings.TrimSpace(line) == ""
		codeIndent := indent >= 4 || strings.HasPrefix(line, "\t")
		if !blank && codeIndent && !inList && (prevBlank || inIndented) {
			inIndented = true
			prevBlank = false
			continue
		}
		if !blank {
			inIndented = false
			if !codeIndent {
				inList = listItem.MatchString(line)
			}
		}
		prevBlank = blank
		if blank {
			continue
		}

		text := stripCodeSpans(line)
		if m := referenceDefinition.FindStringSubmatch(text); m != nil {
			if !strings.HasPrefix(m[1], "^") {
				add(i+1, strings.TrimSuffix(strings.TrimPrefix(m[2], "<"), ">"))
			}
			continue
		}
		for _, raw := range inlineDestinations(text) {
			add(i+1, raw)
		}
	}
	return out
}

// openingFence returns the fence marker (three or more backticks or
// tildes) that opens a fenced code block on this line, or "".
func openingFence(trimmed string) string {
	for _, c := range []string{"`", "~"} {
		n := len(trimmed) - len(strings.TrimLeft(trimmed, c))
		if n >= 3 {
			return strings.Repeat(c, n)
		}
	}
	return ""
}

// stripCodeSpans blanks every inline code span on one line. A backtick
// run with no closing run of the same length stays literal text.
func stripCodeSpans(line string) string {
	var b strings.Builder
	for i := 0; i < len(line); {
		if line[i] != '`' {
			b.WriteByte(line[i])
			i++
			continue
		}
		n := 1
		for i+n < len(line) && line[i+n] == '`' {
			n++
		}
		run := line[i : i+n]
		end := closingRun(line[i+n:], n)
		if end < 0 {
			b.WriteString(run)
			i += n
			continue
		}
		i += n + end + n
	}
	return b.String()
}

// closingRun finds a backtick run of exactly n in s and returns its
// offset, or -1.
func closingRun(s string, n int) int {
	for j := 0; j < len(s); {
		if s[j] != '`' {
			j++
			continue
		}
		k := 1
		for j+k < len(s) && s[j+k] == '`' {
			k++
		}
		if k == n {
			return j
		}
		j += k
	}
	return -1
}

// inlineDestinations returns the raw destination of every `](dest)` on a
// line: the text inside angle brackets, or the run up to whitespace or
// the unbalanced closing parenthesis.
func inlineDestinations(line string) []string {
	var out []string
	for i := 0; i+1 < len(line); i++ {
		if line[i] != ']' || line[i+1] != '(' || (i > 0 && line[i-1] == '\\') {
			continue
		}
		rest := strings.TrimLeft(line[i+2:], " \t")
		if strings.HasPrefix(rest, "<") {
			if end := strings.IndexByte(rest, '>'); end > 0 {
				out = append(out, rest[1:end])
			}
			continue
		}
		depth, end := 0, len(rest)
	scan:
		for j := 0; j < len(rest); j++ {
			switch rest[j] {
			case '\\':
				j++
			case '(':
				depth++
			case ')':
				if depth == 0 {
					end = j
					break scan
				}
				depth--
			case ' ', '\t':
				end = j
				break scan
			}
		}
		out = append(out, rest[:end])
	}
	return out
}

// localLinkPath reports whether raw names a relative local file and
// returns its decoded path without fragment or query.
func localLinkPath(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, "/") ||
		strings.HasPrefix(raw, `\`) || urlScheme.MatchString(raw) {
		return "", false
	}
	p := trimLinkSuffix(raw)
	if decoded, err := url.PathUnescape(p); err == nil {
		p = decoded
	}
	if p == "" || filepath.IsAbs(p) {
		return "", false
	}
	return p, true
}

// trimLinkSuffix drops a `#fragment` or `?query` from a destination.
func trimLinkSuffix(raw string) string {
	if i := strings.IndexAny(raw, "#?"); i >= 0 {
		return raw[:i]
	}
	return raw
}
