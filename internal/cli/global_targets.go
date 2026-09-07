package cli

import (
	"os"
	"path/filepath"
	"sort"
)

// globalTarget describes where one tool keeps its user-level
// configuration. Every path is relative to the target's own config
// directory, which is itself resolved against the user home (or
// XDG_CONFIG_HOME when xdg is set).
//
// A field left empty means the vendor documents no user-level surface
// of that kind; sync --global then emits nothing for it rather than
// guessing a path.
type globalTarget struct {
	// dir is the config directory relative to the user home. Ignored
	// when xdg is set.
	dir string
	// xdg is the config directory relative to XDG_CONFIG_HOME
	// (default ~/.config) for tools that follow the base-dir spec.
	xdg string
	// instructions is the always-on context file inside the config dir.
	instructions string
	// skills is the skills directory inside the config dir.
	skills string
	// hooks is the hooks file inside the config dir.
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

// globalTargets maps target name to its user-level surfaces. Only
// targets listed here are accepted by sync --global.
var globalTargets = map[string]globalTarget{
	"claude": {dir: ".claude", instructions: "CLAUDE.md", skills: "skills", hooks: "settings.json", hooksFormat: "claude"},
	"cursor": {dir: ".cursor", instructions: "AGENTS.md", skills: "skills", hooks: "hooks.json", hooksFormat: "cursor", bridge: true, bridgeEvent: "sessionStart", bridgeKey: "additional_context"},
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

// base returns the absolute config directory for the target.
func (g globalTarget) base(home string) string {
	if g.xdg != "" {
		root := os.Getenv("XDG_CONFIG_HOME")
		if root == "" {
			root = filepath.Join(home, ".config")
		}
		return filepath.Join(root, g.xdg)
	}
	return filepath.Join(home, g.dir)
}
