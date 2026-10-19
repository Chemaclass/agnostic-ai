package adapters

import (
	"maps"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// agentMCPInliner is implemented by a target whose agent file can hold
// an MCP server definition.
type agentMCPInliner interface {
	AgentMCPForm() emit.AgentMCPForm
	// AgentMCPServers renders mcps as the agent file's inline server
	// definitions, keyed by server name.
	AgentMCPServers(mcps []spec.Entry) map[string]any
}

// agentOnlyGapReasons explains, per target without inline agent
// servers, why an agent-only server reaches no agent there.
var agentOnlyGapReasons = map[string]string{
	"antigravity": "Antigravity documents no shape for an inline agent server; set x-antigravity.mcpServers",
	"factory":     "a Factory agent names servers from the project MCP file only",
	"junie":       "a Junie agent names servers from the project MCP file only",
}

const (
	agentOnlyGapReason  = "the agent file cannot define a server"
	agentOnlyUnusedNote = "no agent names it"
	gooseSharedReason   = "OpenHands shares .agents/agents/ with Goose, whose agent file has no MCP key; set outputs.openhands.agents-dir to write the servers"
)

// withAgentMCPServers keeps agent-only MCP specs out of the project MCP
// file and writes the servers each agent names into its agent file
// where the target's agent file takes definitions.
func withAgentMCPServers(a Adapter, b spec.Bundle, cfg *config.Config) spec.Bundle {
	target := a.Name()
	shared, only := spec.SplitAgentOnly(b.MCPs)
	b.MCPs = shared
	for _, e := range only {
		emit.NoteEntryOmitted(target, spec.KindMCP, e.Name)
	}
	inliner, ok := a.(agentMCPInliner)
	if ok && sharesGooseAgents(target, cfg) {
		withList := 0
		for _, agent := range b.Agents {
			if _, set := agent.Meta["mcpServers"]; set {
				withList++
			}
		}
		emit.NoteFieldNoOp(target, spec.KindAgent, "mcpServers", withList, gooseSharedReason)
		noteAgentOnlyGap(target, len(only), gooseSharedReason)
		return b
	}
	if !ok {
		reason, found := agentOnlyGapReasons[target]
		if !found {
			reason = agentOnlyGapReason
		}
		noteAgentOnlyGap(target, len(only), reason)
		return b
	}
	form := inliner.AgentMCPForm()
	byName := map[string]spec.Entry{}
	for _, e := range append(slices.Clone(shared), only...) {
		byName[e.Name] = e
	}
	named := map[string]bool{}
	unknown := 0
	agents := slices.Clone(b.Agents)
	for i, agent := range agents {
		if _, set := agent.Meta["mcpServers"]; !set {
			continue
		}
		list, _ := agent.Meta["mcpServers"].([]any)
		for _, name := range spec.AgentMCPNames(agent.Meta) {
			named[name] = true
		}
		custom, _ := agent.Meta[emit.XPrefix+target].(map[string]any)
		if _, explicit := custom[form.Key]; explicit {
			continue
		}
		var inline []spec.Entry
		ownOnly := map[string]bool{}
		for _, item := range list {
			name, _ := item.(string)
			e, found := byName[name]
			if !found {
				if !form.Names {
					unknown++
				}
				continue
			}
			if form.Names && !e.AgentOnly() {
				continue
			}
			ownOnly[name] = e.AgentOnly()
			inline = append(inline, e)
		}
		agents[i] = withAgentServers(agent, target, form, inliner.AgentMCPServers(inline), ownOnly)
	}
	b.Agents = agents
	unused := 0
	for _, e := range only {
		if !named[e.Name] {
			unused++
		}
	}
	noteAgentOnlyGap(target, unused, agentOnlyUnusedNote)
	emit.NoteFieldNoOp(target, spec.KindAgent, "mcpServers", unknown, "only a name with an MCP spec can be written as an inline server")
	return b
}

// withAgentServers returns agent with servers in its agent file key. A
// name in agentOnly that servers left out, such as a disabled server,
// is dropped from a list of names, since no MCP file defines it.
func withAgentServers(agent spec.Entry, target string, form emit.AgentMCPForm, servers map[string]any, agentOnly map[string]bool) spec.Entry {
	if len(servers) == 0 && (!form.Names || len(agentOnly) == 0) {
		return agent
	}
	agent.Meta = maps.Clone(agent.Meta)
	if form.Names {
		list, _ := agent.Meta["mcpServers"].([]any)
		out := make([]any, 0, len(list))
		for _, item := range list {
			name, _ := item.(string)
			if server, inline := servers[name]; inline {
				out = append(out, map[string]any{name: server})
				continue
			}
			if agentOnly[name] {
				continue
			}
			out = append(out, item)
		}
		agent.Meta["mcpServers"] = out
		if _, styled := agent.MetaStyles["mcpServers"]; styled {
			agent.MetaStyles = maps.Clone(agent.MetaStyles)
			delete(agent.MetaStyles, "mcpServers")
		}
		return agent
	}
	custom, _ := agent.Meta[emit.XPrefix+target].(map[string]any)
	custom = maps.Clone(custom)
	if custom == nil {
		custom = map[string]any{}
	}
	custom[form.Key] = servers
	agent.Meta[emit.XPrefix+target] = custom
	return agent
}

// noteAgentOnlyGap notes count agent-only servers that reach no agent
// on target.
func noteAgentOnlyGap(target string, count int, reason string) {
	emit.NoteCoverageGap(target, spec.KindMCP, count, "agent-only, and "+reason)
}

// sharesGooseAgents reports whether OpenHands writes its agents to the
// folder Goose writes too, where both must write the same bytes.
func sharesGooseAgents(target string, cfg *config.Config) bool {
	if target != "openhands" || cfg == nil || !slices.Contains(cfg.Targets, "goose") {
		return false
	}
	const shared = ".agents/agents"
	return emit.OutputAgentsDir(cfg, "openhands", shared) == emit.OutputAgentsDir(cfg, "goose", shared)
}
