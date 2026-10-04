package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// capabilitiesAgentToolsMigration rewrites an agent's `tools:` as `can:`,
// turning each Claude Code name a capability stands for alone into that
// capability. Every other name stays as an alias, which `can:` reads the
// same, so sync writes the same files after it.
var capabilitiesAgentToolsMigration = specMigration{
	ID:      "capabilities-agent-tools",
	Group:   "capabilities",
	Release: "0.79.0",
	Summary: "rewrite an agent's tools: as can:, with neutral capability names",
	Plan:    planCapabilitiesAgentTools,
}

func planCapabilitiesAgentTools(root string) ([]migrationChange, []migrationSkip, error) {
	cfg, b, err := loadProject(root)
	if err != nil {
		return nil, nil, err
	}
	extended, err := extendedSpecNames(root, cfg, func(lb spec.Bundle) []spec.Entry { return lb.Agents })
	if err != nil {
		return nil, nil, err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, nil, err
	}
	var changes []migrationChange
	var skips []migrationSkip
	for _, a := range b.Agents {
		raw, hasTools := a.Meta["tools"]
		if !hasTools {
			continue
		}
		skip := func(reason string) { skips = append(skips, migrationSkip{Path: a.Path, Reason: reason}) }
		if _, both := a.Meta["can"]; both {
			skips = append(skips, migrationSkip{Path: a.Path, Reason: "sets both tools: and can:; keep one by hand", Actionable: true})
			continue
		}
		if pack, ok := strings.CutPrefix(a.Layer, "pack:"); ok {
			skip("comes from pack " + pack + "; its author migrates it")
			continue
		}
		if extended[a.Name] {
			skips = append(skips, migrationSkip{Path: a.Path, Reason: "a local/ spec extends this agent; rewrite both files by hand", Actionable: true})
			continue
		}
		list, ok := raw.([]any)
		if !ok {
			skip("tools: is not a list")
			continue
		}
		items := map[int]string{}
		var kept []string
		tools := make([]string, len(list))
		for i, v := range list {
			name, _ := v.(string)
			tools[i] = name
			if c, ok := spec.NeutralCapability(name); ok {
				items[i] = c
			} else {
				kept = append(kept, name)
			}
		}
		if len(items) == 0 {
			skip("keeps tools: as written: no entry has a capability of its own")
			continue
		}
		if real, err := filepath.EvalSymlinks(a.Path); err != nil || !pathWithin(realRoot, real) {
			skip("resolves outside the project")
			continue
		}
		can := slices.Clone(tools)
		for i, c := range items {
			can[i] = c
		}
		if got, problem := spec.CapabilityTools(can); problem != "" || !slices.Equal(got, tools) {
			skip("can: would not stand for the same tools")
			continue
		}
		body, err := os.ReadFile(a.Path)
		if err != nil {
			return nil, nil, err
		}
		after, err := rewriteFrontmatter(string(body), func(front string) (string, error) {
			return rewriteTopLevelYAMLSequence(front, yamlSequenceRewrite{Key: "tools", NewKey: "can", Items: items})
		})
		if err != nil {
			skip("cannot rewrite in place: " + err.Error())
			continue
		}
		changes = append(changes, migrationChange{Path: a.Path, Before: string(body), After: after})
		for _, reason := range aliasReasons(kept) {
			skip(reason)
		}
	}
	return changes, skips, nil
}

// aliasReasons says why each kept Claude Code tool name has no
// capability of its own. A capability that stands for several names, all
// kept, gets one line, since writing it in their place is a manual edit.
func aliasReasons(kept []string) []string {
	var out []string
	done := map[string]bool{}
	for _, name := range kept {
		if done[name] {
			continue
		}
		reason := fmt.Sprintf("keeps %s as an alias: no capability stands for it alone", name)
		for _, c := range spec.Capabilities {
			names, _ := spec.CapabilityTools([]string{c})
			if len(names) < 2 || !slices.Contains(names, name) {
				continue
			}
			if slices.ContainsFunc(names, func(n string) bool { return !slices.Contains(kept, n) }) {
				others := slices.DeleteFunc(slices.Clone(names), func(n string) bool { return n == name })
				reason = fmt.Sprintf("keeps %s as an alias: %s also grants %s", name, c, strings.Join(others, " and "))
				break
			}
			for _, n := range names {
				done[n] = true
			}
			reason = fmt.Sprintf("keeps %s as aliases: %s stands for them together; write it in their place by hand", strings.Join(names, " and "), c)
			break
		}
		done[name] = true
		out = append(out, reason)
	}
	return out
}
