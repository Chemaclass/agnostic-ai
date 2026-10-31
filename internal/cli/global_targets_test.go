package cli

import (
	"path/filepath"
	"slices"
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
		surfaces := map[string]string{"instructions": g.instructions, "rules": g.rules, "skills": g.skills, "hooks": g.hooks, "agents": g.agents}
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
		adapter, ok := adapters.Get(name)
		if _, native := adapter.(adapters.AgentEmitter); ok && (g.agents != "") != native {
			t.Errorf("%s: global agent directory and native emitter must agree", name)
		}
		if (g.rootEnv == "") != (g.root == "") {
			t.Errorf("%s: root override needs an environment variable and default root", name)
		}
		if g.instructions != "" && g.rules != "" {
			t.Errorf("%s: rules dir is only for a target with no instructions file", name)
		}
		if g.hooks != "" && g.hooksFormat != "claude" && g.hooksFormat != "cursor" && g.hooksFormat != "augment" && g.hooksFormat != "copilot" && g.hooksFormat != "vibe-toml" && g.hooksFormat != "kimi-toml" {
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

func TestGlobalPaths_IgnoreRelativePlatformRoots(t *testing.T) {
	home := t.TempDir()
	for _, tc := range []struct{ env, prefix, fallback string }{
		{"XDG_CONFIG_HOME", globalPathXDG, ".config"},
		{"APPDATA", globalPathAppData, "AppData/Roaming"},
		{"LOCALAPPDATA", globalPathLocalAppData, "AppData/Local"},
	} {
		t.Run(tc.env, func(t *testing.T) {
			t.Setenv(tc.env, "relative")
			if got, want := globalPath(home, tc.prefix+"tool/file"), filepath.Join(home, tc.fallback, "tool", "file"); got != want {
				t.Errorf("path = %q, want %q", got, want)
			}
			if slices.Contains(globalRoots(home), "relative") {
				t.Error("relative root allows state paths in the working directory")
			}
		})
	}
}

func TestGlobalPaths_RelativeXDGFallsBackForTargets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "relative")
	for _, name := range []string{"opencode", "crush", "amp", "kilo", "goose", "devin"} {
		g := globalTargets[name]
		for _, p := range []string{g.instructions, g.skills, g.agents, g.rules, g.hooks, g.hooksDir, g.hookScriptsDir} {
			if strings.HasPrefix(p, globalPathXDG) {
				if got := g.path(home, p); !strings.HasPrefix(got, filepath.Join(home, ".config")+string(filepath.Separator)) {
					t.Errorf("%s path %q resolved to %q", name, p, got)
				}
			}
		}
	}
}

func TestForeignGlobalPath_RejectsRelativeRecordedPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", ".")
	t.Setenv("GROK_HOME", ".")
	if slices.Contains(globalRoots(home), ".") {
		t.Error("relative target override entered ownership roots")
	}
	path := filepath.Join("relative", "owned.md")
	for name, state := range map[string]globalState{
		"file":     {Files: []string{path}},
		"created":  {Files: []string{path}, Created: []string{path}},
		"hook":     {HookFiles: map[string][]string{"target": {path}}},
		"settings": {SettingsPaths: map[string]string{"target": path}},
	} {
		if got := foreignGlobalPath(state, home); got != path {
			t.Errorf("%s foreign path = %q, want %q", name, got, path)
		}
	}
}

func TestGlobalPath_ResolvesBothRoots(t *testing.T) {
	home, cfg := t.TempDir(), t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	if got, want := globalPath(home, globalPathHome+".claude/CLAUDE.md"), filepath.Join(home, ".claude", "CLAUDE.md"); got != want {
		t.Errorf("home path: got %q want %q", got, want)
	}
	if got, want := globalPath(home, globalPathXDG+"zed/AGENTS.md"), filepath.Join(home, ".config", "zed", "AGENTS.md"); got != want {
		t.Errorf("xdg fallback: got %q want %q", got, want)
	}
	t.Setenv("XDG_CONFIG_HOME", cfg)
	if got, want := globalPath(home, globalPathXDG+"zed/AGENTS.md"), filepath.Join(cfg, "zed", "AGENTS.md"); got != want {
		t.Errorf("xdg override: got %q want %q", got, want)
	}
}
