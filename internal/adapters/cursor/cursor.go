// Package cursor emits .cursor/rules/*.mdc files for the Cursor editor.
//
// Rules emit with alwaysApply=true; agents emit as rules with
// alwaysApply=false. Both honor a frontmatter override.
package cursor

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target             = "cursor"
	defaultDir         = ".cursor/rules"
	defaultExt         = ".mdc"
	defaultMCPFile     = ".cursor/mcp.json"
	defaultReviewFile  = "BUGBOT.md"
	defaultEnvironFile = ".cursor/environment.json"
	defaultIgnoreFile  = ".cursorignore"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindMCP, spec.KindReview, spec.KindEnvironment, spec.KindIgnore},
}

// environRoutingKeys are the agnostic-ai spec fields stripped after
// emit.ResolveMeta before the remaining keys pass through to
// `.cursor/environment.json`. ResolveMeta already removes the target
// allow/deny keys and the x-<target> namespace; these three are the spec
// identity/scoping fields it leaves in place that Cursor has no use for.
var environRoutingKeys = map[string]struct{}{
	"name": {}, "scope": {}, "description": {},
}

// Adapter emits Cursor configs.
type Adapter struct{}

// New returns a Cursor adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one .mdc per rule, agent, and skill, plus an
// `.cursor/mcp.json` when MCP entries exist. When
// `outputs.cursor.commands-dir` is set, also writes one Cursor Custom
// Command per agent at that directory; the rule-form emission still
// happens so users that depend on it keep working.
func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := emit.RulesDirectory(b, emit.RulesDirOpts{
		Dir:         emit.OutputRulesDir(cfg, target, defaultDir),
		Ext:         defaultExt,
		FormatRule:  func(e spec.Entry) string { return emit.WithHeader(mdc(e, true), emit.FormatMarkdown) },
		FormatAgent: func(e spec.Entry) string { return emit.WithHeader(mdc(e, false), emit.FormatMarkdown) },
		FormatSkill: func(e spec.Entry) string { return emit.WithHeader(mdc(e, false), emit.FormatMarkdown) },
	}, dryRun); err != nil {
		return err
	}
	noteDroppedSkillAssets(b)
	if err := emitReviews(b, cfg, dryRun); err != nil {
		return err
	}
	if err := emitEnvironment(b, cfg, dryRun); err != nil {
		return err
	}
	if err := emit.WriteIgnoreFile(b.Ignores, emit.OutputIgnoreFile(cfg, target, defaultIgnoreFile), dryRun); err != nil {
		return err
	}
	if commandsDir := emit.OutputCommandsDir(cfg, target, ""); commandsDir != "" {
		for _, a := range b.Agents {
			path := commandsDir + "/" + a.Name + ".md"
			if err := emit.WriteFile(path, emit.WithHeader(command(a), emit.FormatMarkdown), dryRun); err != nil {
				return err
			}
		}
	}
	return emit.WriteMCPFile(b.MCPs, emit.MCPSchemaServersMap, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// command renders a Cursor Custom Command file. The Cursor docs
// describe these as Markdown with optional frontmatter (`description`,
// `model`); the body is the prompt the IDE sends when the user invokes
// the command.
func command(e spec.Entry) string {
	m := emit.ResolveMeta(e.Meta, target)
	desc, _ := m["description"].(string)
	model, _ := m["model"].(string)
	var b strings.Builder
	b.WriteString("---\n")
	if desc != "" {
		b.WriteString("description: " + desc + "\n")
	}
	if model != "" {
		b.WriteString("model: " + model + "\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(e.Body)
	return b.String()
}

// emitReviews writes Cursor Bugbot review guidance as a `BUGBOT.md` per
// scope. Cursor reads a root `BUGBOT.md` plus optional per-directory files,
// so review specs honor `EffectiveScope` the same way rules do: an unscoped
// spec lands at the repo root, a spec under `reviews/backend/` lands at
// `backend/BUGBOT.md`. Specs sharing a scope concatenate into that scope's
// single file. The basename is overridable via `outputs.cursor.review-file`.
func emitReviews(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if len(b.Reviews) == 0 {
		return nil
	}
	base := emit.OutputReviewFile(cfg, target, defaultReviewFile)
	byScope := map[string][]spec.Entry{}
	var scopeOrder []string
	for _, r := range b.Reviews {
		scope := r.EffectiveScope()
		if _, ok := byScope[scope]; !ok {
			scopeOrder = append(scopeOrder, scope)
		}
		byScope[scope] = append(byScope[scope], r)
	}
	for _, scope := range scopeOrder {
		if scopeEscapesRoot(scope) {
			// A frontmatter `scope: ../x` would anchor BUGBOT.md outside the
			// repo (review files sit at the project root, not under a tool
			// dir). Skip it rather than write beyond the project.
			continue
		}
		var sb strings.Builder
		for i, r := range byScope[scope] {
			if i > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(strings.TrimRight(r.Body, "\n"))
		}
		path := filepath.Join(scope, base)
		if err := emit.WriteFile(path, emit.WithHeader(sb.String()+"\n", emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	return nil
}

// emitEnvironment writes Cursor's background-agent bootstrap config to
// `.cursor/environment.json`. The spec body is the environment.json content:
// every key except the agnostic-ai routing fields passes through verbatim,
// so the author controls Cursor's schema (install, terminals, ...) while
// agnostic-ai single-sources it. Multiple environment specs merge by
// top-level key, last spec winning. The path is overridable via
// `outputs.cursor.environment-file`.
func emitEnvironment(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if len(b.Environments) == 0 {
		return nil
	}
	merged := map[string]any{}
	for _, e := range b.Environments {
		// Resolve the x-<target> namespace first so a cursor-specific
		// override wins and other targets' blocks are dropped, exactly like
		// every other adapter. Then strip the spec identity fields Cursor
		// has no schema for.
		for k, v := range emit.ResolveMeta(e.Meta, target) {
			if _, skip := environRoutingKeys[k]; skip {
				continue
			}
			merged[k] = v
		}
	}
	if len(merged) == 0 {
		return nil
	}
	raw, err := emit.MarshalJSONIndent(merged)
	if err != nil {
		return fmt.Errorf("cursor environment: %w", err)
	}
	path := emit.OutputEnvironmentFile(cfg, target, defaultEnvironFile)
	return emit.WriteFile(path, string(raw)+"\n", dryRun)
}

// scopeEscapesRoot reports whether a cleaned scope points at or above the
// repo root (a leading `..`), which would let an emitted file land outside
// the project tree.
func scopeEscapesRoot(scope string) bool {
	clean := filepath.ToSlash(filepath.Clean(scope))
	return clean == ".." || strings.HasPrefix(clean, "../")
}

// noteDroppedSkillAssets surfaces a coverage note for every folder-based
// skill that bundles sibling files. Cursor flattens each skill to a single
// `.cursor/rules/skill-<name>.mdc`, so attached scripts and assets have no
// native home and would otherwise vanish without a trace (#430).
func noteDroppedSkillAssets(b spec.Bundle) {
	n := 0
	for _, s := range b.Skills {
		if emit.SkillHasBundledAssets(s, emit.SkipSKILLMd) {
			n++
		}
	}
	emit.NoteCoverageGap(target, spec.KindSkill, n, "Cursor flattens skills to .mdc; bundled files are not emitted")
}

func mdc(e spec.Entry, alwaysApplyDefault bool) string {
	m := emit.ResolveMeta(e.Meta, target)
	desc, _ := m["description"].(string)
	globs, _ := m["globs"].(string)
	if globs == "" {
		globs = "**/*"
	}
	always := alwaysApplyDefault
	if v, ok := m["alwaysApply"].(bool); ok {
		always = v
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("description: " + desc + "\n")
	fmt.Fprintf(&b, "globs: %q\n", globs)
	fmt.Fprintf(&b, "alwaysApply: %t\n", always)
	b.WriteString("---\n\n")
	b.WriteString(e.Body)
	return b.String()
}
