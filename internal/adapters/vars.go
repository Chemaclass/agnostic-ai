package adapters

import (
	"maps"
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// expandBundleVars returns b with every entry body expanded for target.
// Entries are copied, so the caller's bundle is untouched and each
// target expands the same source spec to its own paths.
func expandBundleVars(b spec.Bundle, cfg *config.Config, target string) spec.Bundle {
	vals := emit.VarsFor(cfg, target)
	// Count entries per unresolved variable so the coverage note says
	// how much of the project is affected, not just that it happened.
	unresolved := map[string]int{}
	kindOf := map[string]spec.Kind{}
	forms := emit.RefForms[target]
	agentsAreSkills := agentsInSkillsDir(cfg, target)
	var emits []spec.Kind
	if a, ok := Get(target); ok {
		emits = a.Capabilities()
	}
	type plainRef struct {
		keyword string
		kind    spec.Kind
	}
	neutral := map[plainRef]int{}
	expand := func(entries []spec.Entry, kind spec.Kind) []spec.Entry {
		// A rule keeps its variables and references until
		// emit.PrepareScopedDocuments knows whether it lands in a
		// document other tools share.
		keepRaw := kind == spec.KindRule
		if len(entries) == 0 {
			return entries
		}
		kindForms := forms
		if sharesKindDir(cfg, target, kind, vals) {
			kindForms = nil
		}
		// An agent written as a skill is invoked as one.
		if agentsAreSkills {
			kindForms = maps.Clone(kindForms)
			if kindForms == nil {
				kindForms = map[string]string{}
			}
			kindForms[emit.RefAgent] = kindForms[emit.RefSkill]
			if kindForms[emit.RefAgent] == "" {
				kindForms[emit.RefAgent] = "the %s skill"
			}
		}
		out := make([]spec.Entry, len(entries))
		copy(out, entries)
		for i := range out {
			body, missing := emit.ExpandVars(out[i].Body, vals)
			if !keepRaw {
				out[i].Body = body
			}
			for _, name := range missing {
				unresolved[name]++
				if _, seen := kindOf[name]; !seen {
					kindOf[name] = kind
				}
			}
			expanded, plain := emit.ExpandRefs(out[i].Body, kindForms)
			if !keepRaw {
				out[i].Body = expanded
			}
			if !slices.Contains(emits, kind) {
				continue
			}
			// A target with no rules directory reads its rules from an
			// entry point, where references take the neutral phrase.
			if kind == spec.KindRule && emit.TargetVarPaths[target][emit.VarRulesDir] == "" {
				plain = refKeywords(out[i].Body)
			}
			for _, keyword := range plain {
				neutral[plainRef{keyword, kind}]++
			}
		}
		return out
	}
	// Every kind that carries a body. Skipping some would make the
	// feature's reach depend on which kind a user happened to write in.
	b.Agents = expand(b.Agents, spec.KindAgent)
	b.Skills = expand(b.Skills, spec.KindSkill)
	b.Rules = expand(b.Rules, spec.KindRule)
	b.Commands = expand(b.Commands, spec.KindCommand)
	b.Hooks = expand(b.Hooks, spec.KindHook)
	b.Reviews = expand(b.Reviews, spec.KindReview)
	b.Settings = expand(b.Settings, spec.KindSettings)
	b.Environments = expand(b.Environments, spec.KindEnvironment)
	b.Ignores = expand(b.Ignores, spec.KindIgnore)
	b.MCPs = expand(b.MCPs, spec.KindMCP)
	for name, count := range unresolved {
		emit.NoteFieldNoOp(target, kindOf[name], "{{$"+name+"}}", count,
			"this target has no surface for that path, so the variable is left verbatim rather than blanked")
	}
	for ref, count := range neutral {
		emit.NoteFieldNoOp(target, ref.kind, "{{$"+ref.keyword+":<name>}}", count,
			emit.SharedRefReason)
	}
	return b
}

// kindDirVars names the variable for the directory each kind lands in.
var kindDirVars = map[spec.Kind]string{
	spec.KindSkill:   emit.VarSkillsDir,
	spec.KindAgent:   emit.VarAgentsDir,
	spec.KindCommand: emit.VarCommandsDir,
}

// sharesKindDir reports whether another configured target writes kind to
// the same directory as target. Both write one file there, so it takes
// the neutral phrase rather than either tool's own.
func sharesKindDir(cfg *config.Config, target string, kind spec.Kind, vals map[string]string) bool {
	name, ok := kindDirVars[kind]
	if !ok || cfg == nil || vals[name] == "" {
		return false
	}
	dir := filepath.Clean(vals[name])
	for _, t := range cfg.Targets {
		if t != target && filepath.Clean(emit.VarsFor(cfg, t)[name]) == dir {
			return true
		}
	}
	return false
}

// refKeywords returns the distinct reference keywords in body, sorted.
func refKeywords(body string) []string {
	seen := map[string]bool{}
	var out []string
	for _, ref := range BodyRefs(body) {
		if !seen[ref.Keyword] {
			seen[ref.Keyword] = true
			out = append(out, ref.Keyword)
		}
	}
	slices.Sort(out)
	return out
}
