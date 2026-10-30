package cli

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/suggest"
)

// hookEventTypos compares with the events every target reads, not only
// the enabled ones: hooks carry each tool's own names, so another tool's
// event is not a typo.
func hookEventTypos(b spec.Bundle, targets []string) []validationIssue {
	var out []validationIssue
	if len(unionEvents(targets)) > 0 {
		known := unionEvents(slices.Collect(maps.Keys(hookEventsByTarget)))
		// One spelling per event, the enabled targets' first, or
		// PreToolUse and preToolUse tie and suggest nothing.
		byFold := map[string]string{}
		for _, set := range []map[string]struct{}{unionEvents(targets), known} {
			for _, ev := range sortedKeys(set) {
				if _, ok := byFold[strings.ToLower(ev)]; !ok {
					byFold[strings.ToLower(ev)] = ev
				}
			}
		}
		events := slices.Sorted(maps.Values(byFold))
		// Some tools take any case or snake_case, as OpenHands'
		// session_start for SessionStart, so a spelling that only
		// differs that way is known.
		loose := map[string]bool{}
		for ev := range known {
			loose[looseEvent(ev)] = true
		}
		for _, e := range b.Hooks {
			event, _ := e.Meta["event"].(string)
			if event == "" || loose[looseEvent(event)] {
				continue
			}
			if _, _, alias := hookEventAlias(targets, event); alias {
				continue
			}
			if s := suggest.Name(event, events); s != "" {
				out = append(out, validationIssue{Path: e.Path, Field: "event",
					Message: fmt.Sprintf("unknown hook event %q (did you mean %s?)", event, s)})
			}
		}
	}
	return out
}

func agentSkillTypos(b spec.Bundle) []validationIssue {
	var out []validationIssue
	skills := make([]string, 0, len(b.Skills))
	for _, s := range b.Skills {
		skills = append(skills, s.Name)
	}
	for _, a := range b.Agents {
		for _, name := range agentSkills(a.Meta["skills"]) {
			// plugin:skill names a plugin's skill, never a project one.
			if slices.Contains(skills, name) || strings.Contains(name, ":") {
				continue
			}
			if s := suggest.Name(name, skills); s != "" {
				out = append(out, validationIssue{Path: a.Path, Field: "skills",
					Message: fmt.Sprintf("unknown skill %q (did you mean %s?)", name, s)})
			}
		}
	}
	return out
}

// stopOnSpecTypos stops a sync on a hook event typo, an invalid portable
// hook, an agent `can:` or permission rule it cannot read, or an unmarked
// MCP literal (LINT035), before it
// is written into every tool's files. Pack specs are not the user's to
// edit, so validate reports them, except a permission rule: a target
// drops one it cannot read, so a lost deny rule would widen access. A
// pack MCP literal an agent file would carry is refused too.
func stopOnSpecTypos(b spec.Bundle, targets []string) error {
	own := ownSpecs(b)
	issues := append(hookEventTypos(own, targets), portableHookProblems(own.Hooks)...)
	issues = append(issues, agentCapabilityIssues(own.Agents)...)
	issues = append(issues, agentLimitIssues(own.Agents)...)
	issues = append(issues, skillCapabilityIssues(b.Skills)...)
	issues = append(issues, sessionRetentionIssues(b.Settings)...)
	issues = append(issues, voiceIssues(b.Settings)...)
	issues = append(issues, mcpLiteralIssues(own.MCPs)...)
	inlined := agentFileMCPs(b, targets)
	issues = append(issues, mcpLiteralIssues(packSpecs(inlined))...)
	issues = append(issues, agentFileCredentialIssues(inlined)...)
	issues = append(issues, agentOnlyIssues(b.MCPs)...)
	if slices.Contains(targets, "ellipsis") {
		issues = append(issues, ellipsisVariableIssues(b.Settings)...)
	}
	return stopOnIssues(append(issues, permissionCapabilityIssues(b.Settings)...))
}

// permissionCapabilityIssues reports each settings permission rule
// written as a capability that cannot be read.
func permissionCapabilityIssues(settings []spec.Entry) []validationIssue {
	var out []validationIssue
	for _, e := range settings {
		for _, problem := range spec.SettingsPermissionProblems(e.Meta) {
			out = append(out, validationIssue{Path: e.Path, Field: "permissions", Message: problem})
		}
	}
	return out
}

// agentCapabilityIssues reports each agent whose `can:` cannot be read.
func agentCapabilityIssues(agents []spec.Entry) []validationIssue {
	var out []validationIssue
	for _, e := range agents {
		if problem := spec.AgentCapabilityProblem(e.Meta); problem != "" {
			out = append(out, validationIssue{Path: e.Path, Field: "can", Message: problem})
		}
	}
	return out
}

