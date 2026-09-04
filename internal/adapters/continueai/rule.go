package continueai

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// ruleFrontmatterKeys are the keys Continue documents on a rule file,
// in the order the vendor's own worked example writes them
// (continuedev/continue, docs/customize/deep-dives/rules.mdx). `regex`
// has no generic spec field and arrives through `x-continue`.
var ruleFrontmatterKeys = []string{"name", "globs", "regex", "alwaysApply", "description"}

// rule renders one `.continue/rules/<name>.md` file: the documented
// activation frontmatter, then the `# <name>` heading and body the
// shared rules-directory renderer writes.
//
// Before this existed the file carried no frontmatter at all, so every
// scoped rule fell through to Continue's undefined-alwaysApply default
// ("Included if no globs exist OR globs exist and match") with no globs
// to match on, which is always-on (#639).
func rule(e spec.Entry) string {
	front := ruleFrontmatter(e)
	if front == "" {
		return emit.Header(emit.FormatMarkdown) + "\n# " + e.Name + "\n\n" + e.Body
	}
	return emit.WithHeader(front+"# "+e.Name+"\n\n"+e.Body, emit.FormatMarkdown)
}

// ruleFrontmatter builds the `name` / `globs` / `regex` / `alwaysApply`
// / `description` block, or "" for a rule with no activation to state.
//
// `globs` falls back to the rule's source-layout scope (`<scope>/**`)
// when the spec declares none, so a rule authored by directory still
// narrows. `alwaysApply` and `description` emit only when the spec sets
// them, since Continue's undefined default ("Included if no globs exist
// OR globs exist and match") is not the same as an explicit `false`.
// `name` rides along once something else is there, giving Continue's
// rules toolbar a display name; on its own it would churn every
// existing file for no activation change, so a bare rule stays bare.
func ruleFrontmatter(e spec.Entry) string {
	m := emit.ResolveMeta(e.Meta, target)
	meta := map[string]any{}
	var keys []string

	globs, _ := m["globs"].(string)
	if globs == "" {
		if s := e.EffectiveScope(); s != "" {
			globs = s + "/**"
		}
	}
	if globs != "" {
		meta["globs"] = globs
		keys = append(keys, "globs")
	}
	if always, ok := m["alwaysApply"].(bool); ok {
		meta["alwaysApply"] = always
		keys = append(keys, "alwaysApply")
	}
	if desc, _ := m["description"].(string); desc != "" {
		meta["description"] = desc
		keys = append(keys, "description")
	}
	emit.MergeCustomTargetMeta(meta, &keys, e.Meta, target, append(keys, "name")...)
	if len(keys) == 0 {
		return ""
	}
	meta["name"] = e.Name
	keys = append(keys, "name")
	return emit.FrontmatterOrdered(meta, orderedRuleKeys(keys)) + "\n"
}

// orderedRuleKeys puts the documented keys in the vendor's own order
// and appends anything an author added through `x-continue` after them.
func orderedRuleKeys(keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range ruleFrontmatterKeys {
		if contains(keys, k) {
			out = append(out, k)
		}
	}
	for _, k := range keys {
		if !contains(ruleFrontmatterKeys, k) {
			out = append(out, k)
		}
	}
	return out
}

func contains(keys []string, want string) bool {
	for _, k := range keys {
		if k == want {
			return true
		}
	}
	return false
}
