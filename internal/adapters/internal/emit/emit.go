// Package emit holds shared helpers for adapter packages: file writing,
// frontmatter rendering, capability reporting, and the two common emission
// patterns (single merged document, one-file-per-rule directory).
package emit

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ContentSum returns the hex sha256 of content, the fingerprint the sync
// ledger records for every output.
func ContentSum(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// MatchesSum reports whether data holds the content whose ContentSum is
// sum, also after folding CRLF to LF: Git writes LF output with CRLF
// under core.autocrlf or an eol rule, which is no hand edit.
func MatchesSum(data []byte, sum string) bool {
	if ContentSum(string(data)) == sum {
		return true
	}
	return strings.Contains(string(data), "\r\n") && ContentSum(strings.ReplaceAll(string(data), "\r\n", "\n")) == sum
}

// Frontmatter renders meta as a YAML frontmatter block. Empty meta returns
// an empty string. Keys are emitted in alphabetical order; callers that
// need to preserve source key ordering should use FrontmatterOrdered.
func Frontmatter(meta map[string]any) string {
	return FrontmatterOrdered(meta, nil)
}

// FrontmatterOrdered renders meta as a YAML frontmatter block with keys
// emitted in the given order. Keys missing from `keys` are appended
// alphabetically. Empty meta returns "".
//
// Equivalent to FrontmatterStyled(meta, keys, nil): when no source
// styles are available the emitter still preserves source key order
// and forces 2-space sequence indent, and still promotes
// single-quoted scalars to double-quoted (yaml.v3's quoted default).
func FrontmatterOrdered(meta map[string]any, keys []string) string {
	return FrontmatterStyled(meta, keys, nil)
}

// FrontmatterStyled renders meta as a YAML frontmatter block. `keys`
// hints at source order; missing keys are appended alphabetically.
// `styles` carries per-key value styles captured at parse time so
// scalars round-trip byte-equivalently:
//
//   - A double-quoted source scalar stays double-quoted.
//   - A plain source scalar stays plain (no auto-promotion to quotes,
//     even when the value contains characters like `<` that look like
//     they want quoting).
//   - Single-quoted source scalars get promoted to double-quoted,
//     matching the convention used by hand-authored CLI configs.
//   - Keys missing from styles fall through to yaml.v3's encoder
//     default, which is plain whenever the value is unambiguous.
//
// Empty meta returns "".
func FrontmatterStyled(meta map[string]any, keys []string, styles map[string]yaml.Style) string {
	if len(meta) == 0 {
		return ""
	}
	values, err := encodeMetaValues(meta)
	if err != nil {
		return ""
	}
	root := &yaml.Node{Kind: yaml.MappingNode}
	for _, k := range orderedMetaKeys(meta, keys) {
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: k}
		valNode := values[k]
		applySourceStyle(valNode, styles[k])
		preferDoubleQuotes(valNode)
		root.Content = append(root.Content, keyNode, valNode)
	}
	var buf strings.Builder
	enc := yaml.NewEncoder(&yamlWriter{b: &buf})
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		_ = enc.Close()
		return ""
	}
	if err := enc.Close(); err != nil {
		return ""
	}
	var out strings.Builder
	out.WriteString("---\n")
	out.WriteString(buf.String())
	out.WriteString("---\n")
	return out.String()
}

// encodeMetaValues encodes every value of meta into a YAML node. Each
// yaml.v3 Node.Encode builds an encoder and a parser, so the whole map
// goes through one of each. A key the parsed document cannot name
// exactly falls back to its own Encode, as does every key when the
// document does not parse back (yaml.v3 writes some keys, such as one
// starting with a tab and spanning lines, in a form it cannot read).
func encodeMetaValues(meta map[string]any) (map[string]*yaml.Node, error) {
	var doc yaml.Node
	values := make(map[string]*yaml.Node, len(meta))
	if doc.Encode(meta) == nil && doc.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(doc.Content); i += 2 {
			key := doc.Content[i]
			if key.Kind != yaml.ScalarNode || key.ShortTag() != "!!str" {
				continue
			}
			if _, inMeta := meta[key.Value]; !inMeta || values[key.Value] != nil {
				clear(values)
				break
			}
			values[key.Value] = doc.Content[i+1]
		}
	}
	for k, v := range meta {
		if _, ok := values[k]; ok {
			continue
		}
		node := &yaml.Node{}
		if err := node.Encode(v); err != nil {
			return nil, err
		}
		values[k] = node
	}
	return values, nil
}

// applySourceStyle stamps a non-zero source style onto a scalar leaf so
// the encoder reproduces the author's quoting. No-op for zero (plain),
// for non-scalar nodes, and for nodes whose value has nested structure
// where forcing a scalar style would be invalid.
func applySourceStyle(n *yaml.Node, style yaml.Style) {
	if n == nil || style == 0 || n.Kind != yaml.ScalarNode {
		return
	}
	n.Style = style
}

// yamlWriter adapts strings.Builder to io.Writer so the YAML encoder
// can stream into it without an intermediate bytes.Buffer.
type yamlWriter struct{ b *strings.Builder }

func (w *yamlWriter) Write(p []byte) (int, error) { return w.b.Write(p) }

// orderedMetaKeys returns the union of meta's keys with `hint` consulted
// first (preserving the caller's preferred order) and any remaining keys
// appended in alphabetical order. Keys in hint that are absent from
// meta are skipped.
func orderedMetaKeys(meta map[string]any, hint []string) []string {
	seen := make(map[string]bool, len(meta))
	out := make([]string, 0, len(meta))
	for _, k := range hint {
		if _, ok := meta[k]; ok && !seen[k] {
			out = append(out, k)
			seen[k] = true
		}
	}
	rest := make([]string, 0, len(meta))
	for k := range meta {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

// preferDoubleQuotes walks a yaml.Node tree and promotes single-quoted
// scalars to double-quoted (yaml.v3 defaults to single quotes when a
// scalar must be quoted, e.g. starts with `[`; CLI authors typically
// use double quotes). Plain scalars stay plain — source-level style
// preservation now flows through MetaStyles / applySourceStyle, so
// the previous angle-bracket auto-promotion is unnecessary and was
// breaking round-trip for hand-authored plain `<ver>` scalars.
func preferDoubleQuotes(n *yaml.Node) {
	if n == nil {
		return
	}
	if n.Kind == yaml.ScalarNode && n.Style == yaml.SingleQuotedStyle {
		n.Style = yaml.DoubleQuotedStyle
	}
	for _, c := range n.Content {
		preferDoubleQuotes(c)
	}
}

// Warner accepts capability warnings. Defaults to writing to os.Stderr.
// Tests inject a buffer to assert messages.
var Warner io.Writer = os.Stderr
