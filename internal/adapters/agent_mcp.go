package adapters

import (
	"maps"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/claude"
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
	"copilot":     "sync writes no inline agent server here; set x-copilot.mcp-servers",
	"factory":     "a Factory agent names servers from the project MCP file only",
	"gemini":      "sync writes no inline agent server here; set x-gemini.mcp_servers",
	"junie":       "a Junie agent names servers from the project MCP file only",
}

// namesOnlyTargets write an agent's mcpServers as names from the project
// MCP file, so a name of an agent-only server would match nothing.
var namesOnlyTargets = map[string]bool{"factory": true, "junie": true}

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
	if caps := a.Capabilities(); caps != nil && !slices.Contains(caps, spec.KindMCP) {
		return b
	}
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
		if namesOnlyTargets[target] {
			b.Agents = withoutAgentOnlyNames(b.Agents, target, only)
		}
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
		// Claude Code gets no file for an agent with an empty tool list.
		if target == "claude" && claude.LeavesOutAgent(agent) {
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
		missing := false
		for _, item := range list {
			name, _ := item.(string)
			e, found := byName[name]
			if !found {
				missing = missing || !form.Names
				continue
			}
			if form.Names && !e.AgentOnly() {
				continue
			}
			ownOnly[name] = e.AgentOnly()
			inline = append(inline, e)
		}
		if missing {
			unknown++
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

// withoutAgentOnlyNames drops the names of agent-only servers from each
// agent's mcpServers list. Junie reads an empty list as every server,
// so a Junie list that would end up empty stays as written: its names
// match no server.
func withoutAgentOnlyNames(agents []spec.Entry, target string, only []spec.Entry) []spec.Entry {
	agentOnly := map[string]bool{}
	for _, e := range only {
		agentOnly[e.Name] = true
	}
	out := slices.Clone(agents)
	for i, agent := range out {
		custom, _ := agent.Meta[emit.XPrefix+target].(map[string]any)
		if _, explicit := custom["mcpServers"]; explicit {
			continue
		}
		list, _ := agent.Meta["mcpServers"].([]any)
		kept := slices.DeleteFunc(slices.Clone(list), func(item any) bool {
			name, _ := item.(string)
			return agentOnly[name]
		})
		if len(kept) == len(list) || len(kept) == 0 && target == "junie" {
			continue
		}
		out[i].Meta = maps.Clone(agent.Meta)
		out[i].Meta["mcpServers"] = kept
	}
	return out
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

// AgentMCPInlining reports which MCP servers target writes in full into
// an agent file: agent-only ones the agent names, and, when named is
// true, every server the agent names.
func AgentMCPInlining(target string) (agentOnly, named bool) {
	a, err := Resolve(target)
	if err != nil {
		return false, false
	}
	inliner, ok := a.(agentMCPInliner)
	if !ok {
		return false, false
	}
	return true, !inliner.AgentMCPForm().Names
}
