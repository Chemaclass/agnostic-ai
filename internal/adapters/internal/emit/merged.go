package emit

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// SourceComment renders an HTML comment that names the source spec for a
// merged-document section. Returns empty when path is empty.
//
// Example:
//
//	<!-- source: rules/conventional-commits.md -->
//
// The comment is harmless in Markdown and lets readers (and adapter
// authors) find the originating spec when staring at AGENTS.md or
// CONVENTIONS.md.
func SourceComment(path string) string {
	if path == "" {
		return ""
	}
	return "<!-- source: " + filepath.ToSlash(path) + " -->\n"
}

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
	// SkillsHeading defaults to "Skills".
	SkillsHeading string
	// AgentSectionPrefix is prepended to each agent's H3 heading.
	// Default: empty.
	AgentSectionPrefix string
	// SkillsIntro is rendered under the Skills heading. The target tool
	// has no native skill execution; the intro should explain how to
	// invoke them (typically: read the source file).
	SkillsIntro string
}

// MergedDocument writes a single markdown file with rules, agents, and
// skills merged into sections. Used by adapters whose target CLI consumes
// a single file (Codex's flat mode, Gemini, Copilot, Aider).
func MergedDocument(b spec.Bundle, opts MergedOpts, dryRun bool) error {
	if opts.RulesHeading == "" {
		opts.RulesHeading = "Rules"
	}
	if opts.AgentsHeading == "" {
		opts.AgentsHeading = "Agents"
	}
	if opts.SkillsHeading == "" {
		opts.SkillsHeading = "Skills"
	}
	if opts.SkillsIntro == "" {
		opts.SkillsIntro = "No native skill execution. Reference only; invoke by reading the source file."
	}

	var sb strings.Builder
	sb.WriteString("# " + opts.Title + "\n\n")
	if opts.Intro != "" {
		sb.WriteString(opts.Intro + "\n\n")
	}

	if len(b.Rules) > 0 {
		sb.WriteString("## " + opts.RulesHeading + "\n\n")
		for _, r := range b.Rules {
			writeSection(&sb, r.Name, r.Description(), r.Path, r.Body)
		}
	}

	if len(b.Agents) > 0 {
		sb.WriteString("## " + opts.AgentsHeading + "\n\n")
		for _, a := range b.Agents {
			writeSection(&sb, opts.AgentSectionPrefix+a.Name, a.Description(), a.Path, a.Body)
		}
	}

	if len(b.Skills) > 0 {
		sb.WriteString("## " + opts.SkillsHeading + "\n\n")
		sb.WriteString(opts.SkillsIntro + "\n\n")
		for _, s := range b.Skills {
			sb.WriteString("### " + s.Name + "\n\n")
			sb.WriteString(SourceComment(s.Path))
			if d := s.Description(); d != "" {
				sb.WriteString("_" + d + "_\n\n")
			}
			if s.Path != "" {
				sb.WriteString("Source: `" + s.Path + "`\n\n")
			}
		}
	}

	return WriteFile(opts.OutFile, sb.String(), dryRun)
}

func writeSection(sb *strings.Builder, name, description, path, body string) {
	sb.WriteString("### " + name + "\n\n")
	sb.WriteString(SourceComment(path))
	if description != "" {
		sb.WriteString("_" + description + "_\n\n")
	}
	sb.WriteString(body + "\n\n")
}
