package config

import "github.com/chemaclass/agnostic-ai/internal/errs"

// Default lint budgets. Claude Code asks for a CLAUDE.md under 200
// lines, about 2000 words of prose, which also sits well under the
// Codex (32 KiB) and Antigravity (24,000 bytes) caps. 1024 characters
// is the Agent Skills limit on a skill description.
const (
	DefaultInstructionsWords = 2000
	DefaultDescriptionChars  = 1024
)

// LintConfig holds the budgets `lint` checks always-loaded text
// against. Zero keeps the default.
type LintConfig struct {
	// InstructionsWords caps the words one target loads every session:
	// its entry-point file, always-on rule files, and skill and agent
	// descriptions. Default 2000.
	InstructionsWords int `yaml:"instructions-words,omitempty" json:"instructions-words,omitempty"`
	// DescriptionChars caps the characters of one skill or agent
	// description. Default 1024.
	DescriptionChars int `yaml:"description-chars,omitempty" json:"description-chars,omitempty"`
}

// InstructionsWordBudget returns the effective word budget.
func (l LintConfig) InstructionsWordBudget() int {
	if l.InstructionsWords == 0 {
		return DefaultInstructionsWords
	}
	return l.InstructionsWords
}

// DescriptionCharBudget returns the effective description budget.
func (l LintConfig) DescriptionCharBudget() int {
	if l.DescriptionChars == 0 {
		return DefaultDescriptionChars
	}
	return l.DescriptionChars
}

// Validate rejects a negative budget, naming source.
func (l LintConfig) Validate(source string) error {
	if l.InstructionsWords < 0 {
		return errs.Coded(errs.CodeConfigDecode, "%s: lint.instructions-words must be positive, got %d", source, l.InstructionsWords)
	}
	if l.DescriptionChars < 0 {
		return errs.Coded(errs.CodeConfigDecode, "%s: lint.description-chars must be positive, got %d", source, l.DescriptionChars)
	}
	return nil
}
