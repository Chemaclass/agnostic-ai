package emit

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// Each target's entry point must be a file its own vendor documents as a
// read path. Getting one wrong is silent in every direction: sync writes
// the file, `sync --check` stays green, the golden fixtures match, and the
// tool simply never opens it. Two targets shipped that way and neither was
// caught by a test: opencode wrote `.opencode/AGENTS.md` (#623) and zed
// relied on `AGENTS.md` while copilot shadowed it (#624).
//
// vendor_lookup_test.go covers the ordered-lookup hazard, where the file we
// write is real but another emitted file outranks it. This table covers the
// simpler failure underneath it: writing a path the vendor never names.
func TestEntryPointPath_MatchesVendorReadPath(t *testing.T) {
	// Each entry is a path the vendor documents, with the source in the
	// comment. A target belongs here only once its read path is confirmed.
	for target, want := range map[string]string{
		// opencode.ai/docs/rules: an upward walk for files named exactly
		// AGENTS.md. No .opencode/ variant appears in the docs or in the
		// vendor's own instruction-context.ts (#623).
		"opencode": "AGENTS.md",
		// zed.dev/docs/ai/instructions: first match of an ordered list.
		// .rules ranks first, ahead of .github/copilot-instructions.md (#624).
		"zed": ".rules",
		// code.claude.com/docs/en/memory
		"claude": "CLAUDE.md",
		// geminicli.com/docs
		"gemini": "GEMINI.md",
		// aider.chat/docs/usage/conventions.html
		"aider": "CONVENTIONS.md",
		// docs.github.com/en/copilot: repository custom instructions
		"copilot": ".github/copilot-instructions.md",
		// learn.chatgpt.com/docs/agent-configuration/agents-md
		"codex": "AGENTS.md",
		// docs.devin.ai/cli/extensibility/rules: "Devin CLI reads this
		// file automatically", and its supported-file-names table rows
		// AGENTS.md as "Recommended" (#645).
		"windsurf": "AGENTS.md",
	} {
		if got := EntryPointPath(&config.Config{}, target); got != want {
			t.Errorf("%s entry point = %q, want %q (vendor-documented read path)", target, got, want)
		}
	}
}
