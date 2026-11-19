package spec

import "strings"

// Globs returns the entry's globs frontmatter as one comma-separated
// string (see JoinGlobs), or "" if missing or malformed.
func (e Entry) Globs() string {
	return JoinGlobs(e.Meta["globs"])
}

// IsCatchAllGlobs reports whether a joined globs value matches every
// file, so it carries no scoping intent.
func IsCatchAllGlobs(g string) bool {
	switch strings.TrimSpace(g) {
	case "", "**/*", "*":
		return true
	}
	return false
}

// JoinGlobs returns a `globs` value as the comma-separated string the
// adapters read: a string as written, a YAML list of strings joined with
// ",", so `["*.go", "*.mod"]` reads as `"*.go,*.mod"`. Any other value,
// including a list holding a non-string, is "" (see ValidGlobs).
func JoinGlobs(v any) string {
	switch g := v.(type) {
	case string:
		return g
	case []string:
		return strings.Join(g, ",")
	case []any:
		parts := make([]string, 0, len(g))
		for _, item := range g {
			s, ok := item.(string)
			if !ok {
				return ""
			}
			parts = append(parts, s)
		}
		return strings.Join(parts, ",")
	}
	return ""
}

// GlobList returns the patterns a `globs` value names: each list item
// as written, or the string split on the commas outside `{...}` brace
// sets, so "src/**/*.{ts,tsx},lib/*.js" names two patterns. Surrounding
// whitespace is trimmed and empty patterns dropped. A malformed value
// (see ValidGlobs) names none.
func GlobList(v any) []string {
	var raw []string
	switch g := v.(type) {
	case string:
		raw = splitOutsideBraces(g)
	case []string:
		raw = g
	case []any:
		if !ValidGlobs(g) {
			return nil
		}
		for _, item := range g {
			raw = append(raw, item.(string))
		}
	}
	var out []string
	for _, p := range raw {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// splitOutsideBraces splits s on each comma that is not inside a brace
// set. A `{` left unclosed opens no set, so s splits on every comma.
func splitOutsideBraces(s string) []string {
	var parts []string
	depth, start := 0, 0
	for i, c := range s {
		switch c {
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	if depth != 0 {
		return strings.Split(s, ",")
	}
	return append(parts, s[start:])
}

// ValidGlobs reports whether v is one of the two forms `globs` accepts:
// a string or a list of strings.
func ValidGlobs(v any) bool {
	switch g := v.(type) {
	case string, []string:
		return true
	case []any:
		for _, item := range g {
			if _, ok := item.(string); !ok {
				return false
			}
		}
		return true
	}
	return false
}
