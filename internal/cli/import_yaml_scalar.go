package cli

import (
	"bytes"
	"strings"

	"gopkg.in/yaml.v3"
)

// yamlFrontmatterLine renders a `key: value` frontmatter line whose value
// is YAML-safe. The value is encoded through yaml.v3 so scalars carrying a
// `: ` mapping indicator, quotes, leading/trailing whitespace, or other
// indicator characters come out quoted (or as a block scalar) instead of as
// a bare scalar that the tool's own parser would reject (#413).
//
// The returned line ends in a newline. Plain values that need no quoting
// pass through unchanged, so simple slugs stay readable.
func yamlFrontmatterLine(key, value string) string {
	return key + ": " + encodeYAMLScalar(value) + "\n"
}

// encodeYAMLScalar returns value as a YAML scalar, quoting or block-folding
// only when the raw form would be ambiguous. The yaml encoder decides the
// representation; we strip the document's trailing newline so the caller can
// place it after a `key: ` prefix.
func encodeYAMLScalar(value string) string {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(value); err != nil {
		// Encoding a plain string never fails; fall back to a
		// double-quoted scalar so the line stays valid YAML.
		escaped := strings.ReplaceAll(value, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		return `"` + escaped + `"`
	}
	_ = enc.Close()
	return strings.TrimRight(b.String(), "\n")
}
