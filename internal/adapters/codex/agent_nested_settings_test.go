package codex

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		Skills   struct {
			Config []struct {
				Path    string
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

func decodeAgentTOML(t *testing.T, x map[string]any) (map[string]any, string) {
	t.Helper()
	entry := spec.Entry{Kind: spec.KindAgent, Name: "limited", Body: "Review changes.", Meta: map[string]any{"x-codex": x}}
	body := agentTOML(entry)
	var got map[string]any
	if _, err := toml.Decode(body, &got); err != nil {
		t.Fatalf("agent TOML is invalid: %v\n%s", err, body)
	}
	return got, body
}

func TestAgentTOML_UnusualValuesKeepSiblingSwitches(t *testing.T) {
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got, body := decodeAgentTOML(t, map[string]any{
		"features": map[string]any{
			"shell_tool": false,
			"levels":     []any{1, 2},
			"mixed":      []any{"a", 1},
			"since":      when,
			"nan":        math.NaN(),
		},
		"skills": map[string]any{"config": []any{
			map[string]any{"path": "a", "enabled": false, "extra": map[string]any{"deep": true}},
		}},
	})
	features, _ := got["features"].(map[string]any)
	if v, ok := features["shell_tool"]; !ok || v != false {
		t.Errorf("shell_tool = false must survive unsupported siblings: %#v\n%s", features, body)
	}
	if _, ok := features["nan"]; ok {
		t.Errorf("a non-finite float must be skipped: %s", body)
	}
	if levels, _ := features["levels"].([]any); len(levels) != 2 {
		t.Errorf("integer array lost: %#v", features["levels"])
	}
	if _, ok := features["since"]; ok {
		t.Errorf("a datetime is timezone-dependent and must be skipped: %s", body)
	}
}

func TestAgentTOML_KeysAreQuotedWhenNeeded(t *testing.T) {
	got, body := decodeAgentTOML(t, map[string]any{
		"features":     map[string]any{"my key": false, "a.b": true},
		"my top":       "v",
		"skills":       map[string]any{"config": []any{map[string]any{"path": "p"}}, "my sub": map[string]any{"x": map[string]any{"y": true}}},
		"dotted.table": map[string]any{"k": map[string]any{"z": true}},
	})
	features, _ := got["features"].(map[string]any)
	if features["my key"] != false || features["a.b"] != true || len(features) != 2 {
		t.Errorf("feature keys restructured: %#v\n%s", features, body)
	}
	if got["my top"] != "v" {
		t.Errorf("top-level key lost: %s", body)
	}
	if _, ok := got["dotted.table"].(map[string]any); !ok {
		t.Errorf("dotted table key restructured: %s", body)
	}
}

func TestAgentTOML_WholeFloatsStayFloats(t *testing.T) {
	got, body := decodeAgentTOML(t, map[string]any{
		"ratio":    2.0,
		"features": map[string]any{"scale": 3.0, "off": false},
		"inf":      math.Inf(1),
	})
	if v, ok := got["ratio"].(float64); !ok || v != 2 {
		t.Errorf("ratio = %#v, want float 2.0\n%s", got["ratio"], body)
	}
	features, _ := got["features"].(map[string]any)
	if _, ok := features["scale"].(float64); !ok {
		t.Errorf("scale lost its float type: %#v", features["scale"])
	}
	if _, ok := got["inf"]; ok {
		t.Errorf("infinity must be skipped: %s", body)
	}
}

func TestAgentTOML_ControlCharactersEscape(t *testing.T) {
	want := "a\tb\nc\rd\x01e\x7f\"\\"
	got, body := decodeAgentTOML(t, map[string]any{
		"note":   want,
		"skills": map[string]any{"config": []any{map[string]any{"path": want, "enabled": true}}},
	})
	if got["note"] != want {
		t.Errorf("note = %q, want %q\n%s", got["note"], want, body)
	}
	skills, _ := got["skills"].(map[string]any)
	config, _ := skills["config"].([]map[string]any)
	if len(config) != 1 || config[0]["path"] != want {
		t.Errorf("skill path changed: %#v\n%s", skills, body)
	}
}

func TestAgentTOML_ReportsSkippedValues(t *testing.T) {
	entry := spec.Entry{Kind: spec.KindAgent, Name: "limited", Meta: map[string]any{"x-codex": map[string]any{
		"features": map[string]any{"shell_tool": false, "bad": math.Inf(1)},
		"ratio":    math.NaN(),
		"cleared":  nil,
	}}}
	body, skips := agentTOMLWithSkips(entry)
	if !strings.Contains(body, "shell_tool = false") {
		t.Errorf("shell_tool lost: %s", body)
	}
	if got := strings.Join(skips, ","); got != "features.bad,ratio" {
		t.Errorf("skips = %q, want features.bad,ratio", got)
	}
}

func TestAgentTOML_FullyWrittenValuesAreNotReported(t *testing.T) {
	entry := spec.Entry{Kind: spec.KindAgent, Name: "limited", Meta: map[string]any{"x-codex": map[string]any{
		"features": map[string]any{"shell_tool": false},
		"empty":    map[string]any{},
		"cleared":  map[string]any{"gone": nil},
	}}}
	body, skips := agentTOMLWithSkips(entry)
	if len(skips) != 0 {
		t.Errorf("skips = %v, want none", skips)
	}
	if !strings.Contains(body, "features = { shell_tool = false }") {
		t.Errorf("features lost: %s", body)
	}
	if strings.Contains(body, "empty") || strings.Contains(body, "cleared") {
		t.Errorf("empty or null-only maps must write nothing: %s", body)
	}
}

func TestAgentTOML_SkipPathsUseRawKeys(t *testing.T) {
	entry := spec.Entry{Kind: spec.KindAgent, Name: "limited", Meta: map[string]any{"x-codex": map[string]any{
		"features": map[string]any{"my key": math.NaN()},
		"skills":   map[string]any{"config": []any{map[string]any{"path": "a", "bad": math.Inf(1)}}, "my sub": map[string]any{"x": math.NaN()}},
	}}}
	_, skips := agentTOMLWithSkips(entry)
	if got := strings.Join(skips, ","); got != "features.my key,skills.my sub.x,skills.config.bad" {
		t.Errorf("skips = %q", got)
	}
}

func TestAgentTOML_DatetimesAreReported(t *testing.T) {
	entry := spec.Entry{Kind: spec.KindAgent, Name: "limited", Meta: map[string]any{"x-codex": map[string]any{
		"features": map[string]any{"shell_tool": false, "since": time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
	}}}
	body, skips := agentTOMLWithSkips(entry)
	if !strings.Contains(body, "shell_tool = false") || strings.Contains(body, "since") {
		t.Errorf("unexpected body: %s", body)
	}
	if got := strings.Join(skips, ","); got != "features.since" {
		t.Errorf("skips = %q, want features.since", got)
	}
}

func TestAgentTOML_BadLeafInNestedMapDropsOnlyThatLeaf(t *testing.T) {
	entry := spec.Entry{Kind: spec.KindAgent, Name: "limited", Meta: map[string]any{"x-codex": map[string]any{
		"skills": map[string]any{"config": []any{map[string]any{
			"path":  "a",
			"extra": map[string]any{"p": map[string]any{"q": 1, "r": math.NaN()}},
		}}},
	}}}
	body, skips := agentTOMLWithSkips(entry)
	if !strings.Contains(body, "extra = { p = { q = 1 } }") {
		t.Errorf("sibling leaf q lost: %s", body)
	}
	if got := strings.Join(skips, ","); got != "skills.config.extra.p.r" {
		t.Errorf("skips = %q, want skills.config.extra.p.r", got)
	}
}
