package emit

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

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
	for p := range strings.SplitSeq(g, "/") {
		if strings.ContainsAny(p, "*?[") {
			break
		}
		prefix = append(prefix, p)
	}
	return strings.Join(prefix, "/")
}
