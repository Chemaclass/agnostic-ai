package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

const (
	// codexExecPoliciesFile is the on-disk Skylark file Codex CLI reads.
	codexExecPoliciesFile = ".codex/rules/default.rules"

	// codexExecPoliciesOverlayFile is the captured YAML representation
	// the codex emitter loads when no inline `outputs.codex.exec-policies`
	// or explicit `exec-policies-file` is set. Mirrors the
	// `codex.config.toml` overlay pattern so a wipe of `.codex/` between
	// `import` and `sync` does not silently drop the user's exec
	// policies.
	codexExecPoliciesOverlayFile = "codex.exec-policies.yaml"
)

// codexExecPoliciesOverlayPath returns the project-relative path to the
// captured exec-policies overlay.
func codexExecPoliciesOverlayPath(root string) string {
	return filepath.Join(root, agnosticOverlayDir, codexExecPoliciesOverlayFile)
}

// importCodexExecPolicies reads `.codex/rules/default.rules` (if present)
// and writes the parsed `prefix_rule(...)` calls to
// `.agnostic-ai/overlays/codex.exec-policies.yaml` so a subsequent
// `sync -t codex` re-emits the same content. Returns true when an overlay
// was written (used by the import summary printer).
func importCodexExecPolicies(root string) (bool, error) {
	src := filepath.Join(root, codexExecPoliciesFile)
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%s: %w", src, err)
	}
	policies, err := parseCodexExecPolicies(string(data))
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", src, err)
	}
	if len(policies) == 0 {
		return false, nil
	}
	out, err := yaml.Marshal(policies)
	if err != nil {
		return false, fmt.Errorf("marshal exec-policies overlay: %w", err)
	}
	dst := codexExecPoliciesOverlayPath(root)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return false, fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	if err := importWriteFile(dst, out, 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", dst, err)
	}
	return true, nil
}

// parseCodexExecPolicies extracts every `prefix_rule(...)` call from a
// Skylark exec-policy file. Justification = the contiguous block of `#`
// comment lines immediately above a call. Match examples = `# match: ...`
// lines immediately below a call. Anything else passes through as
// unstructured noise that the YAML overlay drops on re-emit.
func parseCodexExecPolicies(body string) ([]config.CodexExecPolicy, error) {
	var out []config.CodexExecPolicy

	lines := strings.Split(body, "\n")
	i := 0
	for i < len(lines) {
		// Collect a contiguous block of leading `#` comments (not # match:)
		// as the justification candidate.
		justification := collectCommentBlock(lines, &i)

		if i >= len(lines) {
			break
		}
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "prefix_rule") {
			i++
			continue
		}
		// Re-join from this line forward and find the matching ')'.
		start := strings.Index(strings.Join(lines[i:], "\n"), "prefix_rule")
		if start < 0 {
			i++
			continue
		}
		rest := strings.Join(lines[i:], "\n")[start:]
		call, consumed, err := extractParenCall(rest)
		if err != nil {
			return nil, err
		}
		policy, err := parsePrefixRuleCall(call)
		if err != nil {
			return nil, fmt.Errorf("prefix_rule call: %w", err)
		}
		if justification != "" {
			policy.Justification = justification
		}

		// Advance the line cursor past the consumed text.
		consumedLines := strings.Count(rest[:consumed], "\n")
		i += consumedLines + 1

		// Collect any trailing `# match: ...` lines, but only when the
		// call itself did not declare inline `match = [...]`. Inline
		// kwargs sit closer to the rule and win.
		if len(policy.Match) == 0 {
			policy.Match = collectMatchLines(lines, &i)
		}

		out = append(out, policy)
	}
	return out, nil
}

// collectCommentBlock advances *i over a contiguous block of `# ...`
// lines and returns the joined text minus the leading `# ` prefix.
// Blank lines and `# match:` lines terminate the block. `# match:`
// belongs to the previous rule, not the next one.
func collectCommentBlock(lines []string, i *int) string {
	var parts []string
	for *i < len(lines) {
		line := strings.TrimSpace(lines[*i])
		if line == "" {
			// blank resets the comment block
			parts = nil
			*i++
			continue
		}
		if !strings.HasPrefix(line, "#") {
			break
		}
		body := strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if strings.HasPrefix(body, "match:") {
			// match comment is not justification; do not consume.
			break
		}
		parts = append(parts, body)
		*i++
	}
	return strings.Join(parts, "\n")
}

// collectMatchLines reads the `# match: <example>` lines that follow a
// rule and returns the example strings (trimmed). Stops at the first
// non-match line.
func collectMatchLines(lines []string, i *int) []string {
	var out []string
	for *i < len(lines) {
		line := strings.TrimSpace(lines[*i])
		if line == "" {
			*i++
			continue
		}
		if !strings.HasPrefix(line, "#") {
			break
		}
		body := strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if !strings.HasPrefix(body, "match:") {
			break
		}
		out = append(out, strings.TrimSpace(strings.TrimPrefix(body, "match:")))
		*i++
	}
	return out
}

