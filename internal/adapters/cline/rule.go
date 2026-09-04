package cline

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// rule renders one rule file: optional `paths` frontmatter, then the
// `# <name>` heading and body the shared rules-directory renderer
// writes.
//
// An always-on rule stays bare. "Rules without frontmatter are always
// active" (docs.cline.bot/customization/cline-rules), so writing an
// empty block would churn every existing file for no behavior change.
func rule(e spec.Entry) string {
	fm := pathsFrontmatter(e)
	if fm == "" {
		// Byte-identical to emit's default rule renderer, which this
		// replaces only to add the frontmatter above.
		return emit.Header(emit.FormatMarkdown) + "\n# " + e.Name + "\n\n" + e.Body
	}
	return emit.WithHeader(fm+"# "+e.Name+"\n\n"+e.Body, emit.FormatMarkdown)
}

// pathsFrontmatter renders Cline's one conditional, `paths`, and
// returns "" for a rule that has nothing to scope to.
//
//	alwaysApply true or unset       -> always active (no frontmatter)
//	alwaysApply false + globs       -> paths, with the globs verbatim
//	alwaysApply false + scope       -> paths: ["<scope>/**"]
//	alwaysApply false, neither      -> always active (see below)
//
// "Currently, `paths` is the supported conditional. It takes an array
// of glob patterns" (docs.cline.bot/customization/cline-rules). Cline
// has no description-driven or manual mode, so the last row has no
// representation: `paths: []` exists but "means the rule never
// activates", which disables the rule rather than narrowing it. The
// rule stays always-active and the gap surfaces as one coverage note
// per sync instead.
//
// Before this existed the file carried no frontmatter at all, so a rule
// scoped to `src/components/**` loaded on every request (#639).
func pathsFrontmatter(e spec.Entry) string {
	patterns := rulePaths(e)
	if len(patterns) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("---\npaths:\n")
	for _, p := range patterns {
		b.WriteString("  - " + emit.YAMLScalar(p) + "\n")
	}
	b.WriteString("---\n\n")
	return b.String()
}

// rulePaths returns the glob patterns a rule narrows to, or nil when it
// is always-on or has nothing to narrow with. Explicit globs win over
// the source-layout scope, which is the same precedence emit.RouteScope
// applies.
func rulePaths(e spec.Entry) []string {
	m := emit.ResolveMeta(e.Meta, target)
	if always, ok := m["alwaysApply"].(bool); !ok || always {
		return nil
	}
	if globs, _ := m["globs"].(string); globs != "" {
		return splitGlobs(globs)
	}
	if s := e.EffectiveScope(); s != "" {
		return []string{s + "/**"}
	}
	return nil
}

// splitGlobs turns a comma-separated globs value into the array Cline's
// `paths` key expects. A single pattern stays a one-element array,
// which is the shape the vendor's own examples use.
func splitGlobs(globs string) []string {
	var out []string
	for _, g := range strings.Split(globs, ",") {
		if g = strings.TrimSpace(g); g != "" {
			out = append(out, g)
		}
	}
	return out
}

// unscopableRules counts rules that asked not to be always-on but gave
// Cline nothing to match on. See pathsFrontmatter for why they stay
// always-active rather than emit `paths: []`.
func unscopableRules(rules []spec.Entry) int {
	n := 0
	for _, r := range rules {
		m := emit.ResolveMeta(r.Meta, target)
		if always, ok := m["alwaysApply"].(bool); !ok || always {
			continue
		}
		if len(rulePaths(r)) == 0 {
			n++
		}
	}
	return n
}