// agentLimitIssues reports each agent whose portable `maxTurns` or
// `background` has the wrong type. Sync stops on one rather than write a
// value the tool rejects or reads as something else.
func agentLimitIssues(agents []spec.Entry) []validationIssue {
	var out []validationIssue
	for _, e := range agents {
		problems := spec.AgentLimitProblems(e.Meta)
		for _, field := range slices.Sorted(maps.Keys(problems)) {
			out = append(out, validationIssue{Path: e.Path, Field: field, Message: problems[field]})
		}
	}
	return out
}

// lintAgentLimits reports the agentLimitIssues validate reports
// (LINT050, error).
func lintAgentLimits(agents []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, issue := range agentLimitIssues(agents) {
		out = append(out, lintFinding{Code: "LINT050", Severity: lintError, Path: issue.Path, Message: issue.Message})
	}
	return out
}

// lintAgentCapabilities reports the `can:` problems validate reports
// (LINT036, error).
func lintAgentCapabilities(agents, settings []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, issue := range append(agentCapabilityIssues(agents), permissionCapabilityIssues(settings)...) {
		out = append(out, lintFinding{Code: "LINT036", Severity: lintError, Path: issue.Path, Message: issue.Message})
	}
	return out
}

// portableHookProblems reports each portable hook no target can read.
func portableHookProblems(hooks []spec.Entry) []validationIssue {
	var out []validationIssue
	for _, e := range hooks {
		if !spec.IsPortableHook(e.Meta) {
			continue
		}
		if problem := spec.PortableHookProblem(e.Meta); problem != "" {
			out = append(out, validationIssue{Path: e.Path, Field: "on", Message: problem})
		}
	}
	return out
}

func stopOnIssues(issues []validationIssue) error {
	if len(issues) == 0 {
		return nil
	}
	lines := make([]string, len(issues))
	for i, is := range issues {
		lines[i] = is.Path + ": " + is.Message
	}
	return fmt.Errorf("%s", strings.Join(lines, "\n"))
}

// agentFileMCPs returns the MCP specs sync may write in full into an
// agent file for a target in targets: the agent-only ones, and every
// server an agent names where a target writes each named server.
func agentFileMCPs(b spec.Bundle, targets []string) []spec.Entry {
	agentOnly, named := false, false
	for _, t := range targets {
		o, n := adapters.AgentMCPInlining(t)
		agentOnly, named = agentOnly || o, named || n
	}
	names := map[string]bool{}
	for _, a := range b.Agents {
		for _, name := range spec.AgentMCPNames(a.Meta) {
			names[name] = true
		}
	}
	var out []spec.Entry
	for _, e := range b.MCPs {
		if e.AgentOnly() && agentOnly || named && names[e.Name] {
			out = append(out, e)
		}
	}
	return out
}

func packSpecs(entries []spec.Entry) []spec.Entry {
	var out []spec.Entry
	for _, e := range entries {
		if strings.HasPrefix(e.Layer, "pack:") {
			out = append(out, e)
		}
	}
	return out
}

// agentOnlyIssues reports an `agent-only` value that is not a boolean,
// which would otherwise write the server into every MCP file.
func agentOnlyIssues(mcps []spec.Entry) []validationIssue {
	var out []validationIssue
	for _, e := range mcps {
		if v, set := e.Meta[spec.AgentOnlyKey]; set {
			if _, ok := v.(bool); !ok {
				out = append(out, validationIssue{Path: e.Path, Field: spec.AgentOnlyKey,
					Message: fmt.Sprintf("MCP server %q: agent-only must be true or false", e.Name)})
			}
		}
	}
	return out
}

// ownSpecs keeps the hooks, agents, and MCP servers outside packs, with every skill an
// agent may name.
func ownSpecs(b spec.Bundle) spec.Bundle {
	own := spec.Bundle{Skills: b.Skills}
	for _, e := range b.Hooks {
		if !strings.HasPrefix(e.Layer, "pack:") {
			own.Hooks = append(own.Hooks, e)
		}
	}
	for _, e := range b.Agents {
		if !strings.HasPrefix(e.Layer, "pack:") {
			own.Agents = append(own.Agents, e)
		}
	}
	for _, e := range b.MCPs {
		if !strings.HasPrefix(e.Layer, "pack:") {
			own.MCPs = append(own.MCPs, e)
		}
	}
	return own
}

// looseEvent folds case and underscores, so session_start and
// SessionStart compare equal.
func looseEvent(event string) string {
	return strings.ToLower(strings.ReplaceAll(event, "_", ""))
}

// agentSkills reads an agent's skills field: a list, or one
// comma-separated string.
func agentSkills(v any) []string {
	switch v := v.(type) {
	case string:
		var out []string
		for _, s := range strings.Split(v, ",") {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		return toStringSlice(v)
	}
	return nil
}

func skillCapabilityIssues(skills []spec.Entry) []validationIssue {
	var out []validationIssue
	for _, e := range skills {
		if problem := spec.SkillCapabilityProblem(e.Meta); problem != "" {
			out = append(out, validationIssue{Path: e.Path, Field: "allowed-tools", Message: problem})
		}
	}
	return out
}
