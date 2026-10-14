package codex

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
)

const (
	sandboxTable     = "sandbox_workspace_write"
	writableRootsKey = "writable_roots"
)

// RemoveWritableRoots drops each root that drop accepts from the writable_roots of
// the [sandbox_workspace_write] table in raw, and leaves every other byte
// as it was. A table left empty goes too. It reports false when the roots
// sit in a form this edit does not read (a dotted key or an inline table),
// so the caller keeps its own copy of the roots. lostComments reports that
// the array had to be rewritten on one line over comments of its own.
func RemoveWritableRoots(raw string, drop func(root string) bool) (text string, lostComments, ok bool) {
	return editWritableRoots(raw, func(roots []string) []string {
		return slices.DeleteFunc(slices.Clone(roots), drop)
	}, false, drop)
}

// addWritableRoots appends the roots raw does not hold to the writable_roots
// of its [sandbox_workspace_write] table. It reports false when raw has no
// such table or holds the roots in a form this edit does not read.
func addWritableRoots(raw string, add []string) (string, bool) {
	if len(add) == 0 {
		return raw, true
	}
	text, _, ok := editWritableRoots(raw, func(roots []string) []string {
		out := slices.Clone(roots)
		for _, root := range add {
			if !slices.Contains(slashed(out), filepath.ToSlash(root)) {
				out = append(out, root)
			}
		}
		return out
	}, true, nil)
	return text, ok
}

// noteOverlayGrants warns about each root of the overlay that sync grants
// by itself: it names one machine, and a project that shares the overlay
// should not carry it.
func noteOverlayGrants(overlay string, grants []string) {
	var doc writableRootsDoc
	if _, err := toml.Decode(overlay, &doc); err != nil {
		return
	}
	for _, root := range doc.Sandbox.WritableRoots {
		if slices.Contains(slashed(grants), filepath.ToSlash(root)) {
			emit.NoteProject(fmt.Sprintf("codex: %s lists %q, which sync grants by itself and which names one machine; remove it from that overlay", configOverlayPath, filepath.ToSlash(root)))
		}
	}
}

func slashed(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.ToSlash(p)
	}
	return out
}

type writableRootsDoc struct {
	Sandbox struct {
		WritableRoots []string `toml:"writable_roots"`
	} `toml:"sandbox_workspace_write"`
}

func editWritableRoots(raw string, edit func([]string) []string, insert bool, drop func(string) bool) (text string, lostComments, ok bool) {
	var before writableRootsDoc
	meta, err := toml.Decode(raw, &before)
	if err != nil {
		return raw, false, false
	}
	held := meta.IsDefined(sandboxTable, writableRootsKey)
	if !held && !insert {
		return raw, false, true
	}
	want := edit(before.Sandbox.WritableRoots)
	if held && slices.Equal(want, before.Sandbox.WritableRoots) {
		return raw, false, true
	}

	lines := strings.Split(raw, "\n")
	header, end := findSandboxTable(lines)
	if header < 0 {
		return raw, false, false
	}
	keyStart, keyEnd := findWritableRoots(lines, header+1, end)
	if (keyStart < 0) == held {
		return raw, false, false
	}

	eol := ""
	if keyStart >= 0 && strings.HasSuffix(lines[keyEnd-1], "\r") || keyStart < 0 && strings.HasSuffix(lines[header], "\r") {
		eol = "\r"
	}
	var out []string
	lost := false
	switch {
	case insert && held && appendToArray(lines, keyStart, keyEnd, want[len(before.Sandbox.WritableRoots):], eol) != nil:
		out = appendToArray(lines, keyStart, keyEnd, want[len(before.Sandbox.WritableRoots):], eol)
	case len(want) == 0 && tableOnlyHolds(lines, header, end, keyStart, keyEnd):
		out = append(slices.Clone(lines[:header]), lines[end:]...)
	case len(want) == 0:
		out = append(slices.Clone(lines[:keyStart]), lines[keyEnd:]...)
	case drop != nil && dropEntryLines(lines, keyStart, keyEnd, drop) != nil:
		out = dropEntryLines(lines, keyStart, keyEnd, drop)
	case keyStart < 0:
		out = append(slices.Clone(lines[:header+1]), renderWritableRoots(want)+eol)
		out = append(out, lines[header+1:]...)
	default:
		lost = commentIn(lines[keyStart:keyEnd])
		out = append(slices.Clone(lines[:keyStart]), renderWritableRoots(want)+eol)
		out = append(out, lines[keyEnd:]...)
	}
	text = strings.Join(out, "\n")

	var after writableRootsDoc
	if _, err := toml.Decode(text, &after); err != nil || !slices.Equal(after.Sandbox.WritableRoots, want) {
		return raw, false, false
	}
	return text, lost, true
}

// appendToArray returns lines with each root on a line of its own before
// the bracket closing a multi-line writable_roots array, in the layout of
// its last entry. It returns nil for an array it cannot extend that way.
func appendToArray(lines []string, keyStart, keyEnd int, roots []string, eol string) []string {
	last := keyEnd - 1
	if last-keyStart < 2 || !strings.HasPrefix(strings.TrimSpace(lines[last]), "]") || len(roots) == 0 {
		return nil
	}
	entry := lines[last-1]
	code := strings.TrimRight(entry[:codeEnd(entry)], " \t\r")
	if !strings.HasSuffix(code, ",") || strings.TrimSpace(code) == "" {
		return nil
	}
	indent := entry[:len(entry)-len(strings.TrimLeft(entry, " \t"))]
	out := slices.Clone(lines[:last])
	for _, root := range roots {
		out = append(out, indent+strings.TrimSuffix(strings.TrimPrefix(renderWritableRoots([]string{root}), writableRootsKey+" = ["), "]")+","+eol)
	}
	return append(out, lines[last:]...)
}

