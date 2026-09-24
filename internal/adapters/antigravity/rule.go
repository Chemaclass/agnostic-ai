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

// Antigravity's four documented rule triggers, all reachable from the
// portable spec's `globs` / `alwaysApply` / `description` fields (see
// ruleTrigger).
const (
	triggerAlwaysOn      = "always_on"
	triggerGlob          = "glob"
	triggerModelDecision = "model_decision"
	triggerManual        = "manual"
)

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
// order (trigger, description, globs). globs is only ever non-empty
// when ruleTrigger chose triggerGlob, so the `globs:` line only appears
// there.
func ruleFrontmatter(m map[string]any) string {
	trigger, desc, globs := ruleTrigger(m)
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("trigger: " + trigger + "\n")
	if desc != "" {
		b.WriteString("description: " + emit.YAMLScalar(desc) + "\n")
	}
	if globs != "" {
		b.WriteString("globs: " + emit.YAMLScalar(globs) + "\n")
	}
	b.WriteString("---\n\n")
	return b.String()
}

// ruleTrigger resolves one rule's frontmatter trigger from its generic
// `globs` / `alwaysApply` / `description` fields. The mapping mirrors
// the one windsurf's `activationFrontmatter` applies to the same three
// fields (internal/adapters/windsurf/windsurf.go, #628), itself mirroring
// cursor's `.mdc` renderer:
//
//	alwaysApply true or unset          -> always_on
//	alwaysApply false + globs          -> glob
//	alwaysApply false + description    -> model_decision
//	alwaysApply false, neither         -> manual
//
// `alwaysApply` decides first and wins outright: an `alwaysApply: true`
// rule ignores `globs` entirely, the same choice cursor's `mdc()` makes
// ("An alwaysApply:true rule ignores globs entirely ... omit it", #443),
// so a rule seeded by `new rule` (`globs: "**/*"` and `alwaysApply: true`
// together, docs/site/content/docs/spec-format.md#rules) stays
// `always_on`: turning it into `trigger: glob` would only activate the
// rule when the agent touches a matching file, not on every turn.
// `alwaysApply` unset behaves the same as `true`: windsurf's own
// `!ok || always` check and cursor's `always := true` default both
// treat a missing key as always-on, so this adapter does too, and
// `globs` alone (no `alwaysApply: false`) still resolves to
// `always_on`.
//
// Every branch reaches a trigger with no unmet vendor requirement:
// `glob` only fires with `globs` in hand, `model_decision` only fires
// with `description` in hand, and `manual` needs neither ("The agent
// injects the rule only when you explicitly @-mention it in chat",
// antigravity.google/docs/rules), so there is no coverage note to raise
// here (#1113; an earlier draft of this fix fell back to `always_on`
// with a note for `alwaysApply: false` + no globs + no description,
// which promotes a rule that opted out of always-on, the exact bug
// #628 fixed for windsurf).
//
// `description` is returned whenever the spec has one, independent of
// the chosen trigger ("recommended for all rules", same page), and
// `globs` is returned only when the trigger is `glob`, so
// ruleFrontmatter never writes a key the chosen trigger has no use for.
func ruleTrigger(m map[string]any) (trigger, desc, globs string) {
	desc, _ = m["description"].(string)
	always := true
	if v, ok := m["alwaysApply"].(bool); ok {
		always = v
	}
	if always {
		return triggerAlwaysOn, desc, ""
	}
	globs = ruleGlobs(m)
	switch {
	case globs != "":
		return triggerGlob, desc, globs
	case desc != "":
		return triggerModelDecision, desc, ""
	default:
		return triggerManual, desc, ""
	}
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
