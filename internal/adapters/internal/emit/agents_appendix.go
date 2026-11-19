package emit

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

// StripAgentsAppendix removes the sentinel-marked agents block (markers
// included) from body. Returns body unchanged when no block is present.
func StripAgentsAppendix(body string) string {
	return stripMarkedBlock(body, AgentsStartMarker, AgentsEndMarker)
}
