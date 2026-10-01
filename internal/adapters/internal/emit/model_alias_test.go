package emit

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestResolveMeta_ResolvesAVendorModelAliasForItsTarget(t *testing.T) {
	for _, tc := range []struct {
		name   string
		meta   map[string]any
		target string
		want   any
	}{
		{name: "scalar on codex", meta: map[string]any{"model": "sol"}, target: "codex", want: "gpt-6.1-sol"},
		{name: "map entry on codex", meta: map[string]any{"model": map[string]any{"claude": "opus", "codex": "luna"}}, target: "codex", want: "gpt-6-luna"},
		{name: "map default on codex", meta: map[string]any{"model": map[string]any{"claude": "opus", "default": "astra"}}, target: "codex", want: "gpt-6-astra"},
		{name: "claude keeps its alias", meta: map[string]any{"model": map[string]any{"claude": "opus", "codex": "sol"}}, target: "claude", want: "opus"},
		{name: "no alias on another target", meta: map[string]any{"model": "sol"}, target: "gemini", want: "sol"},
		{name: "an exact id stays", meta: map[string]any{"model": "gpt-6-sol"}, target: "codex", want: "gpt-6-sol"},
		{name: "x-codex.model stays literal", meta: map[string]any{"model": "luna", "x-codex": map[string]any{"model": "sol"}}, target: "codex", want: "sol"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveMeta(tc.meta, tc.target)["model"]; got != tc.want {
				t.Errorf("model = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSettingsModel_ResolvesAVendorModelAlias(t *testing.T) {
	entries := []spec.Entry{{Name: "team", Meta: map[string]any{"model": map[string]any{"claude": "opus", "codex": "sol"}}}}
	if got := SettingsModel(entries, "codex"); got != "gpt-6.1-sol" {
		t.Errorf("SettingsModel(codex) = %q, want gpt-6.1-sol", got)
	}
	if got := SettingsModel(entries, "claude"); got != "opus" {
		t.Errorf("SettingsModel(claude) = %q, want opus", got)
	}
}

func TestModelAliasIn_NamesTheAliasATargetResolved(t *testing.T) {
	meta := map[string]any{"model": map[string]any{"claude": "opus", "codex": "sol"}}
	if alias := ModelAliasIn(meta, "codex"); alias != "sol" {
		t.Errorf("codex alias = %q, want sol", alias)
	}
	for _, target := range []string{"claude", "gemini"} {
		if alias := ModelAliasIn(meta, target); alias != "" {
			t.Errorf("%s resolved no alias, got %q", target, alias)
		}
	}
	if alias := ModelAliasIn(map[string]any{"model": "luna", "x-codex": map[string]any{"model": "sol"}}, "codex"); alias != "" {
		t.Errorf("x-codex.model is literal, got alias %q", alias)
	}
}
