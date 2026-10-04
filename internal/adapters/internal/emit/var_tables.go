package emit

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// TargetVarPaths declares, per target, the directory or file each spec
// variable resolves to. A kind is listed only when the target has a
// dedicated surface for it. Several targets flatten agents into their
// rules directory with a filename prefix (continue, trae, windsurf) or
// render them as commands (gemini); naming those
// AGENTS_DIR would point users at a directory that is not an agents
// directory, so they are left out and the variable stays unresolved.
//
// TestTargetVarPaths_MatchRealEmission keeps this honest: every entry
// here must be where the adapter actually writes that kind.
var TargetVarPaths = map[string]map[string]string{
	"claude": {
		VarSkillsDir: ".claude/skills", VarAgentsDir: ".claude/agents",
		VarCommandsDir: ".claude/commands", VarRulesDir: ".claude/rules",
		VarMCPFile: ".mcp.json",
	},
	"codex": {
		VarSkillsDir: ".agents/skills", VarAgentsDir: ".codex/agents",
		VarMCPFile: ".codex/config.toml",
	},
	"gemini": {
		VarSkillsDir: ".gemini/skills", VarCommandsDir: ".gemini/commands",
		VarMCPFile: ".gemini/settings.json",
	},
	"cursor": {
		VarSkillsDir: ".cursor/skills", VarAgentsDir: ".cursor/agents",
		VarCommandsDir: ".cursor/commands", VarRulesDir: ".cursor/rules",
		VarMCPFile: ".cursor/mcp.json",
	},
	"copilot": {
		VarSkillsDir: ".github/skills", VarAgentsDir: ".github/agents",
		VarRulesDir: ".github/instructions", VarMCPFile: ".vscode/mcp.json",
	},
	"cline": {
		VarSkillsDir: ".cline/skills", VarAgentsDir: ".cline/agents",
		VarRulesDir: ".clinerules",
	},
	"windsurf": {
		VarSkillsDir: ".agents/skills", VarRulesDir: ".devin/rules",
		VarMCPFile: ".devin/mcp_config.json",
	},
	"continue": {
		VarRulesDir: ".continue/rules",
	},
	// No COMMANDS_DIR: Amp documents no file surface for commands, so
	// the adapter skips that kind with a warning (#553).
	"amp": {
		VarSkillsDir: ".agents/skills", VarMCPFile: ".amp/settings.json",
	},
	"zed": {
		VarSkillsDir: ".agents/skills", VarMCPFile: ".zed/settings.json",
	},
	"warp": {
		VarSkillsDir: ".agents/skills", VarMCPFile: ".warp/.mcp.json",
	},
	"opencode": {
		VarSkillsDir: ".opencode/skills", VarAgentsDir: ".opencode/agents",
		VarCommandsDir: ".opencode/commands", VarMCPFile: "opencode.json",
	},
	"antigravity": {
		VarSkillsDir: ".agents/skills", VarAgentsDir: ".agents/agents", VarRulesDir: ".agents/rules",
		VarMCPFile: ".agents/mcp_config.json",
	},
	"junie": {
		VarSkillsDir: ".junie/skills", VarAgentsDir: ".junie/agents",
		VarCommandsDir: ".junie/commands", VarMCPFile: ".junie/mcp/mcp.json",
	},
	"kiro": {
		VarAgentsDir: ".kiro/agents", VarRulesDir: ".kiro/steering",
		VarMCPFile: ".kiro/settings/mcp.json",
	},
	"crush": {
		VarSkillsDir: ".agents/skills", VarMCPFile: "crush.json",
	},
	"trae": {
		VarSkillsDir: ".trae/skills", VarCommandsDir: ".trae/commands",
		VarRulesDir: ".trae/rules", VarMCPFile: ".trae/mcp.json",
	},
	"augment": {
		VarSkillsDir: ".agents/skills", VarAgentsDir: ".augment/agents",
		VarCommandsDir: ".augment/commands", VarRulesDir: ".augment/rules",
	},
	"qoder": {
		VarSkillsDir: ".qoder/skills", VarAgentsDir: ".qoder/agents",
		VarRulesDir: ".qoder/rules", VarMCPFile: ".qoder/settings.json",
	},
	"openhands": {
		VarSkillsDir: ".agents/skills", VarAgentsDir: ".agents/agents",
	},
	"factory": {
		VarAgentsDir: ".factory/droids", VarCommandsDir: ".factory/commands",
		VarMCPFile: ".factory/mcp.json",
	},
	"kilo": {
		VarSkillsDir: ".agents/skills", VarAgentsDir: ".kilo/agents",
		VarRulesDir: ".kilo/rules", VarMCPFile: "kilo.jsonc",
	},
	// aider and jules carry every spec kind in one entry-point document
	// and have no per-kind directory to point at.
	"aider": {},
	"jules": {},
	"goose": {VarAgentsDir: ".agents/agents"},
}

// dirRelativeVars maps each *_DIR variable to its sub-directory under
// the target's output dir, for the targets that honor
// `outputs.<target>.dir`. Claude is the only one today: its directories
// move with a bare `dir` override, so a spec body that names
// `{{rules_dir}}` must name the moved path, not the default (#849).
var dirRelativeVars = map[string]map[string]string{
	"claude": {
		VarSkillsDir: "skills", VarAgentsDir: "agents",
		VarCommandsDir: "commands", VarRulesDir: "rules",
	},
}

