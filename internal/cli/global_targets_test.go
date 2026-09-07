package cli

import (
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
		if g.dir == "" && g.xdg == "" {
			t.Errorf("%s: no config directory", name)
		}
		if g.dir != "" && g.xdg != "" {
			t.Errorf("%s: dir and xdg are mutually exclusive", name)
		}
		if g.instructions == "" && g.skills == "" && g.hooks == "" {
			t.Errorf("%s: listed with no user-level surface at all", name)
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
