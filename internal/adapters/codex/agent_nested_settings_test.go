package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_AgentPreservesFeatureAndSkillSettings(t *testing.T) {
	dir := testutil.TempCwd(t)
	entry := spec.Entry{Kind: spec.KindAgent, Name: "limited", Body: "Review changes.", Meta: map[string]any{
		"x-codex": map[string]any{
			"features": map[string]any{"shell_tool": false, "memories": true},
			"skills": map[string]any{"config": []any{
				map[string]any{"path": "/skills/first/SKILL.md", "enabled": false},
				map[string]any{"path": "/skills/second/SKILL.md", "enabled": true},
			}},
			"future_strings": map[string]any{"beta": "two", "alpha": "one"},
		},
	}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".codex", "agents", "limited.toml")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Features map[string]bool
		Skills struct {
			Config []struct {
				Path string
				Enabled bool
			}
		}
	}
	if _, err := toml.Decode(string(body), &doc); err != nil {
		t.Fatalf("native agent TOML is invalid: %v\n%s", err, body)
	}
	if shell, ok := doc.Features["shell_tool"]; !ok || shell {
		t.Errorf("shell_tool must remain explicitly false: %#v", doc.Features)
	}
	if !doc.Features["memories"] {
		t.Errorf("memories must remain true: %#v", doc.Features)
	}
	if len(doc.Skills.Config) != 2 {
		t.Fatalf("skill config count = %d, want 2: %s", len(doc.Skills.Config), body)
	}
	if doc.Skills.Config[0].Path != "/skills/first/SKILL.md" || doc.Skills.Config[0].Enabled || doc.Skills.Config[1].Path != "/skills/second/SKILL.md" || !doc.Skills.Config[1].Enabled {
		t.Errorf("skill path/enabled values changed: %#v", doc.Skills.Config)
	}
	if !strings.Contains(string(body), "future_strings = { alpha = \"one\", beta = \"two\" }\n") {
		t.Errorf("existing unknown string-table bytes changed: %s", body)
	}
}
