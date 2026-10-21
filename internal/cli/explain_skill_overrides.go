package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// explainOverride is one skillOverrides entry that names the explained
// skill. A file that could not be read has only target, path, scope,
// and problem.
type explainOverride struct {
	Target    string `json:"target"`
	Path      string `json:"path"`
	Scope     string `json:"scope"`
	Value     string `json:"value"`
	Effective bool   `json:"effective"`
	Rejected  bool   `json:"rejected,omitempty"`
	Problem   string `json:"problem,omitempty"`
	label     string
	rootLocal bool
	overlay   bool
	sameFile  bool
}

func (o explainOverride) MarshalJSON() ([]byte, error) {
	if o.Problem != "" {
		return json.Marshal(struct {
			Target  string `json:"target"`
			Path    string `json:"path"`
			Scope   string `json:"scope"`
			Problem string `json:"problem"`
		}{o.Target, o.Path, o.Scope, o.Problem})
	}
	type override explainOverride
	return json.Marshal(override(o))
}

// explainSkillOverrides lists each Claude settings file entry that names
// the skill, highest precedence first, and each file Claude Code skips.
// It is nil unless sync writes this skill for claude.
func explainSkillOverrides(entry spec.Entry, cfg *config.Config, bundle spec.Bundle) []explainOverride {
	if entry.Kind != spec.KindSkill || !slices.Contains(cfg.Targets, "claude") {
		return nil
	}
	i := slices.IndexFunc(bundle.For("claude").Skills, func(skill spec.Entry) bool {
		return skill.Path == entry.Path && skill.Name == entry.Name
	})
	if i < 0 || !claudeWritesSkill(cfg, bundle.For("claude").Skills[i]) {
		return nil
	}
	folded := claudeSkillName(claudeSeenSkillName(bundle.For("claude").Skills[i]))
	var overrides []explainOverride
	for _, report := range collectClaudeSkillOverrides(cfg, bundle, false) {
		base := explainOverride{Target: "claude", Path: report.recordPath(), Scope: string(report.scope), label: report.label(), rootLocal: report.rootLocal}
		if report.problem != "" {
			base.Problem = report.problem
			overrides = append(overrides, base)
			continue
		}
		for _, e := range report.entries {
			if !e.Synced || claudeSkillName(e.Key) != folded {
				continue
			}
			override := base
			override.Value, override.Effective, override.Rejected = e.Value, e.Effective, e.Rejected
			override.overlay, override.sameFile = e.Overlay, e.sameFile
			overrides = append(overrides, override)
		}
	}
	return overrides
}

func printExplainSkillOverrides(out io.Writer, overrides []explainOverride) {
	if len(overrides) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, "\nclaude overrides:")
	for _, o := range overrides {
		switch {
		case o.Problem != "":
			_, _ = fmt.Fprintf(out, "  %s\n", o.Problem)
		case o.Rejected:
			_, _ = fmt.Fprintf(out, "  %s: %s (rejected by Claude Code, which may skip the whole file; %s)\n", o.label, o.Value, o.where())
		case o.sameFile:
			_, _ = fmt.Fprintf(out, "  %s: %s (not applied: another key in this file names the skill)\n", o.label, o.Value)
		case !o.Effective:
			_, _ = fmt.Fprintf(out, "  %s: %s (not applied: a higher file sets it)\n", o.label, o.Value)
		default:
			_, _ = fmt.Fprintf(out, "  %s: %s (%s; %s)\n", o.label, o.Value, explainOverrideEffect(o.Value), o.where())
		}
	}
}

func explainOverrideEffect(value string) string {
	switch value {
	case "on":
		return "runs as usual"
	case "off":
		return "Claude Code refuses to run it"
	case "user-invocable-only":
		return "hidden from Claude"
	}
	return "listed without its description"
}

// where says how far the file reaches and where to change the entry.
func (o explainOverride) where() string {
	overlay := filepath.ToSlash(claudeOverlayPath(""))
	switch {
	case o.rootLocal:
		return "read for the whole repository; change it in /skills"
	case o.Scope == string(skillOverrideLocal):
		return "change it in /skills"
	case o.overlay:
		return "set by " + overlay
	case o.Scope == string(skillOverrideShared):
		return "team policy, set it in " + overlay
	}
	return "applies to every project"
}
