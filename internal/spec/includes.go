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
		m := includeLineRe.FindStringSubmatch(line)
		if fence != "" || m == nil {
			continue
		}
		ref := m[1]
		if !filepath.IsLocal(filepath.FromSlash(ref)) {
			return "", errs.Coded(errs.CodeSpecParse, "include @%s: the path must stay inside the project root", ref)
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ref)))
		if err != nil {
			return "", errs.Coded(errs.CodeSpecParse, "include @%s: %w", ref, err)
		}
		lines[i] = strings.TrimRight(string(normalizeLineEndings(data)), "\n")
	}
	return strings.Join(lines, "\n"), nil
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
		entries[i].Body = body
	}
	return nil
}
