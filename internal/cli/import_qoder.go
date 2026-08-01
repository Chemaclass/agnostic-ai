package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

const (
	// qoderRulesDir is the native per-rule directory the qoder adapter
	// writes; the plain heading-form files there (no frontmatter) go
	// through the same generic walker other rules-only imports use.
	qoderRulesDir = ".qoder/rules"
	// qoderAgentsDir is the native per-agent directory the qoder adapter
	// writes (docs.qoder.com/extensions/subagent).
	qoderAgentsDir = ".qoder/agents"
)

// importFromQoder reads an existing Qoder project (`.qoder/rules/*.md`
// and `.qoder/agents/*.md`) under root and writes specs into the
// configured source directories.
func importFromQoder(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents); err != nil {
		return err
	}
	c, err := importRulesDirectory(root, qoderRulesDir, src)
	if err != nil {
		return err
	}
	agents, err := importQoderAgents(root, filepath.Join(root, src.Agents))
	if err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents (from qoder)\n", c.rules, agents)
	printImportNextSteps(root, "qoder")
	return nil
}

// importQoderAgents copies every `.qoder/agents/<name>.md` file into
// the agents source dir, rewriting `tools:` from Qoder's comma-
// separated string (`tools: Read, Grep, Bash`) into agnostic-ai's
// generic list form (`tools: [Read, Grep, Bash]`) along the way. Every
// other frontmatter key (name, description, model, skills, mcpServers,
// x-qoder, ...) copies through unchanged. A missing directory imports
// nothing.
//
// The rewrite matters: without it, a re-synced spec's `tools` would be
// a single string everywhere, not just qoder, silently breaking every
// other target's generic tools passthrough on the very next sync.
func importQoderAgents(root, dstDir string) (int, error) {
	src := filepath.Join(root, qoderAgentsDir)
	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		srcPath := filepath.Join(src, e.Name())
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return count, fmt.Errorf("read %s: %w", srcPath, err)
		}
		out := rewriteQoderAgentTools(header.Strip(string(data)))
		dst := filepath.Join(dstDir, e.Name())
		if err := importWriteFile(dst, []byte(out), 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", dst, err)
		}
		count++
	}
	return count, nil
}

// rewriteQoderAgentTools rewrites the `tools:` line in body's leading
// YAML frontmatter, if present, from Qoder's documented comma-separated
// string into agnostic-ai's generic list form. A value that is already
// bracketed, quoted, or a block sequence (multi-line `- item` form) is
// left untouched: it is already list form. A body with no frontmatter
// or no `tools:` key returns unchanged.
func rewriteQoderAgentTools(body string) string {
	if !strings.HasPrefix(body, "---\n") {
		return body
	}
	rest := body[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return body
	}
	front := rest[:end]
	afterMarker := rest[end+len("\n---"):]
	lines := strings.Split(front, "\n")
	changed := false
	for i, line := range lines {
		value := strings.TrimPrefix(line, "tools:")
		if value == line {
			continue // line does not start with "tools:"
		}
		value = strings.TrimSpace(value)
		if value == "" || strings.HasPrefix(value, "[") ||
			strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "'") {
			continue // empty (block sequence follows), or already list/quoted form
		}
		items := splitAndTrimQoderTools(value)
		if len(items) == 0 {
			continue
		}
		lines[i] = "tools: [" + strings.Join(items, ", ") + "]"
		changed = true
	}
	if !changed {
		return body
	}
	return "---\n" + strings.Join(lines, "\n") + "\n---" + afterMarker
}

// splitAndTrimQoderTools splits Qoder's comma-separated tools value
// (`Read, Grep, Bash`) into trimmed, non-empty items.
func splitAndTrimQoderTools(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
