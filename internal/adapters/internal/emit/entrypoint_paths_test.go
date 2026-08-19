package emit

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// Each target's entry point is the file its own vendor documents as a
// read path. Getting one wrong is silent: sync writes it, `sync --check`
// stays green, and the tool never opens it.
func TestEntryPointPath_MatchesVendorReadPath(t *testing.T) {
	for target, want := range map[string]string{
		// OpenCode walks up from the cwd for files literally named
		// AGENTS.md and never reads a .opencode/ variant.
		"opencode": "AGENTS.md",
		// Zed takes the first match of an ordered list; see
		// TestEntryPointPath_NoPointerOnlyFileOutranksZedRules.
		"zed":     ".rules",
		"claude":  "CLAUDE.md",
		"gemini":  "GEMINI.md",
		"aider":   "CONVENTIONS.md",
		"copilot": ".github/copilot-instructions.md",
		"codex":   "AGENTS.md",
	} {
		if got := EntryPointPath(&config.Config{}, target); got != want {
			t.Errorf("%s entry point = %q, want %q", target, got, want)
		}
	}
}

// Zed reads the FIRST match of an ordered list, not a merge, so a file
// another target owns can shadow Zed's own. Rule bodies only reach Zed
// through a file that both outranks every other emitted candidate and
// carries the rules appendix. A pointer-only file ranking higher makes
// every rule silently vanish in Zed.
//
// Order per zed.dev/docs/ai/instructions, first match wins.
func TestEntryPointPath_NoPointerOnlyFileOutranksZedRules(t *testing.T) {
	zedLookupOrder := []string{
		".rules",
		".cursorrules",
		".windsurfrules",
		".clinerules",
		".github/copilot-instructions.md",
		"AGENT.md",
		"AGENTS.md",
		"CLAUDE.md",
		"GEMINI.md",
	}
	rank := make(map[string]int, len(zedLookupOrder))
	for i, name := range zedLookupOrder {
		rank[name] = i
	}

	zedPath := EntryPointPath(&config.Config{}, "zed")
	zedRank, ok := rank[zedPath]
	if !ok {
		t.Fatalf("zed entry point %q is absent from Zed's documented lookup order, so Zed reads nothing we write", zedPath)
	}
	if !InlinesRulesIntoEntryPoint("zed") {
		t.Fatal("zed must inline rules into its entry point; otherwise rule bodies reach Zed through no path at all")
	}

	for target := range entryPointPaths {
		if target == "zed" {
			continue
		}
		path := EntryPointPath(&config.Config{}, target)
		otherRank, ok := rank[path]
		if !ok || otherRank >= zedRank {
			continue
		}
		// This file wins Zed's lookup. That is only safe if it carries
		// the rule bodies too.
		if !InlinesRulesIntoEntryPoint(target) {
			t.Errorf("%s writes %q, which outranks zed's %q in Zed's lookup but carries no rules appendix: "+
				"syncing %s alongside zed would silently disable every rule in Zed",
				target, path, zedPath, target)
		}
	}
}
