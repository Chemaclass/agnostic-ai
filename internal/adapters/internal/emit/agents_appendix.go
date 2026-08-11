package emit

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Sentinel markers delimiting the generated agents appendix inside an
// entry-point file. No adapter emits this block any more: junie was the
// last target inlining agent bodies (its `.junie/AGENTS.md` was the
// only place an Agent body could land, since it had no native per-agent
// surface), and #604 gave it one (`.junie/agents/<name>.md`), matching
// every other target that declares spec.KindAgent
// (`.codex/agents/`, `.cline/agents/`, `.gemini/commands/`, ...).
// These stay as the read-side counterpart instead: `import junie` still
// recognizes the block so a project synced by an adapter version
// between #552 and #604 (agent bodies inlined, no native file yet)
// still imports its agents correctly. See junie.go and import_junie.go.
const (
	AgentsStartMarker = "<!-- agnostic-ai:agents:start -->"
	AgentsEndMarker   = "<!-- agnostic-ai:agents:end -->"
)

// RenderAgentsAppendix renders the sentinel-marked agents block for an
// entry-point file. Each agent contributes a "### <name>" section with
// its source provenance comment, optional description, and full body,
// the same shape RenderRulesAppendix uses for rules. Returns "" when the
// bundle has no agents, so callers can append the result unconditionally.
func RenderAgentsAppendix(b spec.Bundle) string {
	if len(b.Agents) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, a := range b.Agents {
		WriteSection(&sb, a.Name, a)
	}
	return wrapAgentsBlock(sb.String())
}

// wrapAgentsBlock frames inner with the agents sentinel markers and the
// "## Agents" heading.
func wrapAgentsBlock(inner string) string {
	return AgentsStartMarker + "\n\n## Agents\n\n" + inner + AgentsEndMarker + "\n"
}

// AppendAgentsAppendix returns body with the rendered agents block
// appended after one blank line, stripping any pre-existing agents block
// first so repeated syncs never stack appendixes. Returns body unchanged
// when appendix is empty.
func AppendAgentsAppendix(body, appendix string) string {
	if appendix == "" {
		return body
	}
	body = StripAgentsAppendix(body)
	return strings.TrimRight(body, "\n") + "\n\n" + appendix
}

// StripAgentsAppendix removes the sentinel-marked agents block (markers
// included) from body. Returns body unchanged when no block is present.
func StripAgentsAppendix(body string) string {
	return stripMarkedBlock(body, AgentsStartMarker, AgentsEndMarker)
}
