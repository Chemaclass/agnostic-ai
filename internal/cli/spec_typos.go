package cli

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/suggest"
)

// specTypos reports each hook event and agent skill name that is a likely
// typo of a known one. A name close to no known one may still be valid,
// such as a newer vendor event or a user's own skill, so only validate
// reports it.
func specTypos(b spec.Bundle, targets []string) []validationIssue {
	return append(hookEventTypos(b, targets), agentSkillTypos(b)...)
}

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
		for _, e := range b.Hooks {
			event, _ := e.Meta["event"].(string)
			if _, ok := known[event]; event == "" || ok {
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
			if slices.Contains(skills, name) {
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

// specTyposError stops a sync on the typos specTypos finds, before it
// writes them into every tool's files.
func specTyposError(b spec.Bundle, targets []string) error {
	issues := specTypos(b, targets)
	if len(issues) == 0 {
		return nil
	}
	lines := make([]string, len(issues))
	for i, is := range issues {
		lines[i] = is.Path + ": " + is.Message
	}
	return fmt.Errorf("%s", strings.Join(lines, "\n"))
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