// rulesDirIsInstructionsDir lists targets whose native name for the
// rules directory is `instructions-dir`. Copilot writes
// `*.instructions.md` and reads `outputs.copilot.instructions-dir`, so
// `{{rules_dir}}` must resolve from that key; `rules-dir` would expand
// to a directory the sync never writes to.
var rulesDirIsInstructionsDir = map[string]bool{"copilot": true}

// VarsFor resolves the variable table for target, letting an
// outputs.<target>.<field> override win over the declared default so a
// spec body and the emitted tree never disagree about where files land.
func VarsFor(cfg *config.Config, target string) map[string]string {
	declared := TargetVarPaths[target]
	if len(declared) == 0 {
		return nil
	}
	out := make(map[string]string, len(declared))
	subs := dirRelativeVars[target]
	for name, fallback := range declared {
		if sub, ok := subs[name]; ok {
			fallback = OutputSubDir(cfg, target, sub, fallback)
		}
		switch name {
		case VarSkillsDir:
			out[name] = OutputSkillsDir(cfg, target, fallback)
		case VarAgentsDir:
			out[name] = OutputAgentsDir(cfg, target, fallback)
		case VarCommandsDir:
			out[name] = OutputCommandsDir(cfg, target, fallback)
		case VarRulesDir:
			if rulesDirIsInstructionsDir[target] {
				out[name] = OutputInstructionsDir(cfg, target, fallback)
				break
			}
			out[name] = OutputRulesDir(cfg, target, fallback)
		case VarMCPFile:
			out[name] = OutputMCPFile(cfg, target, fallback)
		}
	}
	return out
}

// EntryPointReaders returns, sorted, target and every configured target
// that reads the same entry-point file. Mirrors the reader grouping in
// internal/cli/entrypoint.go.
func EntryPointReaders(cfg *config.Config, target string) []string {
	readers := []string{target}
	path := EntryPointPath(cfg, target)
	if cfg == nil || path == "" {
		return readers
	}
	path = filepath.Clean(path)
	for _, t := range cfg.Targets {
		if !slices.Contains(readers, t) && filepath.Clean(EntryPointPath(cfg, t)) == path && !LegacyRulesFileOwnsEntryPoint(cfg, t) {
			readers = append(readers, t)
		}
	}
	slices.Sort(readers)
	return readers
}

// EntryPointVars resolves the variable table for the entry point
// target reads.
func EntryPointVars(cfg *config.Config, target string) (vals map[string]string, contested []string) {
	return SharedVars(cfg, EntryPointReaders(cfg, target))
}

// ScopeDocumentReaders returns, sorted, target and every configured
// target that reads the scope document doc (AGENTS.md, GEMINI.md) in a
// nested directory. Mirrors the readers CheckScopeReaders checks.
func ScopeDocumentReaders(cfg *config.Config, target, doc string) []string {
	readers := []string{target}
	if cfg == nil {
		return readers
	}
	for _, t := range cfg.Targets {
		reads := scopeDocument(t) == doc || doc == "AGENTS.md" && (t == "cursor" || t == "copilot" || t == "windsurf")
		if reads && !slices.Contains(readers, t) {
			readers = append(readers, t)
		}
	}
	slices.Sort(readers)
	return readers
}

// SharedVars resolves the variable table for a file several readers
// load. Every reader sees one text, so a variable expands only when
// each reader resolves it to the same path. contested lists, sorted,
// the variables some reader resolves that the readers do not agree on.
func SharedVars(cfg *config.Config, readers []string) (vals map[string]string, contested []string) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	tables := make([]map[string]string, len(readers))
	names := map[string]bool{}
	for i, t := range readers {
		tables[i] = VarsFor(cfg, t)
		for name, v := range tables[i] {
			if v != "" {
				names[name] = true
			}
		}
	}
	vals = map[string]string{}
	for name := range names {
		v := tables[0][name]
		agreed := true
		for _, table := range tables[1:] {
			agreed = agreed && table[name] == v
		}
		if agreed {
			vals[name] = v
		} else {
			contested = append(contested, name)
		}
	}
	slices.Sort(contested)
	return vals, contested
}

// NoteSharedVars notes the variables in bodies that path keeps verbatim
// because its readers do not resolve them to one path. bodies are the
// unexpanded bodies of the specs of kind that land in path.
func NoteSharedVars(path string, readers []string, kind spec.Kind, bodies []string, vals map[string]string, contested []string) {
	if len(contested) == 0 {
		return
	}
	used := map[string]bool{}
	count := 0
	for _, body := range bodies {
		hit := false
		_, missing := ExpandVars(body, vals)
		for _, name := range missing {
			if slices.Contains(contested, name) {
				used[name] = true
				hit = true
			}
		}
		if hit {
			count++
		}
	}
	if count == 0 {
		return
	}
	tokens := make([]string, 0, len(used))
	for _, name := range contested {
		if used[name] {
			tokens = append(tokens, "{{$"+name+"}}")
		}
	}
	verb, pronoun := "keeps", "it"
	if count > 1 {
		verb = "keep"
	}
	if len(tokens) > 1 {
		pronoun = "them"
	}
	NoteProject(fmt.Sprintf("%s: %d %s %s %s verbatim, because the tools that read the file (%s) do not resolve %s to one path",
		filepath.ToSlash(path), count, pluralizeKind(kind, count), verb, strings.Join(tokens, ", "), strings.Join(readers, ", "), pronoun))
}
