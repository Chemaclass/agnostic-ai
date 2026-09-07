package cli

import (
	"os"
	"path/filepath"
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

// globalTarget describes one tool's user-level surfaces. Every field is
// a table path (see globalPathHome / globalPathXDG).
//
// An empty field means the vendor documents no user-level surface of
// that kind, so sync --global emits nothing for it rather than guessing
// a path.
type globalTarget struct {
	// instructions is the always-on context file the tool loads with no
	// wiring. Global rules inline into it.
	instructions string
	// rules is a per-rule directory, used only for a target whose
	// vendor documents no user-level instructions file at all.
	rules string
	// skills is the skills directory.
	skills string
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
// documented by each vendor (target-audit 2026-09-07). Only targets
// listed here are accepted by sync --global.
//
// Absent by verdict: aider (a home instructions file reaches it only
// through a `read:` entry in ~/.aider.conf.yml, never automatically),
// continue (its one home surface is the `rules:` list inside the
// config.yaml Continue itself rewrites), and jules (nothing documented
// at user scope at all).
var globalTargets = map[string]globalTarget{
	"claude": {
		instructions: globalPathHome + ".claude/CLAUDE.md",
		skills:       globalPathHome + ".claude/skills",
		hooks:        globalPathHome + ".claude/settings.json",
		hooksFormat:  "claude",
	},
	"cursor": {
		instructions: globalPathHome + ".cursor/AGENTS.md",
		skills:       globalPathHome + ".cursor/skills",
		hooks:        globalPathHome + ".cursor/hooks.json",
		hooksFormat:  "cursor",
		bridge:       true,
		bridgeEvent:  "sessionStart",
		bridgeKey:    "additional_context",
	},
	"codex": {
		instructions: globalPathHome + ".codex/AGENTS.md",
		skills:       globalPathHome + ".agents/skills",
		hooks:        globalPathHome + ".codex/hooks.json",
		hooksFormat:  "claude",
	},
	"gemini": {
		instructions: globalPathHome + ".gemini/GEMINI.md",
		skills:       globalPathHome + ".gemini/skills",
		hooks:        globalPathHome + ".gemini/settings.json",
		hooksFormat:  "claude",
	},
	"qoder": {
		instructions: globalPathHome + ".qoder/AGENTS.md",
		skills:       globalPathHome + ".qoder/skills",
		hooks:        globalPathHome + ".qoder/settings.json",
		hooksFormat:  "claude",
	},
	"copilot": {
		instructions: globalPathHome + ".copilot/copilot-instructions.md",
		skills:       globalPathHome + ".copilot/skills",
	},
	"cline": {
		instructions: globalPathHome + ".agents/AGENTS.md",
		skills:       globalPathHome + ".cline/skills",
	},
	"windsurf": {
		instructions: globalPathXDG + "devin/AGENTS.md",
		skills:       globalPathHome + ".agents/skills",
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
		instructions: globalPathXDG + "opencode/AGENTS.md",
		skills:       globalPathXDG + "opencode/skills",
	},
	"antigravity": {
		instructions: globalPathHome + ".gemini/GEMINI.md",
		skills:       globalPathHome + ".gemini/antigravity/skills",
	},
	"junie": {
		instructions: globalPathHome + ".junie/AGENTS.md",
		skills:       globalPathHome + ".junie/skills",
	},
	"kiro": {
		instructions: globalPathHome + ".kiro/steering/AGENTS.md",
		skills:       globalPathHome + ".kiro/skills",
	},
	"crush": {
		instructions: globalPathXDG + "crush/CRUSH.md",
		skills:       globalPathXDG + "crush/skills",
	},
	"factory": {
		instructions: globalPathHome + ".factory/AGENTS.md",
		skills:       globalPathHome + ".factory/skills",
	},
	"kilo": {
		instructions: globalPathXDG + "kilo/AGENTS.md",
		skills:       globalPathHome + ".kilo/skills",
	},
	"goose": {
		instructions: globalPathXDG + "goose/.goosehints",
		skills:       globalPathHome + ".agents/skills",
	},
	"openhands": {
		skills: globalPathHome + ".agents/skills",
	},
	"trae": {
		skills: globalPathHome + ".trae/skills",
	},
	"augment": {
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
// Everything under one is owned by sync --global and swept when the
// source spec goes away.
func (g globalTarget) trees(home string) []string {
	var out []string
	for _, p := range []string{g.skills, g.rules} {
		if p != "" {
			out = append(out, globalPath(home, p))
		}
	}
	return out
}

// files returns the target's single-file surfaces, resolved, including
// the bridge script when it has one. Ownership of these is by exact
// path, since each sits in a directory the tool also uses for its own
// unmanaged configuration.
func (g globalTarget) files(home string) []string {
	var out []string
	for _, p := range []string{g.instructions, g.hooks} {
		if p != "" {
			out = append(out, globalPath(home, p))
		}
	}
	if g.bridge {
		out = append(out, globalBridgePath(filepath.Dir(globalPath(home, g.hooks))))
	}
	return out
}
