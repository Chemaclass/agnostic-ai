package antigravity

import (
	"unicode/utf8"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// ruleCharLimit is the cap Antigravity states for one rules file:
// "Rules files are limited to 12,000 characters each"
// (antigravity.google/docs/rules-workflows?tab=ide).
const ruleCharLimit = 12000

// ruleTooLongReason is the user-facing half of the over-cap note. The
// vendor states the limit and not what happens past it, so the note
// does not claim truncation or rejection, only that the file is over.
const ruleTooLongReason = "Antigravity limits a rules file to 12,000 characters and does not say whether it truncates or rejects a longer one; split the rule"

// ruleTooLongSurface names what the over-cap rule misses.
const ruleTooLongSurface = "its rules loader in full"

// noteOversizedRules buffers one note per rule whose emitted file runs
// past the vendor's stated character cap. Measured on the text
// RulesDirectory actually writes, provenance header and heading
// included, not on the spec body, so a rule that only clears the cap
// once agnostic-ai's own preamble is added still reports
// (target-audit 2026-09-19, #896).
func noteOversizedRules(rules []spec.Entry) {
	over := 0
	for _, r := range rules {
		if utf8.RuneCountInString(emit.DefaultRuleFile(r)) > ruleCharLimit {
			over++
		}
	}
	emit.NoteSurfaceGap(target, spec.KindRule, over, ruleTooLongSurface, ruleTooLongReason)
}

// activationReason is the user-facing half of both rule activation
// notes. Antigravity lists four modes on
// antigravity.google/docs/rules-workflows?tab=ide (Manual, Always on,
// Model decision, Glob pattern) as prose about the Customizations
// panel. No page on the docs host names a frontmatter key, a file
// format, or an example for any of them, so a bare file is the only
// shape this adapter can write, and a bare file is always-on.
const activationReason = "Antigravity documents its four rule activation modes in prose and names no frontmatter key for them; a bare rule file is always-on"

// noteRuleActivation buffers one coverage note per activation field a
// rule declared and this adapter had nowhere to put. Windsurf writes
// `trigger:`/`globs:` for the same three generic fields, and the
// lineage resemblance makes copying that spelling tempting, but it
// would put a key in users' files that no Antigravity page backs
// (#865). The drop stands; the note is what stops it being silent.
func noteRuleActivation(rules []spec.Entry) {
	globs, notAlways := 0, 0
	for _, r := range rules {
		m := emit.ResolveMeta(r.Meta, target)
		if g, _ := m["globs"].(string); g != "" {
			globs++
		}
		if always, ok := m["alwaysApply"].(bool); ok && !always {
			notAlways++
		}
	}
	emit.NoteFieldNoOp(target, spec.KindRule, "globs", globs, activationReason)
	emit.NoteFieldNoOp(target, spec.KindRule, "alwaysApply", notAlways, activationReason)
}
