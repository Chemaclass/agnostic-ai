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
	// ScopeAtRoot routes a scoped entry to `<scope>/<Dir>/<name><Ext>`
	// instead of nesting it at `<Dir>/<scope>/<name><Ext>`. Set only by
	// targets whose vendor discovers a copy of the tool directory in
	// project sub-directories. Devin (windsurf) is the one such target
	// today: it reads "`.devin/rules` or `.windsurf/rules` in any
	// sub-directory of your workspace"
	// (docs.devin.ai/desktop/cascade/memories) and globs each of them
	// single-level as `.devin/rules/*.md`
	// (docs.devin.ai/cli/extensibility/rules), so a nested file never
	// loads (#628).
	ScopeAtRoot bool
}

// RulesDirectory writes one file per rule, agent, and skill into a directory.
// Used by Cursor (with custom formatters), Cline, Windsurf, and Continue.
//
// Entries with a non-empty Scope route by the target's own documented
// discovery, which is a per-target claim and never a shared default:
//
//   - Default (`ScopeAtRoot` false): nest under the rules directory,
//     `<Dir>/<scope>/<name><Ext>` (e.g. `.cursor/rules/backend/auth.mdc`),
//     which keeps the output inside the tool directory. This only
//     reaches the tool when that tool reads its rules directory
//     recursively, so each caller keeping the default owes its own
//     vendor evidence for that; there is no shared guarantee.
//   - `ScopeAtRoot` true: place a copy of the tool directory in the
//     scope, `<scope>/<Dir>/<name><Ext>` (e.g.
//     `backend/.devin/rules/auth.md`). See the field comment for why
//     windsurf needs it.
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
		path := filepath.Join(scopedDir(opts, r), r.Name+opts.Ext)
		if err := s.WriteFile(path, opts.FormatRule(r), dryRun); err != nil {
			return err
		}
	}
	if !opts.SkipAgents {
		for _, a := range b.Agents {
			name := opts.AgentPrefix + a.Name
			path := filepath.Join(scopedDir(opts, a), name+opts.Ext)
			if err := s.WriteFile(path, opts.FormatAgent(a), dryRun); err != nil {
				return err
			}
		}
	}
	if !opts.SkipSkills {
		for _, sk := range b.Skills {
			name := opts.SkillPrefix + sk.Name
			path := filepath.Join(scopedDir(opts, sk), name+opts.Ext)
			if err := s.WriteFile(path, opts.FormatSkill(sk), dryRun); err != nil {
				return err
			}
		}
	}
	return nil
}

// scopedDir returns the directory one entry's file belongs in:
// `<Dir>/<scope>` by default, `<scope>/<Dir>` under ScopeAtRoot, and
// `Dir` unchanged when the entry has no EffectiveScope.
func scopedDir(opts RulesDirOpts, e spec.Entry) string {
	s := e.EffectiveScope()
	if s == "" {
		return opts.Dir
	}
	if !opts.ScopeAtRoot {
		return filepath.Join(opts.Dir, s)
	}
	// A frontmatter `scope: ../x` would anchor the tool directory
	// outside the project. Fall back to the unscoped dir: the entry
	// loses its scope but still reaches the tool, which beats writing
	// it somewhere nothing reads (#628).
	if ScopeEscapesRoot(s) {
		return opts.Dir
	}
	return filepath.Join(s, opts.Dir)
}

// ScopeEscapesRoot reports whether a cleaned scope points at or above
// the project root (a leading `..`).
func ScopeEscapesRoot(scope string) bool {
	clean := filepath.ToSlash(filepath.Clean(scope))
	return clean == ".." || strings.HasPrefix(clean, "../")
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
