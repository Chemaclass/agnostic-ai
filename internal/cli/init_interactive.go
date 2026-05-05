package cli

import (
	"errors"
	"fmt"
	"strings"
)

// This file holds the canonical target list shared by the default and
// interactive init flows. It grows in later commits to host the
// interactive selection logic (selectTargets, parsePipedSelection)
// behind agnostic-ai init -i.

// targetChoice is one selectable AI CLI target shown to the user and
// written to agnostic.config.yaml.
type targetChoice struct {
	Name string // canonical name written to the config
	Desc string // short human description shown in prompts
}

// allTargets is the canonical target list, in display + emit order.
// Single source of truth for renderConfig and the interactive prompt.
var allTargets = []targetChoice{
	{Name: "claude", Desc: "Claude Code (Anthropic)"},
	{Name: "codex", Desc: "Codex CLI (OpenAI)"},
	{Name: "gemini", Desc: "Gemini CLI (Google)"},
	{Name: "cursor", Desc: "Cursor (cursor.com)"},
	{Name: "copilot", Desc: "GitHub Copilot"},
	{Name: "aider", Desc: "Aider (aider.chat)"},
	{Name: "cline", Desc: "Cline (VSCode extension)"},
	{Name: "windsurf", Desc: "Windsurf (Codeium)"},
	{Name: "continue", Desc: "Continue (continue.dev)"},
	{Name: "amp", Desc: "Amp (Sourcegraph)"},
	{Name: "zed", Desc: "Zed editor"},
	{Name: "warp", Desc: "Warp terminal"},
	{Name: "opencode", Desc: "OpenCode"},
}

// allTargetNames returns just the canonical names of allTargets.
func allTargetNames() []string {
	out := make([]string, len(allTargets))
	for i, t := range allTargets {
		out[i] = t.Name
	}
	return out
}

// errNoTargets is returned when a selection (interactive or piped)
// resolves to zero targets. Surfaced to the user with a clear message.
var errNoTargets = errors.New("no targets selected; rerun and pick at least one")

// parsePipedSelection takes a single line of comma-separated target
// names, trims whitespace around each entry, deduplicates while
// preserving canonical (allTargets) order, and validates every name
// against allTargets. An empty or whitespace-only line returns
// errNoTargets. An unknown name returns a descriptive error listing
// the valid names.
func parsePipedSelection(line string) ([]string, error) {
	raw := strings.Split(strings.TrimSpace(line), ",")
	picked := make(map[string]bool, len(raw))
	for _, r := range raw {
		name := strings.TrimSpace(r)
		if name == "" {
			continue
		}
		if !isKnownTarget(name) {
			return nil, fmt.Errorf("unknown target %q (valid: %s)",
				name, strings.Join(allTargetNames(), ", "))
		}
		picked[name] = true
	}
	if len(picked) == 0 {
		return nil, errNoTargets
	}
	out := make([]string, 0, len(picked))
	for _, t := range allTargets {
		if picked[t.Name] {
			out = append(out, t.Name)
		}
	}
	return out, nil
}

func isKnownTarget(name string) bool {
	for _, t := range allTargets {
		if t.Name == name {
			return true
		}
	}
	return false
}
