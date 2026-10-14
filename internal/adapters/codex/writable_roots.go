package codex

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
)

const (
	sandboxTableHeader = "[sandbox_workspace_write]"
	writableRootsKey   = "writable_roots"
)

// RemoveWritableRoots drops each root in drop from the writable_roots of
// the [sandbox_workspace_write] table in raw, and leaves every other byte
// as it was. A table left empty goes too. It reports false when the roots
// sit in a form this edit does not read (a dotted key or an inline table),
// so the caller keeps its own copy of the roots.
func RemoveWritableRoots(raw string, drop []string) (string, bool) {
	drop = slashed(drop)
	return editWritableRoots(raw, func(roots []string) []string {
		return slices.DeleteFunc(slices.Clone(roots), func(root string) bool {
			return slices.Contains(drop, filepath.ToSlash(root))
		})
	}, false)
}

// addWritableRoots appends the roots raw does not hold to the writable_roots
// of its [sandbox_workspace_write] table. It reports false when raw has no
// such table or holds the roots in a form this edit does not read.
func addWritableRoots(raw string, add []string) (string, bool) {
	if len(add) == 0 {
		return raw, true
	}
	return editWritableRoots(raw, func(roots []string) []string {
		out := slices.Clone(roots)
		for _, root := range add {
			if !slices.Contains(slashed(out), filepath.ToSlash(root)) {
				out = append(out, root)
			}
		}
		return out
	}, true)
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

func editWritableRoots(raw string, edit func([]string) []string, insert bool) (string, bool) {
	var before writableRootsDoc
	meta, err := toml.Decode(raw, &before)
	if err != nil {
		return raw, false
	}
	held := meta.IsDefined("sandbox_workspace_write", writableRootsKey)
	if !held && !insert {
		return raw, true
	}
	want := edit(before.Sandbox.WritableRoots)
	if held && slices.Equal(want, before.Sandbox.WritableRoots) {
		return raw, true
	}

	lines := strings.Split(raw, "\n")
	header, end := findSandboxTable(lines)
	if header < 0 {
		return raw, false
	}
	keyStart, keyEnd := findWritableRoots(lines, header+1, end)
	if (keyStart < 0) == held {
		return raw, false
	}

	var out []string
	switch {
	case len(want) == 0 && tableOnlyHolds(lines, header, end, keyStart, keyEnd):
		out = append(slices.Clone(lines[:header]), lines[end:]...)
	case len(want) == 0:
		out = append(slices.Clone(lines[:keyStart]), lines[keyEnd:]...)
	case keyStart < 0:
		out = append(slices.Clone(lines[:header+1]), renderWritableRoots(want))
		out = append(out, lines[header+1:]...)
	default:
		out = append(slices.Clone(lines[:keyStart]), renderWritableRoots(want))
		out = append(out, lines[keyEnd:]...)
	}
	text := strings.Join(out, "\n")

	var after writableRootsDoc
	if _, err := toml.Decode(text, &after); err != nil || !slices.Equal(after.Sandbox.WritableRoots, want) {
		return raw, false
	}
	return text, true
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
		trimmed := strings.TrimSpace(line)
		isHeader := strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")
		switch {
		case header < 0 && trimmed == sandboxTableHeader:
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
