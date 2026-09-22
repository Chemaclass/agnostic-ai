// Package mdlink finds and rewrites the relative local-file links in a
// Markdown document. Parsing is bounded to the documented link syntax:
// inline links, images, and reference definitions. YAML frontmatter,
// fenced and indented code blocks, and inline code spans are skipped,
// and external URLs, absolute paths, and fragment-only links are
// dropped. `doctor --check-references` uses it to find links that do
// not resolve, and adapters that move a skill body away from its
// bundled assets use it to point those links back at them.
package mdlink

import (
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

// Link is one local-file link destination found in a document. Dest is
// the decoded path with any fragment or query removed; Raw keeps that
// path as written so diagnostics quote the author's text.
type Link struct {
	Line int
	Dest string
	Raw  string
	// start is the byte offset of Raw in the document.
	start int
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

// Local returns the relative local-file links of doc in document order.
func Local(doc string) []Link {
	lines := strings.Split(doc, "\n")
	offsets := make([]int, len(lines))
	for i, pos := 0, 0; i < len(lines); i++ {
		offsets[i] = pos
		pos += len(lines[i]) + 1
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	start := 0
	if len(lines) > 0 && lines[0] == "---" {
		for i := 1; i < len(lines); i++ {
			if lines[i] == "---" {
				start = i + 1
				break
			}
		}
	}

	var out []Link
	add := func(line, at int, raw string) {
		lead := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if dest, ok := localPath(raw); ok {
			path := trimSuffix(strings.TrimSpace(raw))
			out = append(out, Link{Line: line + 1, Dest: dest, Raw: path, start: offsets[line] + at + lead})
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

		text := blankCodeSpans(line)
		if m := referenceDefinition.FindStringSubmatchIndex(text); m != nil {
			if text[m[2]] != '^' {
				at, end := m[4], m[5]
				if text[at] == '<' {
					at, end = at+1, end-1
				}
				add(i, at, text[at:end])
			}
			continue
		}
		for _, d := range inlineDestinations(text) {
			add(i, d.at, d.raw)
		}
	}
	return out
}

// RewriteLocal replaces the path of every relative local-file link in
// doc for which replace returns true. A fragment or query after the path
// is kept, and everything outside the replaced paths stays byte-for-byte.
func RewriteLocal(doc string, replace func(Link) (string, bool)) string {
	links := Local(doc)
	// Splice from the end so earlier offsets stay valid.
	for i := len(links) - 1; i >= 0; i-- {
		l := links[i]
		if path, ok := replace(l); ok {
			doc = doc[:l.start] + path + doc[l.start+len(l.Raw):]
		}
	}
	return doc
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

// blankCodeSpans replaces every inline code span on one line with
// spaces, so byte offsets still point into the original line. A
// backtick run with no closing run of the same length stays literal.
func blankCodeSpans(line string) string {
	b := []byte(line)
	for i := 0; i < len(line); {
		if line[i] != '`' {
			i++
			continue
		}
		n := 1
		for i+n < len(line) && line[i+n] == '`' {
			n++
		}
		end := closingRun(line[i+n:], n)
		if end < 0 {
			i += n
			continue
		}
		stop := i + n + end + n
		for j := i; j < stop; j++ {
			b[j] = ' '
		}
		i = stop
	}
	return string(b)
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

// destination is one raw inline destination and its offset in the line.
type destination struct {
	raw string
	at  int
}

// inlineDestinations returns the raw destination of every `](dest)` on a
// line: the text inside angle brackets, or the run up to whitespace or
// the unbalanced closing parenthesis.
func inlineDestinations(line string) []destination {
	var out []destination
	for i := 0; i+1 < len(line); i++ {
		if line[i] != ']' || line[i+1] != '(' || (i > 0 && line[i-1] == '\\') {
			continue
		}
		at := i + 2
		rest := line[at:]
		if trimmed := strings.TrimLeft(rest, " \t"); strings.HasPrefix(trimmed, "<") {
			if end := strings.IndexByte(trimmed, '>'); end > 0 {
				out = append(out, destination{raw: trimmed[1:end], at: at + len(rest) - len(trimmed) + 1})
			}
			continue
		}
		depth, end := 0, len(rest)
	scan:
		for j := len(rest) - len(strings.TrimLeft(rest, " \t")); j < len(rest); j++ {
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
		out = append(out, destination{raw: rest[:end], at: at})
	}
	return out
}

// localPath reports whether raw names a relative local file and returns
// its decoded path without fragment or query.
func localPath(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, "/") ||
		strings.HasPrefix(raw, `\`) || urlScheme.MatchString(raw) {
		return "", false
	}
	p := trimSuffix(raw)
	if decoded, err := url.PathUnescape(p); err == nil {
		p = decoded
	}
	if p == "" || filepath.IsAbs(p) {
		return "", false
	}
	return p, true
}

// trimSuffix drops a `#fragment` or `?query` from a destination.
func trimSuffix(raw string) string {
	if i := strings.IndexAny(raw, "#?"); i >= 0 {
		return raw[:i]
	}
	return raw
}
