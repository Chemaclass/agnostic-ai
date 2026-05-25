package cli

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// plainScalarLineRE matches a `key: value` line whose value is a plain
// (un-quoted, non-block) YAML scalar. The capture groups are indent,
// key, and value. Lines whose value begins with `"`, `'`, `|`, `>`, `[`,
// or `{` are skipped to avoid touching quoted scalars, block scalars,
// or flow collections. Blank-value lines are skipped too — they are
// container mappings, not scalar assignments.
var plainScalarLineRE = regexp.MustCompile(`^([\t ]*)([A-Za-z0-9_-]+):[\t ]+([^"'|>\[{\n][^\n]*)$`)

// quoteHashInPlainScalars rewrites every plain-scalar frontmatter line
// whose value contains '#' as `key: "<escaped>"`. Strict YAML treats
// '#' preceded by whitespace as a comment delimiter and silently drops
// the rest of the line; the codex CLI's relaxed parser does not, so an
// unquoted `description: ... #number ...` survives in codex but gets
// truncated the moment `yaml.Unmarshal` sees it (#317).
//
// Lines already wrapped in quotes, block scalars (`|`/`>`), and flow
// collections pass through unchanged. Lines whose value starts with a
// `#` (pure comments masquerading as values) are left alone.
func quoteHashInPlainScalars(front string) string {
	lines := strings.Split(front, "\n")
	changed := false
	for i, line := range lines {
		m := plainScalarLineRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		indent, key, val := m[1], m[2], strings.TrimRight(m[3], " \t")
		if !strings.ContainsRune(val, '#') {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(val), "#") {
			continue
		}
		escaped := strings.ReplaceAll(val, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		lines[i] = indent + key + `: "` + escaped + `"`
		changed = true
	}
	if !changed {
		return front
	}
	return strings.Join(lines, "\n")
}

// quoteHashInSkillFrontmatter reads the SKILL.md at path and rewrites
// any plain-scalar frontmatter values that contain '#' as double-
// quoted strings. A no-op when the file lacks frontmatter or no values
// need quoting.
//
// Called immediately after a fresh codex skill copy so the resulting
// agnostic source survives a strict-YAML round-trip without losing the
// half-string after '#' (#317).
func quoteHashInSkillFrontmatter(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	front, body, ok := splitCodexAgentFrontmatter(string(data))
	if !ok {
		return nil
	}
	quoted := quoteHashInPlainScalars(front)
	if quoted == front {
		return nil
	}
	out := "---\n" + strings.TrimRight(quoted, "\n") + "\n---\n\n" + body
	return importWriteFile(path, []byte(out), 0o644)
}
