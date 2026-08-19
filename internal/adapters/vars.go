package adapters

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// targetVarPaths declares, per target, the directory or file each spec
// variable resolves to. A kind is listed only when the target has a
// dedicated surface for it. Several targets flatten agents into their
// rules directory with a filename prefix (antigravity, continue, trae,
// windsurf) or render them as commands (gemini); naming those
// AGENTS_DIR would point users at a directory that is not an agents
// directory, so they are left out and the variable stays unresolved.
//
// TestTargetVarPaths_MatchRealEmission keeps this honest: every entry
// here must be where the adapter actually writes that kind.
var targetVarPaths = map[string]map[string]string{
	"claude": {
		emit.VarSkillsDir: ".claude/skills", emit.VarAgentsDir: ".claude/agents",
		emit.VarCommandsDir: ".claude/commands", emit.VarRulesDir: ".claude/rules",
		emit.VarMCPFile: ".mcp.json",
	},
	"codex": {
		emit.VarSkillsDir: ".agents/skills", emit.VarAgentsDir: ".codex/agents",
		emit.VarMCPFile: ".codex/config.toml",
	},
	"gemini": {
		emit.VarSkillsDir: ".gemini/skills", emit.VarCommandsDir: ".gemini/commands",
		emit.VarMCPFile: ".gemini/settings.json",
	},
	"cursor": {
		emit.VarSkillsDir: ".cursor/skills", emit.VarAgentsDir: ".cursor/agents",
		emit.VarCommandsDir: ".cursor/commands", emit.VarRulesDir: ".cursor/rules",
		emit.VarMCPFile: ".cursor/mcp.json",
	},
	"copilot": {
		emit.VarSkillsDir: ".github/skills", emit.VarAgentsDir: ".github/agents",
		emit.VarRulesDir: ".github/instructions", emit.VarMCPFile: ".vscode/mcp.json",
	},
	"cline": {
		emit.VarSkillsDir: ".cline/skills", emit.VarAgentsDir: ".cline/agents",
		emit.VarRulesDir: ".cline/rules",
	},
	"windsurf": {
		emit.VarSkillsDir: ".agents/skills", emit.VarRulesDir: ".devin/rules",
		emit.VarMCPFile: ".devin/mcp_config.json",
	},
	"continue": {
		emit.VarRulesDir: ".continue/rules",
	},
	// No COMMANDS_DIR: Amp documents no file surface for commands, so
	// the adapter skips that kind with a warning (#553).
	"amp": {
		emit.VarSkillsDir: ".agents/skills", emit.VarMCPFile: ".amp/settings.json",
	},
	"zed": {
		emit.VarSkillsDir: ".agents/skills", emit.VarMCPFile: ".zed/settings.json",
	},
	"warp": {
		emit.VarSkillsDir: ".agents/skills", emit.VarMCPFile: ".warp/.mcp.json",
	},
	"opencode": {
		emit.VarSkillsDir: ".opencode/skills", emit.VarAgentsDir: ".opencode/agents",
		emit.VarCommandsDir: ".opencode/commands", emit.VarMCPFile: "opencode.json",
	},
	"antigravity": {
		emit.VarSkillsDir: ".agents/skills", emit.VarRulesDir: ".agents/rules",
		emit.VarMCPFile: ".agents/mcp_config.json",
	},
	"junie": {
		emit.VarSkillsDir: ".junie/skills", emit.VarAgentsDir: ".junie/agents",
		emit.VarCommandsDir: ".junie/commands", emit.VarMCPFile: ".junie/mcp/mcp.json",
	},
	"kiro": {
		emit.VarAgentsDir: ".kiro/agents", emit.VarRulesDir: ".kiro/steering",
		emit.VarMCPFile: ".kiro/settings/mcp.json",
	},
	"crush": {
		emit.VarSkillsDir: ".agents/skills", emit.VarMCPFile: "crush.json",
	},
	"trae": {
		emit.VarSkillsDir: ".trae/skills", emit.VarCommandsDir: ".trae/commands",
		emit.VarRulesDir: ".trae/rules", emit.VarMCPFile: ".trae/mcp.json",
	},
	"augment": {
		emit.VarSkillsDir: ".agents/skills", emit.VarAgentsDir: ".augment/agents",
		emit.VarRulesDir: ".augment/rules",
	},
	"qoder": {
		emit.VarSkillsDir: ".qoder/skills", emit.VarAgentsDir: ".qoder/agents",
		emit.VarRulesDir: ".qoder/rules", emit.VarMCPFile: ".mcp.json",
	},
	"openhands": {
		emit.VarSkillsDir: ".agents/skills", emit.VarMCPFile: "config.toml",
	},
	"factory": {
		emit.VarAgentsDir: ".factory/droids", emit.VarMCPFile: ".factory/mcp.json",
	},
	"kilo": {
		emit.VarSkillsDir: ".agents/skills", emit.VarAgentsDir: ".kilo/agents",
		emit.VarRulesDir: ".kilo/rules", emit.VarMCPFile: "kilo.jsonc",
	},
	// aider, jules, and goose carry every spec kind in one entry-point
	// document and have no per-kind directory to point at.
	"aider": {},
	"jules": {},
	"goose": {},
}

// varsFor resolves the variable table for target, letting an
// outputs.<target>.<field> override win over the declared default so a
// spec body and the emitted tree never disagree about where files land.
func varsFor(cfg *config.Config, target string) map[string]string {
	declared := targetVarPaths[target]
	if len(declared) == 0 {
		return nil
	}
	out := make(map[string]string, len(declared))
	for name, fallback := range declared {
		switch name {
		case emit.VarSkillsDir:
			out[name] = emit.OutputSkillsDir(cfg, target, fallback)
		case emit.VarAgentsDir:
			out[name] = emit.OutputAgentsDir(cfg, target, fallback)
		case emit.VarCommandsDir:
			out[name] = emit.OutputCommandsDir(cfg, target, fallback)
		case emit.VarRulesDir:
			out[name] = emit.OutputRulesDir(cfg, target, fallback)
		case emit.VarMCPFile:
			out[name] = emit.OutputMCPFile(cfg, target, fallback)
		}
	}
	return out
}

// expandBundleVars returns b with every entry body expanded for target.
// Entries are copied, so the caller's bundle is untouched and each
// target expands the same source spec to its own paths.
func expandBundleVars(b spec.Bundle, cfg *config.Config, target string) spec.Bundle {
	vals := varsFor(cfg, target)
	// Count entries per unresolved variable so the coverage note says
	// how much of the project is affected, not just that it happened.
	unresolved := map[string]int{}
	kindOf := map[string]spec.Kind{}
	expand := func(entries []spec.Entry, kind spec.Kind) []spec.Entry {
		if len(entries) == 0 {
			return entries
		}
		out := make([]spec.Entry, len(entries))
		copy(out, entries)
		for i := range out {
			body, missing := emit.ExpandVars(out[i].Body, vals)
			out[i].Body = body
			for _, name := range missing {
				unresolved[name]++
				if _, seen := kindOf[name]; !seen {
					kindOf[name] = kind
				}
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
	return b
}
