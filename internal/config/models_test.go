package config_test

import (
	"encoding/json"
	"maps"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func TestLoad_ReadsModelTiers(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `version: 1
targets: [claude, codex]
models:
  strong: {claude: opus, codex: gpt-5.5, default: gpt-5.5, effort: {claude: xhigh, codex: high}}
  fast: {claude: haiku, effort: low}
`)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	strong := cfg.Models["strong"]
	if want := map[string]string{"claude": "opus", "codex": "gpt-5.5", "default": "gpt-5.5"}; !maps.Equal(strong.Models, want) {
		t.Errorf("strong models = %v, want %v", strong.Models, want)
	}
	if effort, _ := strong.Effort.(map[string]any); effort["claude"] != "xhigh" || effort["codex"] != "high" {
		t.Errorf("strong effort = %#v", strong.Effort)
	}
	if fast := cfg.Models["fast"]; fast.Effort != "low" || fast.Models["claude"] != "haiku" {
		t.Errorf("fast = %#v", fast)
	}
}

func TestLoad_RejectsMalformedModelTiers(t *testing.T) {
	cases := map[string]string{
		"empty model":    "models:\n  strong: {claude: \"\"}\n",
		"list effort":    "models:\n  strong: {claude: opus, effort: [high]}\n",
		"nested effort":  "models:\n  strong: {claude: opus, effort: {claude: {level: high}}}\n",
		"empty tier key": "models:\n  \"\": {claude: opus}\n",
		"null tier":      "models:\n  strong:\n",
		"float effort":   "models:\n  strong: {claude: opus, effort: 1.5}\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeConfig(t, dir, "version: 1\ntargets: [claude]\n"+body)
			if _, err := config.Load(dir); err == nil || !strings.Contains(err.Error(), "models") {
				t.Fatalf("Load() error = %v, want a models error", err)
			}
		})
	}
}

func TestModelTier_MarshalsFlat(t *testing.T) {
	tier := config.ModelTier{Models: map[string]string{"claude": "opus"}, Effort: "high"}
	data, err := json.Marshal(tier)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != `{"claude":"opus","effort":"high"}` {
		t.Errorf("json = %s", got)
	}
}
