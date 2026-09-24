package antigravity

import (
	"strings"
	"unicode/utf8"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// ruleCharLimit is the cap Antigravity states for one rules file:
// "Rules files are limited to 12,000 characters each"
// (antigravity.google/docs/rules).
const ruleCharLimit = 12000

// ruleTooLongReason is the user-facing half of the over-cap note. The
// vendor states the limit and not what happens past it, so the note
// does not claim truncation or rejection, only that the file is over.
const ruleTooLongReason = "Antigravity limits a rules file to 12,000 characters and does not say whether it truncates or rejects a longer one; split the rule"

// ruleTooLongSurface names what the over-cap rule misses.
const ruleTooLongSurface = "its rules loader in full"

// noteOversizedRules buffers one note per rule whose emitted file runs
// past the vendor's stated character cap. Measured on the text rule
// actually writes, frontmatter, provenance header, and heading
// included, not on the spec body, so a rule that only clears the cap
// once agnostic-ai's own preamble is added still reports
// (target-audit 2026-09-19, #896).
func noteOversizedRules(rules []spec.Entry) {
	over := 0
	for _, r := range rules {
		if utf8.RuneCountInString(rule(r)) > ruleCharLimit {
			over++
		}
	}
	emit.NoteSurfaceGap(target, spec.KindRule, over, ruleTooLongSurface, ruleTooLongReason)
}

// Antigravity's three rule triggers this adapter can reach without
// guessing an unwritten frontmatter shape. `manual` is documented too,
// but nothing in the portable rule spec asks for "load only on an
// @-mention", so this adapter never writes it (see rule() and
// ruleTrigger()).
const (
	triggerAlwaysOn      = "always_on"
	triggerGlob          = "glob"
	triggerModelDecision = "model_decision"
)

// modelDecisionNeedsDescriptionReason is the user-facing half of the
// one coverage note this adapter still buffers for rule activation.
// antigravity.google/docs/rules marks `description` "Required for
// model_decision" but its own caution text only confirms that a
// missing frontmatter block, or an unrecognized `trigger` value, gets
// the rule silently discarded; it says nothing about a valid trigger
// missing its required companion field. Guessing either "still
// discarded" or "loads anyway, minus the description" would be
// unverified, so this adapter picks the one outcome that needs no
// description and is itself vendor-documented: `always_on`, which puts
// the full rule body in the prompt regardless of a description.
const modelDecisionNeedsDescriptionReason = "an alwaysApply: false rule with no globs would need trigger: model_decision, but antigravity.google/docs/rules requires a description for that trigger and does not say what happens on an otherwise-valid trigger missing one, so the rule stays trigger: always_on instead"

// noteRuleActivation buffers one coverage note per rule whose
// `alwaysApply: false` could not select `model_decision` for lack of a
// description, so it fell back to `always_on` (see ruleTrigger). Every
// other activation field this adapter used to drop -- `globs` and
// `alwaysApply` on their own -- now has a frontmatter key to land in;
// see rule.go's package-level rule() for the mapping (#1113).
func noteRuleActivation(rules []spec.Entry) {
	fellBack := 0
	for _, r := range rules {
		m := emit.ResolveMeta(r.Meta, target)
		if _, _, _, ok := ruleTrigger(m); ok {
			fellBack++
		}
	}
	emit.NoteFieldNoOp(target, spec.KindRule, "alwaysApply", fellBack, modelDecisionNeedsDescriptionReason)
}

// rule renders one rule file for RulesDirectory: mandatory YAML
// frontmatter as the very first bytes, then the provenance header (via
// emit.WithHeader, which inserts it right after the closing `---` so
// the frontmatter parser stays valid), then the `# <name>` heading and
// body. antigravity.google/docs/rules: "Every .md file inside rules/
// must start with YAML frontmatter declaring a valid trigger"; a file
// that omits it, or names an unrecognized trigger, is silently
// discarded.
func rule(e spec.Entry) string {
	fm := ruleFrontmatter(emit.ResolveMeta(e.Meta, target))
	return emit.WithHeader(fm+"# "+e.Name+"\n\n"+e.Body, emit.FormatMarkdown)
}

// ruleFrontmatter renders the frontmatter block in the vendor's own key
// order (trigger, description, globs).
func ruleFrontmatter(m map[string]any) string {
	trigger, desc, globs, _ := ruleTrigger(m)
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("trigger: " + trigger + "\n")
	if desc != "" {
		b.WriteString("description: " + emit.YAMLScalar(desc) + "\n")
	}
	if trigger == triggerGlob {
		b.WriteString("globs: " + emit.YAMLScalar(globs) + "\n")
	}
	b.WriteString("---\n\n")
	return b.String()
}

// ruleTrigger resolves one rule's frontmatter trigger from its generic
// `globs` / `alwaysApply` / `description` fields:
//
//	globs set                          -> glob
//	alwaysApply: false, no globs,
//	  description set                  -> model_decision
//	alwaysApply: false, no globs,
//	  no description                   -> always_on (fellBack: true)
//	anything else (including unset)    -> always_on
//
// globs wins over alwaysApply even when alwaysApply is explicitly true,
// matching the order the fix in #1113 was specified in: a spec that
// declares both is asking to scope the rule, and `glob` is the only
// trigger with a home for that. Returns the description and the
// (possibly comma-joined) globs string alongside the trigger so both
// rule() and noteRuleActivation derive the same decision from one
// place.
func ruleTrigger(m map[string]any) (trigger, desc, globs string, fellBack bool) {
	desc, _ = m["description"].(string)
	globs = ruleGlobs(m)
	always, hasAlways := m["alwaysApply"].(bool)

	trigger = triggerAlwaysOn
	switch {
	case globs != "":
		trigger = triggerGlob
	case hasAlways && !always:
		trigger = triggerModelDecision
	}
	if trigger == triggerModelDecision && desc == "" {
		trigger = triggerAlwaysOn
		fellBack = true
	}
	return trigger, desc, globs, fellBack
}

// ruleGlobs reads the generic `globs` field as a comma-joined string.
// The portable spec format allows a plain string (passed through
// as-is) or a YAML list (joined with ", ", the vendor's own example
// spacing: `globs: "*.ts, *.tsx"`).
func ruleGlobs(m map[string]any) string {
	switch v := m["globs"].(type) {
	case string:
		return v
	case []string:
		return strings.Join(v, ", ")
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	default:
		return ""
	}
}
