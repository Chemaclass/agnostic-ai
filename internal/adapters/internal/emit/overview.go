package emit

import "strings"

// NativeArtifact describes one generated artifact a target tool reads
// natively: a label ("Rules", "MCP servers"), the resolved
// project-relative location, and an optional note ("one file per rule").
// Adapters that have an entry-point file implement NativeArtifacts to
// describe their layout; the sync layer renders the result into the
// target-overview appendix of that entry-point.
type NativeArtifact struct {
	Label    string
	Location string
	Note     string
}

// TargetArtifacts pairs a target name with its native artifacts. The
// overview for a shared entry-point path (codex + amp + warp all write
// AGENTS.md) carries one entry per contributing target.
type TargetArtifacts struct {
	Target    string
	Artifacts []NativeArtifact
}

// Sentinel markers delimiting the generated target-overview appendix
// inside an entry-point file. Import strips everything between (and
// including) the markers before mirroring the body to AGNOSTIC_AI.md,
// so the canonical body round-trips losslessly.
const (
	OverviewStartMarker = "<!-- agnostic-ai:target-overview:start -->"
	OverviewEndMarker   = "<!-- agnostic-ai:target-overview:end -->"
)

// RenderTargetOverview renders the sentinel-marked appendix for one
// entry-point file. Returns "" when no target contributes artifacts,
// so callers can append the result unconditionally.
//
// With a single contributing target the artifact bullets render flat;
// with several (a shared AGENTS.md) each target gets its own
// sub-heading so the reader can tell the layouts apart.
func RenderTargetOverview(sections []TargetArtifacts) string {
	var present []TargetArtifacts
	for _, s := range sections {
		if len(s.Artifacts) > 0 {
			present = append(present, s)
		}
	}
	if len(present) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(OverviewStartMarker)
	b.WriteString("\n\n## Native artifact locations\n\n")
	b.WriteString("Generated copies read natively by the tool(s) consuming this file. ")
	b.WriteString("Edit the source specs above, not these paths; `agnostic-ai sync` overwrites them.\n")
	for _, s := range present {
		if len(present) > 1 {
			b.WriteString("\n### ")
			b.WriteString(s.Target)
			b.WriteString("\n")
		}
		b.WriteString("\n")
		for _, a := range s.Artifacts {
			b.WriteString("- **")
			b.WriteString(a.Label)
			b.WriteString("**: `")
			b.WriteString(a.Location)
			b.WriteString("`")
			if a.Note != "" {
				b.WriteString(" (")
				b.WriteString(a.Note)
				b.WriteString(")")
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(OverviewEndMarker)
	b.WriteString("\n")
	return b.String()
}

// AppendTargetOverview returns body with the rendered overview appended
// after one blank line, stripping any pre-existing overview block first
// so repeated syncs never stack appendixes. Returns body unchanged when
// overview is empty.
func AppendTargetOverview(body, overview string) string {
	if overview == "" {
		return body
	}
	body = StripTargetOverview(body)
	return strings.TrimRight(body, "\n") + "\n\n" + overview
}

// StripTargetOverview removes the sentinel-marked overview block (markers
// included) from body. Returns body unchanged when no block is present.
// A start marker without an end marker drops everything from the start
// marker on, which matches how a truncated generated block should heal
// on the next sync.
func StripTargetOverview(body string) string {
	return stripMarkedBlock(body, OverviewStartMarker, OverviewEndMarker)
}

// stripMarkedBlock removes the block delimited by startMarker/endMarker
// (markers included) from body. Returns body unchanged when startMarker
// is absent. A start marker without an end marker drops everything from
// the start marker on, which matches how a truncated generated block
// should heal on the next sync.
func stripMarkedBlock(body, startMarker, endMarker string) string {
	start := strings.Index(body, startMarker)
	if start < 0 {
		return body
	}
	head := strings.TrimRight(body[:start], "\n")
	rest := body[start+len(startMarker):]
	end := strings.Index(rest, endMarker)
	if end < 0 {
		if head == "" {
			return ""
		}
		return head + "\n"
	}
	tail := strings.TrimLeft(rest[end+len(endMarker):], "\n")
	if head == "" {
		return tail
	}
	if tail == "" {
		return head + "\n"
	}
	return head + "\n\n" + tail
}
