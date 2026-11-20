package openhands

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const pathRuleNamePrefix = "path-rule/"
const pathRuleFileSuffix = "-path-rule"

func emitPathTriggeredRules(sess *emit.Session, rules []spec.Entry, skillsDir string, dryRun bool) error {
	usedStems := make(map[string]bool, len(rules))
	for _, r := range rules {
		usedStems[strings.ToLower(r.Name)] = true
	}
	for _, r := range rules {
		globs := pathTriggerGlobs(r)
		if len(globs) == 0 {
			continue
		}
		stem := r.Name
		if reservedPathRuleStem(stem) {
			base := stem + pathRuleFileSuffix
			stem = base
			for i := 2; usedStems[strings.ToLower(stem)]; i++ {
				stem = base + "-" + strconv.Itoa(i)
			}
			usedStems[strings.ToLower(stem)] = true
		}
		path := filepath.Join(skillsDir, stem+".md")
		if err := sess.WriteFile(path, emit.WithHeader(pathRuleMarkdown(r, globs), emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	return nil
}

func reservedPathRuleStem(name string) bool {
	// The SDK skips SKILL.md and README.md and reserves the other names for instruction files.
	switch strings.ToLower(name) {
	case "agent", "agents", "claude", "gemini", "readme", "skill":
		return true
	default:
		return false
	}
}

// PathRuleSourceName reverses the filename and name emitted for a scoped rule.
func PathRuleSourceName(stem, nativeName string) (string, bool) {
	name, ok := strings.CutPrefix(nativeName, pathRuleNamePrefix)
	if !ok || spec.ValidateName(spec.KindRule, name) != nil {
		return "", false
	}
	if !reservedPathRuleStem(name) {
		return name, stem == name
	}
	base := name + pathRuleFileSuffix
	if stem == base {
		return name, true
	}
	number, ok := strings.CutPrefix(stem, base+"-")
	i, err := strconv.Atoi(number)
	return name, ok && err == nil && i >= 2 && strconv.Itoa(i) == number
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
//
// An unscoped `paths`/`globs` value that resolves to nothing but catch-all
// patterns (`**`, `**/*`, `*`, or empty) also stays always-on: it
// applies to every file, so writing it as a path-triggered rule would
// only relocate an always-on rule out of AGENTS.md, not scope it.
// agents_tree.go's RouteScope treats the same three patterns as "no
// scope" for the same reason. This also absorbs a real round-trip: a
// rule authored with `globs: "**/*"` and `alwaysApply: true` loses the
// explicit `alwaysApply` key once it is re-synced through Claude Code
// (whose own rule frontmatter carries only `paths`, dropping
// `alwaysApply` and translating a catch-all `globs` into a catch-all
// `paths` on the way) and re-imported; without this check the
// re-imported rule would flip from always-on to path-triggered on the
// next sync.
func pathTriggerGlobs(e spec.Entry) []string {
	m := emit.ResolveMeta(e.Meta, target)
	if always, ok := m["alwaysApply"].(bool); ok && always {
		return nil
	}
	scoped := e.EffectiveScope() != ""
	if paths := emit.PathList(m["paths"]); len(paths) > 0 && (scoped || !hasCatchAllGlob(paths)) {
		return paths
	}
	if globs := spec.GlobList(m["globs"]); len(globs) > 0 && (scoped || !hasCatchAllGlob(globs)) {
		return globs
	}
	if s := e.EffectiveScope(); s != "" {
		return []string{s + "/**"}
	}
	return nil
}

// hasCatchAllGlob reports whether any pattern in globs matches every
// file. Per docs.openhands.dev/overview/skills/path's own glob
// semantics table, `**` matches "any number of path segments,
// including zero", and a slash-less pattern like a bare `*` matches
// "the basename at any depth", so either alone matches every path; one
// catch-all pattern makes the whole list catch-all regardless of what
// else it contains.
func hasCatchAllGlob(globs []string) bool {
	for _, g := range globs {
		switch strings.TrimPrefix(g, "/") {
		case "", "*", "**", "**/*":
			return true
		}
	}
	return false
}

func pathRuleMarkdown(e spec.Entry, globs []string) string {
	// OpenHands deduplicates rules and skills by name. Folder skills cannot contain a slash.
	meta := map[string]any{"paths": globs, "name": pathRuleNamePrefix + e.Name}
	keys := []string{"paths", "name"}
	emit.MergeCustomTargetMeta(meta, &keys, e.Meta, target, "paths", "name", "globs", "alwaysApply", "scope")
	front := emit.FrontmatterOrdered(meta, keys)
	return front + "\n" + strings.TrimSpace(e.Body) + "\n"
}
