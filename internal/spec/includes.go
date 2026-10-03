package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// includeLineRe matches a line holding only `@path`. Prose that merely
// mentions a handle never matches, because the whole line must be the token.
var includeLineRe = regexp.MustCompile(`^[ \t]*@(\S+)[ \t]*$`)

// resolveIncludes replaces each line of body that holds only `@path` with
// the content of that file, read from root. Lines inside a fenced code
// block stay as written. An included file is not searched for further
// includes. A path that is absolute or leaves root, or a file that cannot
// be read, is an error, so a dangling reference fails the load instead of
// shipping as text.
func resolveIncludes(body, root string) (string, error) {
	lines := strings.Split(body, "\n")
	for _, inc := range includeLines(lines) {
		ref := inc.ref
		if !filepath.IsLocal(filepath.FromSlash(ref)) {
			return "", errs.Coded(errs.CodeSpecParse, "include @%s: the path must stay inside the project root", ref)
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ref)))
		if err != nil {
			return "", errs.Coded(errs.CodeSpecParse, "include @%s: %w", ref, err)
		}
		lines[inc.line] = strings.TrimRight(string(normalizeLineEndings(data)), "\n")
	}
	return strings.Join(lines, "\n"), nil
}

// IncludeRefs returns the project-relative paths that body's `@path`
// lines inline, cleaned, skipping any that leave the project root.
func IncludeRefs(body string) []string {
	var out []string
	for _, inc := range includeLines(strings.Split(body, "\n")) {
		if filepath.IsLocal(filepath.FromSlash(inc.ref)) {
			out = append(out, filepath.ToSlash(filepath.Clean(filepath.FromSlash(inc.ref))))
		}
	}
	return out
}

// StripFrontmatter returns a spec file's body, without the YAML
// frontmatter block when one opens the file.
func StripFrontmatter(data string) string {
	data = string(normalizeLineEndings([]byte(data)))
	if !strings.HasPrefix(data, "---\n") {
		return data
	}
	if end := strings.Index(data[4:], "\n---"); end >= 0 {
		rest := data[4+end+4:]
		return strings.TrimPrefix(rest, "\n")
	}
	return data
}

type includeLine struct {
	line int
	ref  string
}

// includeLines finds the lines holding only `@path`, outside fenced code
// blocks.
func includeLines(lines []string) []includeLine {
	var out []includeLine
	fence := ""
	for i, line := range lines {
		if marker := fenceMarker(line); marker != "" {
			switch {
			case fence == "":
				fence = marker
			case strings.HasPrefix(marker, fence):
				fence = ""
			}
			continue
		}
		if m := includeLineRe.FindStringSubmatch(line); fence == "" && m != nil {
			out = append(out, includeLine{i, m[1]})
		}
	}
	return out
}

// fenceMarker returns the run of three or more backticks or tildes that
// opens or closes a fenced code block on line, or "".
func fenceMarker(line string) string {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 || len(trimmed) < 3 {
		return ""
	}
	char := trimmed[0]
	if char != '`' && char != '~' {
		return ""
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == char {
		n++
	}
	if n < 3 {
		return ""
	}
	return trimmed[:n]
}

func resolveEntryIncludes(entries []Entry, root string) error {
	for i := range entries {
		body, err := resolveIncludes(entries[i].Body, root)
		if err != nil {
			return fmt.Errorf("%s: %w", entries[i].Path, err)
		}
		if body != entries[i].Body {
			entries[i].Body, entries[i].BodyLine = body, 0
		}
	}
	return nil
}
