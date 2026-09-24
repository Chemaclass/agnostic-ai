package antigravity

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// ruleByteLimit is the per-file cap Antigravity states for one rules
// file: "Antigravity truncates any single rule file that exceeds
// 24,000 bytes (after expanding `@[label](path)` includes)"
// (antigravity.google/docs/rules).
const ruleByteLimit = 24000

// ruleTooLongReason is the user-facing half of the over-cap note,
// naming the vendor's own documented outcome: truncation, not
// rejection.
const ruleTooLongReason = "Antigravity truncates a rules file over 24,000 bytes; split the rule"

// ruleTooLongSurface names what the over-cap rule misses.
const ruleTooLongSurface = "its rules loader in full"

// noteOversizedRules buffers one note per rule whose emitted file runs
// past the vendor's stated per-file byte cap. Measured on the bytes
// rule actually writes, frontmatter, provenance header, and heading
// included, not on the spec body, so a rule that only clears the cap
// once agnostic-ai's own preamble is added still reports
// (target-audit 2026-09-19, #896; cap corrected to bytes, #1114). The
// separate 20,000-token aggregate budget across every always_on rule
// degrades gracefully to a path-plus-description pointer, so it raises
// no note here; see the package doc and the target page.
func noteOversizedRules(rules []spec.Entry) {
	over := 0
	for _, r := range rules {
		if len(rule(r)) > ruleByteLimit {
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
	trigger, desc, globs, _ := ruleTrigger(m)
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

// invalidTriggerOverrideReason is the user-facing half of the coverage
// note noteInvalidTriggerOverrides buffers.
const invalidTriggerOverrideReason = "the rule's native antigravity trigger is either not one of always_on/glob/model_decision/manual, or is missing the companion field that trigger needs (globs for glob, description for model_decision); it renders as trigger: manual instead"

// noteInvalidTriggerOverrides buffers one coverage note per rule whose
// `x-antigravity.trigger` override (see ruleTrigger) could not be
// honored, so it rendered as `manual` instead.
func noteInvalidTriggerOverrides(rules []spec.Entry) {
	invalid := 0
	for _, r := range rules {
		m := emit.ResolveMeta(r.Meta, target)
		if _, _, _, bad := ruleTrigger(m); bad {
			invalid++
		}
	}
	emit.NoteFieldNoOp(target, spec.KindRule, "trigger", invalid, invalidTriggerOverrideReason)
}

// ruleTrigger resolves one rule's frontmatter trigger.
//
// An `x-antigravity.trigger` override wins outright when present:
// `emit.ResolveMeta` already flattens it onto the top-level `trigger`
// key ahead of this call, and `import antigravity` writes it whenever
// a native `.agents/rules/*.md` file carried its own `trigger` value
// (normalizeAntigravityRuleMeta, internal/cli/import_antigravity.go).
// Without honoring it, a re-emit had to re-derive activation from the
// generic `globs` / `alwaysApply` / `description` fields alone, which
// cannot always recover the original intent: a `manual` rule that also
// carries a `description` (the vendor recommends one on every rule)
// re-derived as `model_decision`, turning an explicit @-mention-only
// rule into an automatically-loaded one, and an unrecognized trigger
// value silently re-derived as `always_on`, Antigravity's most active
// mode, on the very next sync (#1117 review). The override still
// answers to the vendor's own required companion field: `glob` without
// `globs`, or `model_decision` without `description`, has nowhere to
// land, so both fall to `manual` with a coverage note
// (noteInvalidTriggerOverrides), the same fallback an unrecognized
// value gets. `manual` is the narrowest mode Antigravity documents and
// needs no companion field, so it is always a safe landing spot.
//
// With no override, the trigger derives from the generic fields, the
// same mapping windsurf's `activationFrontmatter` applies to the same
// three fields (internal/adapters/windsurf/windsurf.go, #628), itself
// mirroring cursor's `.mdc` renderer:
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
// `always_on`. Every one of these four branches reaches a trigger with
// no unmet vendor requirement, so none of them can set the invalid
// return.
//
// `description` is returned whenever the spec has one, independent of
// the chosen trigger ("recommended for all rules", same page), and
// `globs` is returned only when the trigger is `glob`, so
// ruleFrontmatter never writes a key the chosen trigger has no use for.
func ruleTrigger(m map[string]any) (trigger, desc, globs string, invalid bool) {
	desc, _ = m["description"].(string)
	rawGlobs := ruleGlobs(m)

	if override, ok := m["trigger"].(string); ok {
		switch override {
		case triggerAlwaysOn:
			return triggerAlwaysOn, desc, "", false
		case triggerGlob:
			if rawGlobs != "" {
				return triggerGlob, desc, rawGlobs, false
			}
		case triggerModelDecision:
			if desc != "" {
				return triggerModelDecision, desc, "", false
			}
		case triggerManual:
			return triggerManual, desc, "", false
		}
		return triggerManual, desc, "", true
	}

	always := true
	if v, ok := m["alwaysApply"].(bool); ok {
		always = v
	}
	if always {
		return triggerAlwaysOn, desc, "", false
	}
	switch {
	case rawGlobs != "":
		return triggerGlob, desc, rawGlobs, false
	case desc != "":
		return triggerModelDecision, desc, "", false
	default:
		return triggerManual, desc, "", false
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
