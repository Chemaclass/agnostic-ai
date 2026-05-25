package cli

import (
	"fmt"
	"os"
	"strings"
)

// fenceDivergent merges two same-name spec bodies, wrapping the parts
// that differ in `::target` fences so both tools keep their authored
// content after a round-trip import.
//
// The diff runs at line granularity: the longest common prefix and
// suffix stay un-fenced and each tool's middle gets its own
// `::target <name>` / `::end` block. Identical bodies pass through
// unchanged; if one side is empty the other ends up wholly inside a
// fence.
func fenceDivergent(claudeBody, codexBody, claudeName, codexName string) string {
	if claudeBody == codexBody {
		return claudeBody
	}
	if strings.TrimSpace(claudeBody) == "" {
		return wrapInFence(codexBody, codexName)
	}
	if strings.TrimSpace(codexBody) == "" {
		return wrapInFence(claudeBody, claudeName)
	}

	cLines := splitLines(claudeBody)
	xLines := splitLines(codexBody)

	prefix := 0
	for prefix < len(cLines) && prefix < len(xLines) && cLines[prefix] == xLines[prefix] {
		prefix++
	}
	suffix := 0
	for prefix+suffix < len(cLines) && prefix+suffix < len(xLines) &&
		cLines[len(cLines)-1-suffix] == xLines[len(xLines)-1-suffix] {
		suffix++
	}

	commonPrefix := strings.Join(cLines[:prefix], "\n")
	commonSuffix := strings.Join(cLines[len(cLines)-suffix:], "\n")
	cMiddle := strings.Join(cLines[prefix:len(cLines)-suffix], "\n")
	xMiddle := strings.Join(xLines[prefix:len(xLines)-suffix], "\n")

	var b strings.Builder
	// Tight stitch: ::end runs into the next ::target with no blank in
	// between, and the common suffix attaches directly to the last ::end.
	// `renderBodyForTarget` keeps every blank line outside a fence, so any
	// visual padding here would survive into the active target's emit as
	// a stray blank between sections (#306). The fence markers are
	// already on dedicated lines, so the spec stays readable.
	if commonPrefix != "" {
		b.WriteString(commonPrefix)
		b.WriteString("\n")
	}
	if cMiddle != "" {
		b.WriteString("::target ")
		b.WriteString(claudeName)
		b.WriteString("\n")
		b.WriteString(cMiddle)
		b.WriteString("\n::end\n")
	}
	if xMiddle != "" {
		b.WriteString("::target ")
		b.WriteString(codexName)
		b.WriteString("\n")
		b.WriteString(xMiddle)
		b.WriteString("\n::end\n")
	}
	if commonSuffix != "" {
		b.WriteString(commonSuffix)
	}
	out := b.String()
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out
}

func wrapInFence(body, target string) string {
	trimmed := strings.TrimRight(body, "\n")
	return "::target " + target + "\n" + trimmed + "\n::end\n"
}

// splitLines splits body on '\n' without consuming the trailing newline.
// A body ending in '\n' yields an empty final entry which the join step
// converts back into a trailing newline, preserving line counts.
func splitLines(s string) []string {
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

// mergeSkillBodies rewrites claudePath's SKILL.md so divergent body
// content from codexPath's SKILL.md survives as a `::target codex`
// fence. Claude's frontmatter is the source of truth (per the
// claude-wins precedence documented in #287); only the body changes.
// A no-op when the bodies are byte-identical or either file lacks
// frontmatter.
func mergeSkillBodies(claudePath, codexPath string) error {
	claudeData, err := os.ReadFile(claudePath)
	if err != nil {
		return fmt.Errorf("read %s: %w", claudePath, err)
	}
	codexData, err := os.ReadFile(codexPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", codexPath, err)
	}

	claudeFront, claudeBody, claudeOK := splitCodexAgentFrontmatter(string(claudeData))
	_, codexBody, codexOK := splitCodexAgentFrontmatter(string(codexData))
	if !claudeOK || !codexOK {
		return nil
	}
	if strings.TrimRight(claudeBody, "\n") == strings.TrimRight(codexBody, "\n") {
		return nil
	}

	merged := fenceDivergent(
		strings.TrimRight(claudeBody, "\n"),
		strings.TrimRight(codexBody, "\n"),
		"claude", "codex",
	)
	out := "---\n" + claudeFront + "\n---\n\n" + merged
	if out == string(claudeData) {
		return nil
	}
	return importWriteFile(claudePath, []byte(out), 0o644)
}

// mergeAgentBody returns claudeBody with divergent codex content
// fenced. claudeBody is the existing post-frontmatter body; codexBody
// is the codex `developer_instructions` value. The result is intended
// to slot back below the merged frontmatter.
func mergeAgentBody(claudeBody, codexBody string) string {
	cb := strings.TrimRight(claudeBody, "\n")
	xb := strings.TrimRight(codexBody, "\n")
	if cb == xb {
		return claudeBody
	}
	merged := fenceDivergent(cb, xb, "claude", "codex")
	return merged
}
