package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_WritesWholeServerWildcardForDoubleUnderscoreServer(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent,
			Name: "rep",
			Meta: map[string]any{"name": "rep", "can": []any{"mcp:github", "mcp:reports__v2"}},
			Body: "report",
		},
		{
			Kind: spec.KindSettings,
			Name: "perms",
			Path: "settings/perms.yaml",
			Meta: map[string]any{"permissions": map[string]any{"allow": []any{"mcp:reports__v2", "mcp:github"}}},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries).For("claude"), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	agent, err := os.ReadFile(filepath.Join(dir, ".claude", "agents", "rep.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agent), "mcp__github") || !strings.Contains(string(agent), "mcp__reports__v2__*") {
		t.Errorf("agent tools missing github or the wildcard in %s", agent)
	}
	if strings.Contains(string(agent), "mcp__reports__v2,") || strings.Contains(string(agent), "mcp__reports__v2]") {
		t.Errorf("agent still has the bare server rule in %s", agent)
	}
	settings, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"mcp__reports__v2__*"`, `"mcp__github"`} {
		if !strings.Contains(string(settings), want) {
			t.Errorf("settings missing %s in %s", want, settings)
		}
	}
	if strings.Contains(string(settings), `"mcp__reports__v2"`) {
		t.Errorf("settings still has the bare server rule in %s", settings)
	}
}
