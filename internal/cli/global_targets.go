package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Path prefixes used in the globalTargets table. A path carries its own
// root because several tools split surfaces across two roots: Kilo Code
// reads instructions from ~/.config/kilo/ but skills from ~/.kilo/, and
// Goose reads hints from ~/.config/goose/ but skills from ~/.agents/.
const (
	globalPathHome = "home:"
	globalPathXDG  = "xdg:"
)

// globalTarget declares one tool's user-level paths and discovery settings.
// Default paths use globalPathHome or globalPathXDG.
//
// An empty field means sync --global has no supported user-level surface
// of that kind.
type globalTarget struct {
	// instructions is the always-on context file the tool loads with no
	// wiring. Global rules inline into it.
	instructions string
	// rules is a per-rule directory, used only for a target whose
	// vendor documents no user-level instructions file at all.
	rules string
	// skills is the skills directory.
	skills string
	// agents is the native agent directory.
	agents string
	// rootEnv names the vendor's configuration root variable. When set,
	// its value replaces root for every surface under root, so all of a
	// target's files move together.
	rootEnv string
	// root is the default location rootEnv replaces.
	root string
	// rootSubdir is appended to the rootEnv value, for a vendor whose
	// variable names the parent of its configuration directory.
	rootSubdir string
	// agentEfforts is the user settings file holding per-agent effort
	// for a target whose agent files have no effort key.
	agentEfforts string
	// agentsWindows is relative to APPDATA when the vendor uses it on Windows.
	agentsWindows string
	// hooks is the hooks file.
	hooks string
	// hooksFormat selects the native hooks schema: "claude" or "cursor".
	hooksFormat string
	// bridge marks a target that does not auto-load its user-level
	// instructions file, so the body is injected through a managed
	// session-start hook instead.
	bridge bool
	// bridgeEvent is the session-start event name in the target's own
	// hook vocabulary.
	bridgeEvent string
	// bridgeKey is the JSON field the target reads injected context
	// from in that hook's stdout.
	bridgeKey string
}

// globalTargets maps target name to its user-level surfaces, as
// documented by each vendor (agents verified 2026-09-22). Only targets
// listed here are accepted by sync --global.
//
// Absent by verdict: aider (a home instructions file reaches it only
// through a `read:` entry in ~/.aider.conf.yml, never automatically),
// continue (its one home surface is the `rules:` list inside the
// config.yaml Continue itself rewrites), and jules (nothing documented
// at user scope at all).
var globalTargets = map[string]globalTarget{
	"claude": {
		rootEnv:      "CLAUDE_CONFIG_DIR",
		root:         globalPathHome + ".claude",
		instructions: globalPathHome + ".claude/CLAUDE.md",
		agents:       globalPathHome + ".claude/agents",
		skills:       globalPathHome + ".claude/skills",
		hooks:        globalPathHome + ".claude/settings.json",
		hooksFormat:  "claude",
	},
	"cursor": {
		agents:       globalPathHome + ".cursor/agents",
		instructions: globalPathHome + ".cursor/AGENTS.md",
		skills:       globalPathHome + ".cursor/skills",
		hooks:        globalPathHome + ".cursor/hooks.json",
		hooksFormat:  "cursor",
		bridge:       true,
		bridgeEvent:  "sessionStart",
		bridgeKey:    "additional_context",
	},
	"codex": {
		rootEnv:      "CODEX_HOME",
		root:         globalPathHome + ".codex",
		agents:       globalPathHome + ".codex/agents",
		instructions: globalPathHome + ".codex/AGENTS.md",
		skills:       globalPathHome + ".agents/skills",
		hooks:        globalPathHome + ".codex/hooks.json",
		hooksFormat:  "claude",
	},
	"gemini": {
		rootEnv:      "GEMINI_CLI_HOME",
		root:         globalPathHome + ".gemini",
		rootSubdir:   ".gemini",
		agents:       globalPathHome + ".gemini/agents",
		instructions: globalPathHome + ".gemini/GEMINI.md",
		skills:       globalPathHome + ".gemini/skills",
		hooks:        globalPathHome + ".gemini/settings.json",
		hooksFormat:  "claude",
	},
	"qoder": {
		rootEnv:      "QODER_CONFIG_DIR",
		root:         globalPathHome + ".qoder",
		agents:       globalPathHome + ".qoder/agents",
		instructions: globalPathHome + ".qoder/AGENTS.md",
		skills:       globalPathHome + ".qoder/skills",
		hooks:        globalPathHome + ".qoder/settings.json",
		hooksFormat:  "claude",
	},
	"copilot": {
		rootEnv:      "COPILOT_HOME",
		root:         globalPathHome + ".copilot",
		agents:       globalPathHome + ".copilot/agents",
		agentEfforts: globalPathHome + ".copilot/settings.json",
		instructions: globalPathHome + ".copilot/copilot-instructions.md",
		skills:       globalPathHome + ".copilot/skills",
	},
	"cline": {
		rootEnv:      "CLINE_DIR",
		root:         globalPathHome + ".cline",
		agents:       globalPathHome + ".cline/agents",
		instructions: globalPathHome + ".agents/AGENTS.md",
		skills:       globalPathHome + ".cline/skills",
	},
	"windsurf": {
		agents:        globalPathHome + ".config/devin/agents",
		agentsWindows: "devin/agents",
		instructions:  globalPathXDG + "devin/AGENTS.md",
		skills:        globalPathHome + ".agents/skills",
	},
	"amp": {
		instructions: globalPathXDG + "amp/AGENTS.md",
		skills:       globalPathHome + ".agents/skills",
	},
	"zed": {
		instructions: globalPathXDG + "zed/AGENTS.md",
		skills:       globalPathHome + ".agents/skills",
	},
	"warp": {
		instructions: globalPathHome + ".agents/AGENTS.md",
		skills:       globalPathHome + ".agents/skills",
	},
	"opencode": {
		agents:       globalPathXDG + "opencode/agents",
		instructions: globalPathXDG + "opencode/AGENTS.md",
		skills:       globalPathXDG + "opencode/skills",
	},
	"antigravity": {
		agents:       globalPathHome + ".gemini/config/agents",
		instructions: globalPathHome + ".gemini/GEMINI.md",
		// antigravity.google/docs/skills?tab=ide rows the global scope
		// as "`~/.gemini/config/skills/<skill-folder>/` | Global (all
		// workspaces; legacy `~/.gemini/antigravity/skills/` is also
		// supported)", and the Antigravity 2.0 tab names the config
		// path with no legacy alternative at all. The legacy tree still
		// loads in the IDE, so nothing breaks there, but Antigravity
		// 2.0 on the same machine reads only the config path
		// (target-audit 2026-09-19, #896).
		skills: globalPathHome + ".gemini/config/skills",
	},
	"junie": {
		rootEnv:      "JUNIE_HOME",
		root:         globalPathHome + ".junie",
		agents:       globalPathHome + ".junie/agents",
		instructions: globalPathHome + ".junie/AGENTS.md",
		skills:       globalPathHome + ".junie/skills",
	},
	"kiro": {
		rootEnv:      "KIRO_HOME",
		root:         globalPathHome + ".kiro",
		agents:       globalPathHome + ".kiro/agents",
		instructions: globalPathHome + ".kiro/steering/AGENTS.md",
		skills:       globalPathHome + ".kiro/skills",
	},
	"crush": {
		instructions: globalPathXDG + "crush/CRUSH.md",
		skills:       globalPathXDG + "crush/skills",
	},
	"factory": {
		agents:       globalPathHome + ".factory/droids",
		instructions: globalPathHome + ".factory/AGENTS.md",
		skills:       globalPathHome + ".factory/skills",
	},
	"kilo": {
		agents:       globalPathXDG + "kilo/agents",
		instructions: globalPathXDG + "kilo/AGENTS.md",
		skills:       globalPathHome + ".kilo/skills",
	},
	"goose": {
		agents:       globalPathHome + ".agents/agents",
		instructions: globalPathXDG + "goose/.goosehints",
		skills:       globalPathHome + ".agents/skills",
	},
	"openhands": {
		agents: globalPathHome + ".agents/agents",
		skills: globalPathHome + ".agents/skills",
	},
	"trae": {
		agents: globalPathHome + ".trae-cn/agents",
		skills: globalPathHome + ".trae/skills",
	},
	"augment": {
		agents: globalPathHome + ".augment/agents",
		rules:  globalPathHome + ".augment/rules",
		skills: globalPathHome + ".augment/skills",
	},
}

