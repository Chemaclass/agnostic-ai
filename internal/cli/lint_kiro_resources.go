package cli

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// lintKiroAgentResources flags a Kiro agent whose `resources` omit
// AGENTS.md while the always-on rules reach Kiro only through it. Kiro
// custom agents inherit AGENTS.md by default; with the
// `chat.disableInheritingDefaultResources` setting on, which sync cannot
// see, that agent would load none of those rules.
func lintKiroAgentResources(cfg *config.Config, targets []string, b spec.Bundle) []lintFinding {
	if !slices.Contains(targets, "kiro") {
		return nil
	}
	if len(adapters.RulesInEntryPoint(cfg, b, "kiro")) == 0 {
		return nil
	}
	kiro := b.For("kiro")
	var out []lintFinding
	for _, a := range kiro.Agents {
		resources, _ := adapters.ResolveMeta(a.Meta, "kiro")["resources"].([]any)
		if len(resources) == 0 || slices.ContainsFunc(resources, namesAgentsMd) {
			continue
		}
		out = append(out, lintFinding{
			Code:     "LINT029",
			Severity: lintWarn,
			Path:     a.Path,
			Message: fmt.Sprintf("Kiro agent %q lists `resources` without `file://AGENTS.md`, where sync puts the always-on rules; with `chat.disableInheritingDefaultResources` on, it loads none of them. Add `file://AGENTS.md` to its `x-kiro.resources`",
				a.Name),
		})
	}
	return out
}

// namesAgentsMd reports whether a `file://` resource loads the root
// AGENTS.md. Kiro resources are globs, so `file://*.md` counts, and a
// leading `**/` also matches the root.
func namesAgentsMd(resource any) bool {
	s, _ := resource.(string)
	p, ok := strings.CutPrefix(s, "file://")
	if !ok {
		return false
	}
	p = path.Clean(p)
	for {
		if matched, _ := path.Match(p, "AGENTS.md"); matched {
			return true
		}
		rest, deeper := strings.CutPrefix(p, "**/")
		if !deeper {
			return false
		}
		p = rest
	}
}