// dropEntryLines returns lines without the lines of the entries of a
// multi-line writable_roots array that drop accepts, so the rest keeps its
// layout and comments. It returns nil unless the array opens and closes on
// lines of their own and holds one entry per line.
func dropEntryLines(lines []string, keyStart, keyEnd int, drop func(string) bool) []string {
	_, value, _ := strings.Cut(lines[keyStart], "=")
	if keyEnd-keyStart < 3 || strings.TrimSpace(value[:codeEnd(value)]) != "[" ||
		strings.TrimSpace(lines[keyEnd-1][:codeEnd(lines[keyEnd-1])]) != "]" {
		return nil
	}
	out := slices.Clone(lines[:keyStart+1])
	for _, line := range lines[keyStart+1 : keyEnd-1] {
		code := strings.TrimSpace(line[:codeEnd(line)])
		if code == "" {
			out = append(out, line)
			continue
		}
		var entry struct {
			Roots []string `toml:"v"`
		}
		if _, err := toml.Decode("v = ["+code+"]", &entry); err != nil || len(entry.Roots) != 1 {
			return nil
		}
		if !drop(entry.Roots[0]) {
			out = append(out, line)
		}
	}
	return append(out, lines[keyEnd-1:]...)
}

func commentIn(lines []string) bool {
	for _, line := range lines {
		if codeEnd(line) < len(line) {
			return true
		}
	}
	return false
}

func renderWritableRoots(roots []string) string {
	var sb strings.Builder
	emit.WriteTOMLStringArray(&sb, writableRootsKey, roots)
	return strings.TrimSuffix(sb.String(), "\n")
}

// findSandboxTable returns the header line of the [sandbox_workspace_write]
// table and the line where the next table starts, or -1.
func findSandboxTable(lines []string) (header, end int) {
	header, inString := -1, ""
	for i, line := range lines {
		if inString != "" {
			if strings.Contains(line, inString) {
				inString = ""
			}
			continue
		}
		inString = openTripleQuote(line)
		trimmed := strings.TrimSpace(line[:codeEnd(line)])
		isHeader := strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")
		switch {
		case header < 0 && isSandboxHeader(trimmed):
			header = i
		case header >= 0 && isHeader:
			return header, i
		}
	}
	if header < 0 {
		return -1, -1
	}
	return header, len(lines)
}

// isSandboxHeader reports whether a line, without its comment, opens the
// sandbox table, quoted or not.
func isSandboxHeader(trimmed string) bool {
	inner, ok := strings.CutPrefix(trimmed, "[")
	if !ok || strings.HasPrefix(inner, "[") {
		return false
	}
	inner = strings.TrimSpace(strings.TrimSuffix(inner, "]"))
	if len(inner) >= 2 && (inner[0] == '"' || inner[0] == '\'') && inner[len(inner)-1] == inner[0] {
		inner = inner[1 : len(inner)-1]
	}
	return inner == sandboxTable
}

// codeEnd returns the index where the comment of line starts, or its length.
func codeEnd(line string) int {
	var quote byte
	for i := 0; i < len(line); i++ {
		switch c := line[i]; {
		case quote == '"' && c == '\\':
			i++
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#':
			return i
		}
	}
	return len(line)
}

func openTripleQuote(line string) string {
	for _, delim := range []string{`"""`, `'''`} {
		if first := strings.Index(line, delim); first >= 0 && first == strings.LastIndex(line, delim) {
			return delim
		}
	}
	return ""
}

// findWritableRoots returns the lines of the writable_roots assignment in
// lines[from:to], from its key to the bracket closing its array.
func findWritableRoots(lines []string, from, to int) (start, end int) {
	for i := from; i < to; i++ {
		name, value, found := strings.Cut(lines[i], "=")
		if !found || strings.TrimSpace(name) != writableRootsKey {
			continue
		}
		depth, opened := 0, false
		for j := i; j < to; j++ {
			text := lines[j]
			if j == i {
				text = value
			}
			depth, opened = scanArray(text, depth, opened)
			if opened && depth == 0 {
				return i, j + 1
			}
		}
		return -1, -1
	}
	return -1, -1
}

// scanArray tracks bracket depth across text, skipping strings and comments.
func scanArray(text string, depth int, opened bool) (int, bool) {
	var quote byte
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case quote == '"' && c == '\\':
			i++
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#':
			return depth, opened
		case c == '[':
			depth++
			opened = true
		case c == ']':
			depth--
		}
	}
	return depth, opened
}

func tableOnlyHolds(lines []string, header, end, keyStart, keyEnd int) bool {
	for i := header + 1; i < end; i++ {
		if i >= keyStart && i < keyEnd {
			continue
		}
		if trimmed := strings.TrimSpace(lines[i]); trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			return false
		}
	}
	return true
}