// globalTargetNames returns every target sync --global supports, sorted.
func globalTargetNames() []string {
	out := make([]string, 0, len(globalTargets))
	for name := range globalTargets {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// globalPath resolves one table path against the user home.
func globalPath(home, p string) string {
	if rest, ok := strings.CutPrefix(p, globalPathXDG); ok {
		root := os.Getenv("XDG_CONFIG_HOME")
		if root == "" {
			root = filepath.Join(home, ".config")
		}
		return filepath.Join(root, filepath.FromSlash(rest))
	}
	return filepath.Join(home, filepath.FromSlash(strings.TrimPrefix(p, globalPathHome)))
}

// trees returns the target's managed directory surfaces, resolved.
// Recorded files under these trees are swept when their source goes away.
func (g globalTarget) trees(home string) []string {
	var out []string
	for _, p := range []string{g.skills, g.rules} {
		if p != "" {
			out = append(out, g.path(home, p))
		}
	}
	if dir := g.agentsPath(home); dir != "" {
		out = append(out, dir)
	}
	return out
}

func (g globalTarget) agentsPath(home string) string {
	if g.agents == "" {
		return ""
	}
	if runtime.GOOS == "windows" && g.agentsWindows != "" {
		root := os.Getenv("APPDATA")
		if root == "" {
			root = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(root, filepath.FromSlash(g.agentsWindows))
	}
	return g.path(home, g.agents)
}

// path resolves one of the target's table paths, honoring rootEnv.
func (g globalTarget) path(home, p string) string {
	if g.rootEnv != "" {
		if root := os.Getenv(g.rootEnv); root != "" {
			if rest, ok := strings.CutPrefix(p, g.root); ok && (rest == "" || rest[0] == '/') {
				return filepath.Join(root, filepath.FromSlash(g.rootSubdir), filepath.FromSlash(rest))
			}
		}
	}
	return globalPath(home, p)
}

// rootError reports a relative rootEnv value, which would make ownership
// depend on the working directory.
func (g globalTarget) rootError(target string) error {
	if g.rootEnv == "" {
		return nil
	}
	if root := os.Getenv(g.rootEnv); root != "" && !filepath.IsAbs(root) {
		return fmt.Errorf("%s: %s=%q must be an absolute path", target, g.rootEnv, root)
	}
	return nil
}

// files returns the target's single-file surfaces, resolved, including
// the bridge script when it has one. Ownership of these is by exact
// path, since each sits in a directory the tool also uses for its own
// unmanaged configuration.
func (g globalTarget) files(home string) []string {
	var out []string
	for _, p := range []string{g.instructions, g.hooks} {
		if p != "" {
			out = append(out, g.path(home, p))
		}
	}
	if g.bridge {
		out = append(out, globalBridgePath(filepath.Dir(g.path(home, g.hooks))))
	}
	return out
}
