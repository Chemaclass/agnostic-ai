package cli

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
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
// fence. Top-level frontmatter keys whose values diverge between the
// two tools (description, etc.) get routed through `x-codex` so the
// codex emit reproduces its source-of-truth (#304). A no-op when both
// frontmatter and body are byte-identical, or when either file lacks
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
	codexFront, codexBody, codexOK := splitCodexAgentFrontmatter(string(codexData))
	if !claudeOK || !codexOK {
		return nil
	}

	mergedFront, frontChanged, err := mergeSkillFrontmatter(claudeFront, codexFront)
	if err != nil {
		return err
	}
	frontOut := claudeFront
	if frontChanged {
		frontOut = strings.TrimRight(mergedFront, "\n")
	}

	bodiesDiffer := strings.TrimRight(claudeBody, "\n") != strings.TrimRight(codexBody, "\n")
	if !frontChanged && !bodiesDiffer {
		return nil
	}

	body := strings.TrimRight(claudeBody, "\n")
	if bodiesDiffer {
		body = fenceDivergent(
			strings.TrimRight(claudeBody, "\n"),
			strings.TrimRight(codexBody, "\n"),
			"claude", "codex",
		)
	}
	out := "---\n" + frontOut + "\n---\n\n" + body
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	if out == string(claudeData) {
		return nil
	}
	return importWriteFile(claudePath, []byte(out), 0o644)
}

// mergeSkillFrontmatter parses the claude + codex frontmatters and, for
// each divergent top-level key in `divergentSkillTopLevelKeys`, records
// the codex value under `x-codex.<key>` (or `x-codex.<key>: null` when
// claude has the key and codex doesn't). Returns the rewritten claude
// frontmatter (or the original on no-op) and a flag indicating whether
// any change happened.
func mergeSkillFrontmatter(claudeFront, codexFront string) (string, bool, error) {
	claudeFM := map[string]any{}
	codexFM := map[string]any{}
	if err := yaml.Unmarshal([]byte(claudeFront), &claudeFM); err != nil {
		return "", false, fmt.Errorf("parse claude frontmatter: %w", err)
	}
	if err := yaml.Unmarshal([]byte(codexFront), &codexFM); err != nil {
		return "", false, fmt.Errorf("parse codex frontmatter: %w", err)
	}
	xcodex, _ := claudeFM["x-codex"].(map[string]any)
	if xcodex == nil {
		xcodex = map[string]any{}
	}
	changed := false
	for _, key := range divergentSkillTopLevelKeys {
		before := len(xcodex)
		mergeDivergentMetaKey(claudeFM, xcodex, codexFM, key)
		if len(xcodex) != before {
			changed = true
		}
	}
	if !changed {
		return claudeFront, false, nil
	}
	claudeFM["x-codex"] = xcodex
	raw, err := marshalAgentFrontmatter(claudeFM)
	if err != nil {
		return "", false, fmt.Errorf("re-marshal frontmatter: %w", err)
	}
	return string(raw), true, nil
}

// divergentSkillTopLevelKeys lists the skill-frontmatter keys whose
// values often differ between claude and codex (description, model).
// Each gets compared during codex import: divergent codex values land
// under `x-codex.<key>` so the codex emit reproduces them faithfully.
var divergentSkillTopLevelKeys = []string{"description", "model"}

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
