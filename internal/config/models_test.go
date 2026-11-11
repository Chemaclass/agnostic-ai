package config_test

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
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

func TestLoad_LocalOverrideReplacesATierWhole(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "version: 1\ntargets: [claude, codex]\nmodels:\n  strong: {claude: sonnet, codex: gpt-5.4}\n  fast: {claude: haiku}\n")
	if err := os.WriteFile(filepath.Join(dir, config.LocalOverrideFileName), []byte("models:\n  strong: {claude: opus}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"claude": "opus"}; !maps.Equal(cfg.Models["strong"].Models, want) {
		t.Errorf("strong = %v, want %v", cfg.Models["strong"].Models, want)
	}
	if cfg.Models["fast"].Models["claude"] != "haiku" {
		t.Errorf("a tier the local file leaves alone must stay: %v", cfg.Models)
	}
}

func TestLoad_RejectsMalformedModelTiers(t *testing.T) {
	cases := map[string]string{
		"empty model":                "models:\n  strong: {claude: \"\"}\n",
		"list effort":                "models:\n  strong: {claude: opus, effort: [high]}\n",
		"nested effort":              "models:\n  strong: {claude: opus, effort: {claude: {level: high}}}\n",
		"empty tier key":             "models:\n  \"\": {claude: opus}\n",
		"null tier":                  "models:\n  strong:\n",
		"float effort":               "models:\n  strong: {claude: opus, effort: 1.5}\n",
		"multi-line description":     "models:\n  strong: {claude: opus, description: \"for\\nall\"}\n",
		"comment in description":     "models:\n  strong: {claude: opus, description: \"for <!-- x\"}\n",
		"description only":           "models:\n  strong: {description: for all}\n",
		"map description":            "models:\n  strong: {claude: opus, description: {a: b}}\n",
		"marker in described name":   "models:\n  \"<!-- agnostic-ai:global:end -->\": {claude: opus, description: for all}\n",
		"backtick in described name": "models:\n  \"str`ong\": {claude: opus, description: for all}\n",
		"backtick in description":    "models:\n  strong: {claude: opus, description: \"for `all\"}\n",
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

func TestLoad_LocalNullTierKeepsSharedTier(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "version: 1\ntargets: [claude]\nmodels:\n  fast: {claude: haiku}\n")
	if err := os.WriteFile(filepath.Join(dir, config.LocalOverrideFileName), []byte("models:\n  fast:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Models["fast"].Models["claude"] != "haiku" {
		t.Errorf("shared tier lost: %+v", cfg.Models)
	}
}

func TestLoad_ReadsModelTierDescription(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "version: 1\ntargets: [claude]\nmodels:\n  fast: {claude: haiku, description: for lookups}\n")

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	fast := cfg.Models["fast"]
	if fast.Description != "for lookups" {
		t.Errorf("description = %q, want %q", fast.Description, "for lookups")
	}
	if want := map[string]string{"claude": "haiku"}; !maps.Equal(fast.Models, want) {
		t.Errorf("models = %v, want %v", fast.Models, want)
	}
}

func TestModelTier_MarshalsDescription(t *testing.T) {
	tier := config.ModelTier{Models: map[string]string{"claude": "opus"}, Description: "for everyday coding"}

	data, err := json.Marshal(tier)
	if err != nil {
		t.Fatal(err)
	}

	if got := string(data); got != `{"claude":"opus","description":"for everyday coding"}` {
		t.Errorf("json = %s", got)
	}
}

func TestLoad_LocalTierReplacesTheDescription(t *testing.T) {
	cases := map[string]struct{ local, want string }{
		"local description wins": {"models:\n  strong: {claude: opus, description: for hard work}\n", "for hard work"},
		"local tier without one": {"models:\n  strong: {claude: opus}\n", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeConfig(t, dir, "version: 1\ntargets: [claude]\nmodels:\n  strong: {claude: sonnet, description: for everyday coding}\n")
			if err := os.WriteFile(filepath.Join(dir, config.LocalOverrideFileName), []byte(tc.local), 0o644); err != nil {
				t.Fatal(err)
			}

			cfg, err := config.Load(dir)
			if err != nil {
				t.Fatal(err)
			}

			if got := cfg.Models["strong"].Description; got != tc.want {
				t.Errorf("description = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLoad_KeepsAnUndescribedTierNameAsBefore(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "version: 1\ntargets: [claude]\nmodels:\n  \"str`ong\": {claude: opus}\n")

	if _, err := config.Load(dir); err != nil {
		t.Errorf("a tier without a description should load as before: %v", err)
	}
}

func TestLoad_NamesTheKeyOfABadTier(t *testing.T) {
	cases := map[string]string{
		"models.str`ong: tier name": "models:\n  \"str`ong\": {claude: opus, description: for all}\n",
		"models.strong.description": "models:\n  strong: {claude: opus, description: \"for `all\"}\n",
	}
	for want, body := range cases {
		t.Run(want, func(t *testing.T) {
			dir := t.TempDir()
			writeConfig(t, dir, "version: 1\ntargets: [claude]\n"+body)

			_, err := config.Load(dir)

			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("Load() error = %v, want one naming %s", err, want)
			}
		})
	}
}