// extractParenCall reads from a `prefix_rule(` opener through the matching
// `)`. Returns the entire `prefix_rule(...)` substring and the number of
// bytes consumed in s. Quoted strings are skipped so a `")"` inside a
// pattern element does not falsely close the call.
func extractParenCall(s string) (string, int, error) {
	depth := 0
	inString := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			if c == '\\' && i+1 < len(s) {
				i++
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[:i+1], i + 1, nil
			}
		}
	}
	return "", 0, fmt.Errorf("unterminated prefix_rule call")
}

// parsePrefixRuleCall extracts pattern, decision, justification, and
// match from the body of a `prefix_rule(...)` call. Justification +
// match can also be expressed as comment blocks around the call (handled
// by parseCodexExecPolicies); inline kwargs found here take precedence
// because they sit closer to the rule they describe.
func parsePrefixRuleCall(call string) (config.CodexExecPolicy, error) {
	inner := call
	if i := strings.Index(inner, "("); i >= 0 {
		inner = inner[i+1:]
	}
	if j := strings.LastIndex(inner, ")"); j >= 0 {
		inner = inner[:j]
	}
	pattern, err := readPrefixRulePattern(inner)
	if err != nil {
		return config.CodexExecPolicy{}, err
	}
	decision, err := readPrefixRuleDecision(inner)
	if err != nil {
		return config.CodexExecPolicy{}, err
	}
	return config.CodexExecPolicy{
		Pattern:       pattern,
		Decision:      decision,
		Justification: readPrefixRuleJustification(inner),
		Match:         readPrefixRuleMatch(inner),
	}, nil
}

var patternRE = regexp.MustCompile(`pattern\s*=\s*\[([^\]]*)\]`)
var decisionRE = regexp.MustCompile(`decision\s*=\s*"([^"]*)"`)
var justificationRE = regexp.MustCompile(`justification\s*=\s*"((?:[^"\\]|\\.)*)"`)
var matchRE = regexp.MustCompile(`match\s*=\s*\[([^\]]*)\]`)

// readPrefixRulePattern pulls the `pattern = [...]` list from a call
// body, splitting the contents on comma into one string per element.
// Empty patterns return an empty slice; the caller decides whether to
// reject them.
func readPrefixRulePattern(call string) ([]string, error) {
	m := patternRE.FindStringSubmatch(call)
	if m == nil {
		return nil, fmt.Errorf("pattern = [...] not found")
	}
	return splitStringList(m[1]), nil
}

// readPrefixRuleDecision pulls the `decision = "<word>"` field.
func readPrefixRuleDecision(call string) (string, error) {
	m := decisionRE.FindStringSubmatch(call)
	if m == nil {
		return "", fmt.Errorf(`decision = "..." not found`)
	}
	return m[1], nil
}

// readPrefixRuleJustification pulls the optional `justification = "..."`
// field, unescaping the backslash sequences Skylark allows inside a
// double-quoted string. Returns "" when the field is absent.
func readPrefixRuleJustification(call string) string {
	m := justificationRE.FindStringSubmatch(call)
	if m == nil {
		return ""
	}
	return unescapeSkylarkString(m[1])
}

// readPrefixRuleMatch pulls the optional `match = [...]` list. Returns
// nil when absent so the YAML overlay omits the field entirely on
// re-serialization (matches the emitter's `omitempty`).
func readPrefixRuleMatch(call string) []string {
	m := matchRE.FindStringSubmatch(call)
	if m == nil {
		return nil
	}
	items := splitStringList(m[1])
	if len(items) == 0 {
		return nil
	}
	return items
}

// unescapeSkylarkString unwraps the common `\"`, `\\`, `\n` escapes from
// a double-quoted Skylark string literal. Any other escape sequence
// (e.g. `\x41`) passes through verbatim so we do not need a full Skylark
// lexer just to capture justification text.
func unescapeSkylarkString(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			b.WriteByte(c)
			continue
		}
		next := s[i+1]
		switch next {
		case '"', '\\':
			b.WriteByte(next)
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		default:
			b.WriteByte(c)
			b.WriteByte(next)
		}
		i++
	}
	return b.String()
}

// splitStringList splits a comma-separated list of double-quoted Skylark
// strings into the unquoted values. Whitespace between tokens ignored.
// Backslash escapes within a string are unwrapped (\" → ").
func splitStringList(body string) []string {
	var out []string
	r := bufio.NewReader(strings.NewReader(body))
	var cur strings.Builder
	inString := false
	for {
		c, _, err := r.ReadRune()
		if err != nil {
			break
		}
		if inString {
			if c == '\\' {
				nxt, _, err2 := r.ReadRune()
				if err2 == nil {
					cur.WriteRune(nxt)
				}
				continue
			}
			if c == '"' {
				inString = false
				out = append(out, cur.String())
				cur.Reset()
				continue
			}
			cur.WriteRune(c)
			continue
		}
		if c == '"' {
			inString = true
		}
	}
	return out
}
