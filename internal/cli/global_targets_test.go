package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

func TestGlobalTargets_TableInvariants(t *testing.T) {
	known := map[string]bool{}
	for _, name := range adapters.Names() {
		known[name] = true
	}
	for name, g := range globalTargets {
		if !known[name] {
			t.Errorf("%s: global target is not a known adapter", name)
		}
		surfaces := map[string]string{"instructions": g.instructions, "rules": g.rules, "skills": g.skills, "hooks": g.hooks}
		var declared int
		for kind, path := range surfaces {
			if path == "" {
				continue
			}
			declared++
			if !strings.HasPrefix(path, globalPathHome) && !strings.HasPrefix(path, globalPathXDG) {
				t.Errorf("%s: %s path %q has no root prefix", name, kind, path)
			}
			if strings.Contains(path, "\\") {
				t.Errorf("%s: %s path %q must use forward slashes", name, kind, path)
			}
		}
		if declared == 0 {
			t.Errorf("%s: listed with no user-level surface at all", name)
		}
		if g.instructions != "" && g.rules != "" {
			t.Errorf("%s: rules dir is only for a target with no instructions file", name)
		}
		if g.hooks != "" && g.hooksFormat != "claude" && g.hooksFormat != "cursor" {
			t.Errorf("%s: hooks file %q has no native schema", name, g.hooks)
		}
		if g.hooksFormat != "" && g.hooks == "" {
			t.Errorf("%s: hooks schema without a hooks file", name)
		}
		if !g.bridge {
			if g.bridgeEvent != "" || g.bridgeKey != "" {
				t.Errorf("%s: bridge fields set without bridge", name)
			}
			continue
		}
		if g.instructions == "" {
			t.Errorf("%s: bridge with no instructions file to inject", name)
		}
		if g.hooks == "" || g.bridgeEvent == "" || g.bridgeKey == "" {
			t.Errorf("%s: bridge needs a hooks file, a session-start event, and a context key", name)
		}
	}
}

func TestGlobalPath_ResolvesBothRoots(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	if got, want := globalPath("/h", globalPathHome+".claude/CLAUDE.md"), "/h/.claude/CLAUDE.md"; got != want {
		t.Errorf("home path: got %q want %q", got, want)
	}
	if got, want := globalPath("/h", globalPathXDG+"zed/AGENTS.md"), "/h/.config/zed/AGENTS.md"; got != want {
		t.Errorf("xdg fallback: got %q want %q", got, want)
	}
	t.Setenv("XDG_CONFIG_HOME", "/cfg")
	if got, want := globalPath("/h", globalPathXDG+"zed/AGENTS.md"), "/cfg/zed/AGENTS.md"; got != want {
		t.Errorf("xdg override: got %q want %q", got, want)
	}
}
