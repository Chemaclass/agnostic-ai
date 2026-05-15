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
//
// Returns nil without writing when the bundle has no rules, agents, or
// skills, so a fresh `init` followed by `sync` does not pollute the
// project root with empty stub files.
func MergedDocument(b spec.Bundle, opts MergedOpts, dryRun bool) error {
	if len(b.Rules) == 0 && len(b.Agents) == 0 && len(b.Skills) == 0 {
		return nil
	}
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
			WriteSection(&sb, r.Name, r)
		}
	}

	if len(b.Agents) > 0 {
		sb.WriteString("## " + opts.AgentsHeading + "\n\n")
		for _, a := range b.Agents {
			WriteSection(&sb, opts.AgentSectionPrefix+a.Name, a)
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
				sb.WriteString("Source: `" + filepath.ToSlash(s.Path) + "`\n\n")
			}
		}
	}

	return WriteFile(opts.OutFile, sb.String(), dryRun)
}

// WriteSection writes a "### <heading>" block followed by source
// provenance comment, optional italic description, and body.
//
// heading is taken as a parameter (rather than e.Name) so callers can
// prepend a prefix like "Agent: ".
func WriteSection(sb *strings.Builder, heading string, e spec.Entry) {
	sb.WriteString("### " + heading + "\n\n")
	sb.WriteString(SourceComment(e.Path))
	if d := e.Description(); d != "" {
		sb.WriteString("_" + d + "_\n\n")
	}
	sb.WriteString(e.Body + "\n\n")
}

// WriteReference writes a "### <name>" reference block: provenance
// comment, italic description, and a `Source: <path>` pointer. Unlike
// WriteSection it does NOT inline the spec body — callers use it when
// the body lives in a separate generated file (e.g. one agent per
// TOML, one skill per folder) and the merged document only indexes
// them.
func WriteReference(sb *strings.Builder, e spec.Entry, sourcePath string) {
	sb.WriteString("### " + e.Name + "\n\n")
	sb.WriteString(SourceComment(e.Path))
	if d := e.Description(); d != "" {
		sb.WriteString("_" + d + "_\n\n")
	}
	if sourcePath != "" {
		sb.WriteString("Source: `" + filepath.ToSlash(sourcePath) + "`\n\n")
	}
}
