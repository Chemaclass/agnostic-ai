package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PersistTargets rewrites the `targets:` block in agnostic.config.yaml
// in place. Comments, header lines (e.g. `yaml-language-server:`), and
// every other key are preserved. The list is rendered with two-space
// indentation matching the format written by `init`.
//
// When the file has no `targets:` key, the block is appended at the
// end. When `targets` is empty, the block is rewritten as an empty list
// (no items); callers should validate non-empty selections upstream.
func PersistTargets(root string, targets []string) error {
	path := filepath.Join(root, "agnostic.config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	out := rewriteTargetsBlock(string(data), targets)
	return os.WriteFile(path, []byte(out), 0o644)
}

func rewriteTargetsBlock(content string, targets []string) string {
	lines := strings.Split(content, "\n")
	start, end := findTargetsBlock(lines)
	block := renderTargetsBlock(targets)
	if start < 0 {
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		return content + "\n" + block
	}
	out := make([]string, 0, len(lines)-(end-start)+1)
	out = append(out, lines[:start]...)
	out = append(out, strings.Split(block, "\n")...)
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n")
}

// findTargetsBlock locates the `targets:` line and the first line at or
// before column 0 that is neither a list item nor blank — that boundary
// marks the end of the block. Returns (-1, -1) when no `targets:` key
// is present.
func findTargetsBlock(lines []string) (start, end int) {
	start = -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimRight(line, " \t"), "targets:") &&
			!strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			start = i
			break
		}
	}
	if start < 0 {
		return -1, -1
	}
	end = len(lines)
	for i := start + 1; i < len(lines); i++ {
		trim := strings.TrimSpace(lines[i])
		if trim == "" {
			continue
		}
		if strings.HasPrefix(lines[i], "  -") || strings.HasPrefix(lines[i], "\t-") {
			continue
		}
		end = i
		break
	}
	return start, end
}

func renderTargetsBlock(targets []string) string {
	var sb strings.Builder
	sb.WriteString("targets:")
	for _, t := range targets {
		sb.WriteString("\n  - ")
		sb.WriteString(t)
	}
	sb.WriteString("\n")
	return sb.String()
}
