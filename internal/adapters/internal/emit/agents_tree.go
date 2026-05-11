package emit

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// GroupRulesByScope buckets rules by the directory they route to. Use
// the keys (sorted alphabetically by the caller) to drive per-scope
// document emission for adapters with a hierarchical layout
// (Codex AGENTS.md, Gemini GEMINI.md, Amp AGENTS.md, Warp AGENTS.md).
func GroupRulesByScope(rules []spec.Entry) map[string][]spec.Entry {
	out := map[string][]spec.Entry{}
	for _, r := range rules {
		out[RouteScope(r)] = append(out[RouteScope(r)], r)
	}
	return out
}

// RouteScope returns the subdirectory a rule should route to. Source
// layout scope wins over globs because it is explicit (the rule's
// physical location declared the intent). Globs are parsed as a
// fallback for rules authored without a nested layout.
//
// Returns "" for root-scoped rules.
func RouteScope(r spec.Entry) string {
	if s := r.EffectiveScope(); s != "" {
		return s
	}
	g := strings.TrimPrefix(r.Globs(), "/")
	if g == "" || g == "**/*" || g == "*" {
		return ""
	}
	var prefix []string
	for _, p := range strings.Split(g, "/") {
		if strings.ContainsAny(p, "*?[") {
			break
		}
		prefix = append(prefix, p)
	}
	return strings.Join(prefix, "/")
}
