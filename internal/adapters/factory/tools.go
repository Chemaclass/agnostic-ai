package factory

import "github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"

// factoryToolID maps agnostic-ai's Claude-style tool identifiers onto
// Droid CLI's own tool IDs (docs.factory.ai/harness/subagents, "Tool
// categories"). Factory's table is the complete set of valid IDs:
// `Read`, `LS`, `Grep`, `Glob`, `Create`, `Edit`, `ApplyPatch`,
// `Execute`, `WebSearch`, `FetchUrl`. Seven Claude-style names are
// already spelled that way and pass through; `Bash`, `Write`, and
// `WebFetch` are not, and Factory's own Claude Code importer renames
// them the same way this table does. IDs are case-sensitive, so the
// lookup is exact: `bash` is an unknown ID, not a spelling of `Bash`.
var factoryToolID = map[string]string{
	// Already valid Factory IDs.
	"Read":       "Read",
	"LS":         "LS",
	"Grep":       "Grep",
	"Glob":       "Glob",
	"Create":     "Create",
	"Edit":       "Edit",
	"ApplyPatch": "ApplyPatch",
	"Execute":    "Execute",
	"WebSearch":  "WebSearch",
	"FetchUrl":   "FetchUrl",
	// Claude-style names Factory spells differently.
	"Bash":     "Execute",
	"Write":    "Create",
	"WebFetch": "FetchUrl",
}

// factoryAlwaysOnTool holds the two tools Factory grants every droid
// unconditionally: "`TodoWrite` and `Skill` are always included for
// every droid so it can track tasks and load skills. You do not list
// them, and they do not appear in the tool count." Listing either would
// be an unknown ID at load time, and dropping them costs the droid
// nothing, so they leave the list without a coverage note.
var factoryAlwaysOnTool = map[string]bool{"TodoWrite": true, "Skill": true}

// translateTools maps a spec's generic Claude-style tools list onto
// Factory's tool IDs (factoryToolID), deduplicated in first-seen order
// because two Claude-style names can share one ID and Factory's
// DroidValidator warns on duplicate tools. A name with no table entry
// is never written verbatim: "Unknown IDs cause a validation error", so
// one bad name would cost the author the whole droid. It drops instead
// and sets hasDropped so the caller can surface it. That covers
// `ExitSpecMode` and `GenerateDroid` too, which have no entry on
// purpose: "cannot be enabled by a custom droid; listing either one is
// a validation error". The always-on pair is the one silent drop, since
// the droid keeps the capability either way.
func translateTools(names []string) (mapped []string, hasDropped bool) {
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		if factoryAlwaysOnTool[n] {
			continue
		}
		id, ok := factoryToolID[n]
		if !ok {
			hasDropped = true
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		mapped = append(mapped, id)
	}
	return mapped, hasDropped
}

// xFactorySetsTools reports whether the spec already carries an explicit
// x-factory.tools override: the one channel this adapter trusts to
// already be Factory's own vocabulary rather than agnostic-ai's generic
// Claude-style names. It is also the only way to reach the two shapes
// the cross-tool list cannot express, a category name (`read-only`,
// `edit`, `execute`, `web`, `mcp`) and a registered MCP tool ID.
func xFactorySetsTools(meta map[string]any) bool {
	x, _ := emit.CustomTargetMeta(meta, target)
	if x == nil {
		return false
	}
	_, tools := x["tools"]
	return tools
}
