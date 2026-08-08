package emit

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Sentinel markers delimiting the generated agents appendix inside an
// entry-point file. Mirrors the rules appendix (see rules_appendix.go)
// for the one target that needs it today: junie has no native per-agent
// surface at all (Rule and Agent specs share the same `.junie/rules/`
// flattening, and even that directory is unreachable, see junie.go's
// package doc), so its preferred entry-point file `.junie/AGENTS.md` is
// the only place an Agent body can land. Every other target that
// declares spec.KindAgent has a real native destination (`.codex/agents/`,
// `.cline/agents/`, `.gemini/commands/`, ...), so this stays
// deliberately unwired from the shared multi-consumer entry-point
// dispatch in internal/cli/entrypoint.go: inlining agent bodies into the
// root AGENTS.md that codex/amp/warp/... also consume would duplicate
// content those targets already deliver natively. junie.go calls
// RenderAgentsAppendix/AppendAgentsAppendix directly against its own
// `.junie/AGENTS.md` write instead.
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
