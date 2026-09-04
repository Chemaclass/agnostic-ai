package openhands

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// emitPathTriggeredRules writes one `<skillsDir>/<name>/SKILL.md` per
// rule that resolves a non-empty glob list (see pathTriggerGlobs), the
// vendor's own deterministic path-triggered rule form
// (docs.openhands.dev/overview/skills/path): "Path-triggered rules are
// skills that OpenHands injects deterministically whenever the agent
// reads, edits, or creates a file whose path matches a glob pattern...
// guaranteed to load for the files they scope, with no reliance on the
// model choosing them." A rule that resolves no glob list reaches
// OpenHands only through the shared AGENTS.md entry-point sync writes
// centrally, unchanged.
func emitPathTriggeredRules(sess *emit.Session, rules []spec.Entry, skillsDir string, dryRun bool) error {
	for _, r := range rules {
		globs := pathTriggerGlobs(r)
		if len(globs) == 0 {
			continue
		}
		path := filepath.Join(skillsDir, r.Name, "SKILL.md")
		if err := sess.WriteFile(path, emit.WithHeader(pathRuleMarkdown(r, globs), emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	return nil
}

// pathTriggerGlobs returns the glob patterns that make a rule a
// path-triggered rule, or nil for a rule that stays always-on. An
// explicit `alwaysApply: true` wins outright, the same override every
// other adapter that reads the cross-tool field honors. Otherwise
// OpenHands' own frontmatter key, `paths` (the same spelling Claude
// Code uses), wins when set; the cross-tool `globs` field (the Cursor
// spelling, comma-joined) falls back; and a rule with neither but a
// source-layout or frontmatter scope widens to `<scope>/**`, the same
// fallback order Copilot's applyToFor uses for its own path-scoped
// instructions.
func pathTriggerGlobs(e spec.Entry) []string {
	m := emit.ResolveMeta(e.Meta, target)
	if always, ok := m["alwaysApply"].(bool); ok && always {
		return nil
	}
	if paths := globList(m["paths"]); len(paths) > 0 {
		return paths
	}
	if g, _ := m["globs"].(string); g != "" {
		return splitGlobs(g)
	}
	if s := e.EffectiveScope(); s != "" {
		return []string{s + "/**"}
	}
	return nil
}

// globList normalizes a `paths` value (a scalar string or a list) into
// a slice of glob strings. Returns nil when the key is absent or
// carries no usable value.
func globList(v any) []string {
	switch p := v.(type) {
	case string:
		if p == "" {
			return nil
		}
		return []string{p}
	case []string:
		return p
	case []any:
		out := make([]string, 0, len(p))
		for _, item := range p {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// splitGlobs splits a comma-separated `globs` string into individual
// patterns, trimming surrounding whitespace and dropping empty
// entries.
func splitGlobs(g string) []string {
	parts := strings.Split(g, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// pathRuleMarkdown renders one path-triggered rule as a SKILL.md body:
// a `paths:` frontmatter list carrying the resolved globs (OpenHands'
// own field name), arbitrary `x-openhands` keys passed through, then
// the trimmed rule body. The vendor's own example places this content
// in a flat `.md` file with `paths:` as the frontmatter's only key; the
// folder form used here needs nothing else to qualify.
func pathRuleMarkdown(e spec.Entry, globs []string) string {
	meta := map[string]any{"paths": globs}
	keys := []string{"paths"}
	emit.MergeCustomTargetMeta(meta, &keys, e.Meta, target, "paths", "globs", "alwaysApply", "scope")
	front := emit.FrontmatterOrdered(meta, keys)
	return front + "\n" + strings.TrimSpace(e.Body) + "\n"
}
