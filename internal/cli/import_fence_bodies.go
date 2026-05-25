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
	// Codex CLI accepts unquoted '#' in plain scalars; strict YAML does
	// not. Quote those scalars before yaml.Unmarshal so the codex
	// description survives intact instead of being truncated at the
	// first '#' (#317).
	codexFront = quoteHashInPlainScalars(codexFront)

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
//
// Claude's existing frontmatter keys keep their original yaml.Node form
// (scalar style, comments, key order) so a hand-quoted `allowed-tools:
// "Read, Bash(*)"` survives byte-for-byte. Only the x-codex subtree is
// added or modified (#313).
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
	overrides := map[string]any{}
	for _, key := range divergentSkillTopLevelKeys {
		before := len(xcodex)
		mergeDivergentMetaKey(claudeFM, xcodex, codexFM, key)
		if len(xcodex) != before {
			overrides[key] = xcodex[key]
		}
	}
	if len(overrides) == 0 {
		return claudeFront, false, nil
	}
	merged, err := upsertXCodexInFrontmatter(claudeFront, overrides)
	if err != nil {
		return "", false, fmt.Errorf("upsert x-codex: %w", err)
	}
	return merged, true, nil
}

// upsertXCodexInFrontmatter rewrites the given YAML frontmatter so its
// `x-codex` mapping contains every key in overrides (creating the
// subtree when absent). Existing top-level entries keep their original
// yaml.Node form, preserving scalar style, key order, and comments
// (#313).
func upsertXCodexInFrontmatter(front string, overrides map[string]any) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(front), &doc); err != nil {
		return "", err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return "", fmt.Errorf("frontmatter is not a mapping")
	}
	mapping := doc.Content[0]
	xcodex := findOrAppendMapping(mapping, "x-codex")
	for k, v := range overrides {
		setMappingValue(xcodex, k, v)
	}
	raw, err := yaml.Marshal(&doc)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(raw), "\n"), nil
}

// findOrAppendMapping locates the value node for key in mapping. When
// key is absent or its value isn't a mapping, a fresh mapping node
// replaces the slot. Returns the mapping value node so callers can
// mutate it directly.
func findOrAppendMapping(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			if mapping.Content[i+1].Kind != yaml.MappingNode {
				mapping.Content[i+1] = &yaml.Node{Kind: yaml.MappingNode}
			}
			return mapping.Content[i+1]
		}
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
	valNode := &yaml.Node{Kind: yaml.MappingNode}
	mapping.Content = append(mapping.Content, keyNode, valNode)
	return valNode
}

// setMappingValue assigns or replaces the value under key in mapping.
// A nil value emits as the YAML scalar `null`, which ResolveMeta treats
// as a deletion marker for the per-target emit (#304).
func setMappingValue(mapping *yaml.Node, key string, value any) {
	valNode := &yaml.Node{}
	if value == nil {
		valNode.Kind = yaml.ScalarNode
		valNode.Tag = "!!null"
		valNode.Value = "null"
	} else {
		_ = valNode.Encode(value)
	}
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = valNode
			return
		}
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
	mapping.Content = append(mapping.Content, keyNode, valNode)
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
