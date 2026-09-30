package spec

import (
	"reflect"
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func modelTiersFixture() map[string]config.ModelTier {
	return map[string]config.ModelTier{
		"strong": {
			Models: map[string]string{"claude": "opus", "codex": "gpt-5.5", "default": "gpt-5.5"},
			Effort: map[string]any{"claude": "xhigh", "codex": "high"},
		},
		"balanced": {Models: map[string]string{"claude": "sonnet"}},
	}
}

func TestApplyModelTiers_ExpandsATierName(t *testing.T) {
	b := Bundle{Agents: []Entry{{Kind: KindAgent, Name: "a", Meta: map[string]any{"name": "a", "model": "strong"}, MetaKeys: []string{"name", "model"}}}}
	b.ApplyModelTiers(modelTiersFixture())

	got := b.Agents[0]
	if want := map[string]any{"claude": "opus", "codex": "gpt-5.5", "default": "gpt-5.5"}; !reflect.DeepEqual(got.Meta["model"], want) {
		t.Errorf("model = %#v, want %#v", got.Meta["model"], want)
	}
	if want := map[string]any{"claude": "xhigh", "codex": "high"}; !reflect.DeepEqual(got.Meta["effort"], want) {
		t.Errorf("effort = %#v, want %#v", got.Meta["effort"], want)
	}
	if got.ModelTier != "strong" {
		t.Errorf("ModelTier = %q", got.ModelTier)
	}
	if want := []string{"name", "model", "effort"}; !slices.Equal(got.MetaKeys, want) {
		t.Errorf("MetaKeys = %v, want %v", got.MetaKeys, want)
	}
}

func TestApplyModelTiers_SpecValuesWin(t *testing.T) {
	b := Bundle{
		Skills: []Entry{{Kind: KindSkill, Name: "s", Meta: map[string]any{
			"model":  map[string]any{"codex": "gpt-5.4-mini", "default": "strong"},
			"effort": "low",
		}}},
		Settings: []Entry{{Kind: KindSettings, Name: "d", Meta: map[string]any{"model": "balanced"}}},
	}
	b.ApplyModelTiers(modelTiersFixture())

	skill := b.Skills[0]
	if want := map[string]any{"claude": "opus", "codex": "gpt-5.4-mini", "default": "gpt-5.5"}; !reflect.DeepEqual(skill.Meta["model"], want) {
		t.Errorf("skill model = %#v, want %#v", skill.Meta["model"], want)
	}
	if skill.Meta["effort"] != "low" {
		t.Errorf("spec effort must win, got %#v", skill.Meta["effort"])
	}
	settings := b.Settings[0]
	if want := map[string]any{"claude": "sonnet"}; !reflect.DeepEqual(settings.Meta["model"], want) {
		t.Errorf("settings model = %#v, want %#v", settings.Meta["model"], want)
	}
	if _, set := settings.Meta["effort"]; set {
		t.Errorf("a tier without effort must not add one: %#v", settings.Meta)
	}
}

func TestApplyModelTiers_LeavesLiteralModels(t *testing.T) {
	tiers := modelTiersFixture()
	b := Bundle{Agents: []Entry{
		{Kind: KindAgent, Name: "literal", Meta: map[string]any{"model": "gpt-5.5"}},
		{Kind: KindAgent, Name: "scoped", Meta: map[string]any{"model": map[string]any{"claude": "strong"}}},
		{Kind: KindAgent, Name: "custom", Meta: map[string]any{"x-codex": map[string]any{"model": "strong"}}},
	}}
	b.ApplyModelTiers(tiers)

	if b.Agents[0].Meta["model"] != "gpt-5.5" || b.Agents[0].ModelTier != "" {
		t.Errorf("literal changed: %#v", b.Agents[0])
	}
	if want := map[string]any{"claude": "strong"}; !reflect.DeepEqual(b.Agents[1].Meta["model"], want) {
		t.Errorf("a per-target value is literal, got %#v", b.Agents[1].Meta["model"])
	}
	if custom := b.Agents[2].Meta["x-codex"].(map[string]any); custom["model"] != "strong" {
		t.Errorf("x-codex.model is literal, got %#v", custom)
	}
	if tiers["strong"].Models["claude"] != "opus" {
		t.Errorf("tiers mutated: %#v", tiers)
	}
}

func TestApplyModelTiers_TierEffortSkipsTargetsTheSpecSets(t *testing.T) {
	b := Bundle{Agents: []Entry{
		{Kind: KindAgent, Name: "mapped", Meta: map[string]any{"model": map[string]any{"codex": "o4-mini", "default": "strong"}}},
		{Kind: KindAgent, Name: "custom", Meta: map[string]any{"model": "strong", "x-codex": map[string]any{"model": "o4-mini"}}},
	}}
	b.ApplyModelTiers(modelTiersFixture())

	for _, agent := range b.Agents {
		if want := map[string]any{"claude": "xhigh"}; !reflect.DeepEqual(agent.Meta["effort"], want) {
			t.Errorf("%s effort = %#v, want %#v", agent.Name, agent.Meta["effort"], want)
		}
	}
	if custom := b.Agents[1].Meta["x-codex"].(map[string]any); custom["model"] != "o4-mini" {
		t.Errorf("x-codex = %#v", custom)
	}
}

func TestApplyModelTiers_ScalarTierEffortSkipsTargetsTheSpecSets(t *testing.T) {
	tiers := map[string]config.ModelTier{"strong": {Models: map[string]string{"claude": "opus"}, Effort: "high"}}
	b := Bundle{Agents: []Entry{{Kind: KindAgent, Name: "a", Meta: map[string]any{"model": map[string]any{"codex": "o4-mini", "default": "strong"}}}}}
	b.ApplyModelTiers(tiers)

	meta := b.Agents[0].Meta
	if meta["effort"] != "high" {
		t.Errorf("effort = %#v", meta["effort"])
	}
	custom, _ := meta["x-codex"].(map[string]any)
	if v, set := custom["effort"]; !set || v != nil {
		t.Errorf("codex needs an effort delete marker, got %#v", meta["x-codex"])
	}
}
