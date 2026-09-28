package cli

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// claudeOnlySkillKeys are Claude Code skill and command keys no other
// target reads. At the top level of a source spec lint reports them
// (LINT007), so import moves them under `x-claude:`, which sync
// flattens back into the Claude file at the same position.
var claudeOnlySkillKeys = []string{"allowed-tools"}

var xClaudeBlockRE = regexp.MustCompile(`^x-claude:[ \t]*(#.*)?$`)

// isTopLevelKeyLine reports whether line opens the top-level key, plain
// or quoted, as YAML allows.
func isTopLevelKeyLine(line, key string) bool {
	for _, spelled := range []string{key, "'" + key + "'", `"` + key + `"`} {
		if rest, ok := strings.CutPrefix(line, spelled); ok && strings.HasPrefix(strings.TrimLeft(rest, " \t"), ":") {
			return true
		}
	}
	return false
}

// moveClaudeOnlyKeysInFile applies moveClaudeOnlyKeys to the file at path.
func moveClaudeOnlyKeysInFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	out := moveClaudeOnlyKeys(string(data))
	if out == string(data) {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	return importWriteFile(path, []byte(out), info.Mode().Perm())
}

// moveClaudeOnlyKeys moves each top-level claudeOnlySkillKeys entry of a
// Markdown document's frontmatter under `x-claude:`, editing the text so
// every other line stays as written. The document comes back unchanged
// when the move cannot be proven safe: an inline `x-claude` value, a key
// `x-claude` already sets, or a result that does not parse to the same
// data with the keys moved.
func moveClaudeOnlyKeys(doc string) string {
	if !strings.HasPrefix(doc, "---\n") {
		return doc
	}
	rest := doc[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return doc
	}
	lines := strings.Split(rest[:end], "\n")

	var before map[string]any
	if err := yaml.Unmarshal([]byte(rest[:end]), &before); err != nil || before == nil {
		return doc
	}
	existing, hasX := before["x-claude"]
	xMap, _ := existing.(map[string]any)
	if hasX && xMap == nil {
		return doc
	}

	xStart, xEnd := -1, -1
	for i, line := range lines {
		if xClaudeBlockRE.MatchString(line) {
			xStart, xEnd = i, frontmatterBlockEnd(lines, i)
		}
	}
	if hasX && xStart < 0 {
		return doc
	}
	indent := "  "
	if xStart >= 0 {
		for _, line := range lines[xStart+1 : xEnd] {
			if trimmed := strings.TrimLeft(line, " "); trimmed != "" && len(trimmed) < len(line) {
				indent = line[:len(line)-len(trimmed)]
				break
			}
		}
	}

	var moved []string
	drop := map[int]bool{}
	first := -1
	for _, key := range claudeOnlySkillKeys {
		if _, ok := before[key]; !ok {
			continue
		}
		if _, set := xMap[key]; set {
			continue
		}
		for i, line := range lines {
			if !isTopLevelKeyLine(line, key) {
				continue
			}
			stop := frontmatterBlockEnd(lines, i)
			for j := i; j < stop; j++ {
				drop[j] = true
				if lines[j] == "" {
					moved = append(moved, "")
				} else {
					moved = append(moved, indent+lines[j])
				}
			}
			if first < 0 || i < first {
				first = i
			}
			break
		}
	}
	if len(moved) == 0 {
		return doc
	}

	var out []string
	for i, line := range lines {
		if xStart < 0 && i == first {
			out = append(out, "x-claude:")
			out = append(out, moved...)
		}
		if !drop[i] {
			out = append(out, line)
		}
		if xStart >= 0 && i == xEnd-1 {
			out = append(out, moved...)
		}
	}
	front := strings.Join(out, "\n")

	var after map[string]any
	if err := yaml.Unmarshal([]byte(front), &after); err != nil {
		return doc
	}
	if !reflect.DeepEqual(after, withClaudeOnlyKeysMoved(before)) {
		return doc
	}
	return "---\n" + front + rest[end:]
}

// frontmatterBlockEnd returns the index just past the top-level key that
// starts at lines[start]: its indented lines, a block sequence written at
// the key's own indentation, and blank lines inside them.
func frontmatterBlockEnd(lines []string, start int) int {
	end := start + 1
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "-") {
			break
		}
		end = i + 1
	}
	return end
}

// withClaudeOnlyKeysMoved is the data moveClaudeOnlyKeys must produce.
func withClaudeOnlyKeysMoved(meta map[string]any) map[string]any {
	out := make(map[string]any, len(meta))
	x := map[string]any{}
	if existing, ok := meta["x-claude"].(map[string]any); ok {
		for k, v := range existing {
			x[k] = v
		}
	}
	for k, v := range meta {
		out[k] = v
	}
	for _, key := range claudeOnlySkillKeys {
		v, ok := meta[key]
		if !ok {
			continue
		}
		if _, set := x[key]; set {
			continue
		}
		x[key] = v
		delete(out, key)
	}
	out["x-claude"] = x
	return out
}
