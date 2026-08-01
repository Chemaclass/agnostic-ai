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
	// SkipSkills suppresses the per-skill rule-form output. Set by
	// adapters that emit skills through a native surface (e.g.
	// antigravity's `.agents/skills/<name>/SKILL.md`) and must not also
	// leak a `skill-<name>` rule file.
	SkipSkills bool
	// SkipAgents suppresses the per-agent rule-form output. Set by
	// adapters that emit agents through a native surface (e.g. cursor's
	// `.cursor/agents/<name>.md` subagents) and must not also leak an
	// agent rule file.
	SkipAgents bool
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
//
// Entries with a non-empty Scope nest under the rules directory:
// `<Dir>/<scope>/<name><Ext>` (e.g. `.cursor/rules/backend/auth.mdc`).
// Tools that load rules recursively from `.cursor/rules/`, `.clinerules/`,
// etc. pick up the scoped output automatically, and the output stays inside
// the tool directory instead of leaking a `<scope>/` tree at the repo root.
func (s *Session) RulesDirectory(b spec.Bundle, opts RulesDirOpts, dryRun bool) error {
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
		path := filepath.Join(scopedDir(opts.Dir, r), r.Name+opts.Ext)
		if err := s.WriteFile(path, opts.FormatRule(r), dryRun); err != nil {
			return err
		}
	}
	if !opts.SkipAgents {
		for _, a := range b.Agents {
			name := opts.AgentPrefix + a.Name
			path := filepath.Join(scopedDir(opts.Dir, a), name+opts.Ext)
			if err := s.WriteFile(path, opts.FormatAgent(a), dryRun); err != nil {
				return err
			}
		}
	}
	if !opts.SkipSkills {
		for _, sk := range b.Skills {
			name := opts.SkillPrefix + sk.Name
			path := filepath.Join(scopedDir(opts.Dir, sk), name+opts.Ext)
			if err := s.WriteFile(path, opts.FormatSkill(sk), dryRun); err != nil {
				return err
			}
		}
	}
	return nil
}

// scopedDir returns `<dir>/<scope>` when the entry has a non-empty
// EffectiveScope, otherwise `dir` unchanged. The scope nests inside the
// rules directory so output stays under the tool dir (e.g.
// `.cursor/rules/backend`) rather than at the repo root.
func scopedDir(dir string, e spec.Entry) string {
	if s := e.EffectiveScope(); s != "" {
		return filepath.Join(dir, s)
	}
	return dir
}

func defaultFormatRule(e spec.Entry) string {
	var b strings.Builder
	b.WriteString(Header(FormatMarkdown) + "\n")
	b.WriteString("# " + e.Name + "\n\n")
	b.WriteString(e.Body)
	return b.String()
}

func defaultFormatAgent(e spec.Entry) string {
	var b strings.Builder
	b.WriteString(Header(FormatMarkdown) + "\n")
	b.WriteString("# Agent: " + e.Name + "\n\n")
	b.WriteString(e.Body)
	return b.String()
}

func defaultFormatSkill(e spec.Entry) string {
	var b strings.Builder
	b.WriteString(Header(FormatMarkdown) + "\n")
	b.WriteString("# Skill: " + e.Name + "\n\n")
	b.WriteString(e.Body)
	return b.String()
}
