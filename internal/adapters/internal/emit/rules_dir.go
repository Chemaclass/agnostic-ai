package emit

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// RulesDirOpts configures RulesDirectory output.
type RulesDirOpts struct {
	// Dir is the output directory.
	Dir string
	// Ext is the file extension (e.g. ".md", ".mdc"). Default ".md".
	Ext string
	// AgentPrefix is prepended to agent file names (e.g. "agent-").
	// Default: empty.
	AgentPrefix string
	// SkillPrefix is prepended to skill file names (e.g. "skill-").
	// Default: "skill-".
	SkillPrefix string
	// FormatRule renders one rule into file content. Defaults to
	// `# <name>\n\n<body>\n` when nil.
	FormatRule func(spec.Entry) string
	// FormatAgent renders one agent into file content. Defaults to
	// `# Agent: <name>\n\n<body>\n` when nil.
	FormatAgent func(spec.Entry) string
	// FormatSkill renders one skill into file content. Defaults to
	// `# Skill: <name>\n\n<body>\n` when nil.
	FormatSkill func(spec.Entry) string
}

// RulesDirectory writes one file per rule, agent, and skill into a directory.
// Used by Cursor (with custom formatters), Cline, Windsurf, and Continue.
func RulesDirectory(b spec.Bundle, opts RulesDirOpts, dryRun bool) error {
	if opts.Ext == "" {
		opts.Ext = ".md"
	}
	if opts.SkillPrefix == "" {
		opts.SkillPrefix = "skill-"
	}
	if opts.FormatRule == nil {
		opts.FormatRule = defaultFormatRule
	}
	if opts.FormatAgent == nil {
		opts.FormatAgent = defaultFormatAgent
	}
	if opts.FormatSkill == nil {
		opts.FormatSkill = defaultFormatSkill
	}

	for _, r := range b.Rules {
		path := filepath.Join(opts.Dir, r.Name+opts.Ext)
		if err := WriteFile(path, opts.FormatRule(r), dryRun); err != nil {
			return err
		}
	}
	for _, a := range b.Agents {
		name := opts.AgentPrefix + a.Name
		path := filepath.Join(opts.Dir, name+opts.Ext)
		if err := WriteFile(path, opts.FormatAgent(a), dryRun); err != nil {
			return err
		}
	}
	for _, s := range b.Skills {
		name := opts.SkillPrefix + s.Name
		path := filepath.Join(opts.Dir, name+opts.Ext)
		if err := WriteFile(path, opts.FormatSkill(s), dryRun); err != nil {
			return err
		}
	}
	return nil
}

func defaultFormatRule(e spec.Entry) string {
	var b strings.Builder
	b.WriteString("# " + e.Name + "\n\n")
	b.WriteString(e.Body)
	return b.String()
}

func defaultFormatAgent(e spec.Entry) string {
	var b strings.Builder
	b.WriteString("# Agent: " + e.Name + "\n\n")
	b.WriteString(e.Body)
	return b.String()
}

func defaultFormatSkill(e spec.Entry) string {
	var b strings.Builder
	b.WriteString("# Skill: " + e.Name + "\n\n")
	b.WriteString(e.Body)
	return b.String()
}
