package antigravity

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

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
