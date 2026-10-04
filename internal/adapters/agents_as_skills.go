package adapters

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// AgentsAsSkills is the `outputs.<target>.agents` value that writes each
// agent as an on-demand skill on a target without subagents.
const AgentsAsSkills = "skill"

// agentSkillPreamble opens each agent written as a skill. Nothing
// isolates the role on such a target, so the text asks for it.
const agentSkillPreamble = "This tool has no subagents, so run this role inline. " +
	"Set aside the framing of the current task, follow only the instructions below, " +
	"and say in one line that the role ran inline."

// subagentlessTargets write skill folders but drop agents by default.
// Some still declare the agent kind, for an opt-in rules document that
// carries agent bodies, so the capability list cannot tell them apart.
var subagentlessTargets = map[string]bool{"amp": true, "crush": true, "warp": true, "zed": true}

// AgentsAsSkillsTarget reports whether target can write agents as skills.
func AgentsAsSkillsTarget(target string) bool { return subagentlessTargets[target] }

// agentSkillFields are the agent fields a skill carries over.
var agentSkillFields = map[string]bool{
	"name": true, "description": true,
	"target": true, "targets": true, "target-exclude": true, "targets-exclude": true,
}

// WritesAgentsAsSkills reports whether target writes agents as skills:
// it opted in, has skills and no agents, and no other enabled target
// with agents reads its skills directory.
func WritesAgentsAsSkills(cfg *config.Config, target string) bool {
	ok, _ := agentsAsSkills(cfg, target)
	return ok
}

// agentsAsSkills is WritesAgentsAsSkills plus, when another target's
// native agents would see the skills, those targets.
func agentsAsSkills(cfg *config.Config, target string) (bool, []string) {
	if cfg == nil || cfg.Outputs[target].Agents != AgentsAsSkills {
		return false, nil
	}
	if !subagentlessTargets[target] {
		return false, nil
	}
	dir := varsFor(cfg, target)[emit.VarSkillsDir]
	if dir == "" {
		return false, nil
	}
	var conflicts []string
	for _, t := range cfg.Targets {
		other, ok := Get(t)
		if t == target || subagentlessTargets[t] || !ok || !slices.Contains(other.Capabilities(), spec.KindAgent) {
			continue
		}
		if filepath.Clean(varsFor(cfg, t)[emit.VarSkillsDir]) == filepath.Clean(dir) {
			conflicts = append(conflicts, t)
		}
	}
	sort.Strings(conflicts)
	return len(conflicts) == 0, conflicts
}

// withAgentsAsSkills returns b with its agents moved into its skills when
// target writes agents as skills. A skill and an agent of one name would
// write one folder, so that fails.
func withAgentsAsSkills(b spec.Bundle, cfg *config.Config, target string) (spec.Bundle, error) {
	if len(b.Agents) == 0 {
		return b, nil
	}
	ok, conflicts := agentsAsSkills(cfg, target)
	if !ok {
		if len(conflicts) > 0 {
			emit.NoteSurfaceGap(target, spec.KindAgent, len(b.Agents), "as skills",
				"outputs."+target+".agents: skill stays off: "+strings.Join(conflicts, ", ")+
					" also reads that skills directory and has subagents")
		}
		return b, nil
	}
	skills := map[string]bool{}
	for _, s := range b.Skills {
		skills[s.Name] = true
	}
	dropped := map[string]int{}
	converted := make([]spec.Entry, 0, len(b.Agents))
	for _, a := range b.Agents {
		if skills[a.Name] {
			return b, fmt.Errorf("%s: agent %q and skill %q both write the %s skill folder %q; rename one",
				a.Path, a.Name, a.Name, target, a.Name)
		}
		converted = append(converted, agentSkill(a, dropped))
	}
	for field, count := range dropped {
		emit.NoteFieldNoOp(target, spec.KindAgent, field, count, "the agent is written as a skill, which has no such field")
	}
	emit.NoteSurfaceGap(target, spec.KindAgent, len(b.Agents), "as subagents",
		"outputs."+target+".agents: skill writes each as an on-demand skill")
	b.Skills = append(slices.Clone(b.Skills), converted...)
	b.Agents = nil
	return b, nil
}

// agentSkill is agent a as a skill: its name and description, and the
// preamble before its body. dropped counts the fields left behind.
func agentSkill(a spec.Entry, dropped map[string]int) spec.Entry {
	s := a
	s.Kind = spec.KindSkill
	s.AssetDir = ""
	s.Meta = map[string]any{}
	s.MetaKeys = nil
	for _, k := range a.MetaKeys {
		if _, ok := a.Meta[k]; ok && agentSkillFields[k] {
			s.Meta[k] = a.Meta[k]
			s.MetaKeys = append(s.MetaKeys, k)
		}
	}
	for k := range a.Meta {
		if !agentSkillFields[k] && !strings.HasPrefix(k, "x-") {
			dropped[k]++
		} else if !slices.Contains(s.MetaKeys, k) && agentSkillFields[k] {
			s.Meta[k] = a.Meta[k]
			s.MetaKeys = append(s.MetaKeys, k)
		}
	}
	s.Body = agentSkillPreamble + "\n\n" + a.Body
	return s
}
