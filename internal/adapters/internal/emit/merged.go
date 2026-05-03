package emit

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// MergedOpts configures MergedDocument output.
type MergedOpts struct {
	// OutFile is the target path.
	OutFile string
	// Title is the H1 heading (e.g. "AGENTS.md").
	Title string
	// Intro is an optional paragraph rendered after the title.
	Intro string
	// RulesHeading defaults to "Rules".
	RulesHeading string
	// AgentsHeading defaults to "Agents".
	AgentsHeading string
	// AgentSectionPrefix is prepended to each agent's H3 heading.
	// Default: empty.
	AgentSectionPrefix string
}

// MergedDocument writes a single markdown file with rules and agents merged
// into sections. Used by adapters whose target CLI consumes a single file
// (Codex's flat mode, Gemini, Copilot, Aider).
func MergedDocument(b spec.Bundle, opts MergedOpts, dryRun bool) error {
	if opts.RulesHeading == "" {
		opts.RulesHeading = "Rules"
	}
	if opts.AgentsHeading == "" {
		opts.AgentsHeading = "Agents"
	}

	var sb strings.Builder
	sb.WriteString("# " + opts.Title + "\n\n")
	if opts.Intro != "" {
		sb.WriteString(opts.Intro + "\n\n")
	}

	if len(b.Rules) > 0 {
		sb.WriteString("## " + opts.RulesHeading + "\n\n")
		for _, r := range b.Rules {
			writeSection(&sb, r.Name, r.Description(), r.Body)
		}
	}

	if len(b.Agents) > 0 {
		sb.WriteString("## " + opts.AgentsHeading + "\n\n")
		for _, a := range b.Agents {
			writeSection(&sb, opts.AgentSectionPrefix+a.Name, a.Description(), a.Body)
		}
	}

	return WriteFile(opts.OutFile, sb.String(), dryRun)
}

func writeSection(sb *strings.Builder, name, description, body string) {
	sb.WriteString("### " + name + "\n\n")
	if description != "" {
		sb.WriteString("_" + description + "_\n\n")
	}
	sb.WriteString(body + "\n\n")
}
