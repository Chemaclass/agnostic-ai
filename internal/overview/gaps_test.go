package overview

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func gapByID(gaps []Gap, id string) (Gap, bool) {
	for _, g := range gaps {
		if g.ID == id {
			return g, true
		}
	}
	return Gap{}, false
}

func TestBuild_ListsAKindTheEnabledToolsSupportWithNoSpec(t *testing.T) {
	v := Build(Inputs{
		Cfg:    &config.Config{Targets: []string{"claude", "codex"}, Builtins: []string{"handoff"}},
		Bundle: spec.Bundle{Rules: []spec.Entry{entry(spec.KindRule, "testing", ".agnostic-ai/rules/testing.md", nil)}},
	})

	skills, ok := gapByID(v.Health.Gaps, "kind:skill")
	if !ok {
		t.Fatalf("no skills gap in %+v", v.Health.Gaps)
	}
	if skills.Label != "Skills" || skills.Section != SectionContext || !slices.Equal(skills.Supported, []string{"claude", "codex"}) || skills.Missing != "no skill yet" {
		t.Errorf("skills gap: %+v", skills)
	}
	if !strings.Contains(skills.Fix, "agnostic-ai new skill") || !strings.Contains(skills.Fix, "agnostic-ai sync") {
		t.Errorf("skills fix prompt: %q", skills.Fix)
	}
	if _, ok := gapByID(v.Health.Gaps, "kind:rule"); ok {
		t.Error("a kind with a spec is not a gap")
	}
	if _, ok := gapByID(v.Health.Gaps, "builtin:handoff"); ok {
		t.Error("an enabled built-in is not a gap")
	}
}

func TestBuild_LeavesOutAKindNoEnabledToolSupports(t *testing.T) {
	caps := capabilities()
	for _, g := range Build(Inputs{Cfg: &config.Config{Targets: []string{"claude", "codex"}}}).Health.Gaps {
		if !strings.HasPrefix(g.ID, "kind:") {
			continue
		}
		for _, target := range g.Supported {
			if !slices.Contains(caps[target], spec.Kind(strings.TrimPrefix(g.ID, "kind:"))) {
				t.Errorf("%s names %s, which does not support it", g.ID, target)
			}
		}
		if len(g.Supported) == 0 {
			t.Errorf("%s names no tool", g.ID)
		}
	}
	if gaps := Build(Inputs{Cfg: &config.Config{}}).Health.Gaps; len(gaps) != 0 {
		t.Errorf("no tools, gaps %+v", gaps)
	}
}

func TestBuild_ListsAStageWithHookSupportAndNoHook(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}}
	empty := Build(Inputs{Cfg: cfg}).Health.Gaps
	stage, ok := gapByID(empty, "stage:before-tool")
	if !ok {
		t.Fatalf("no before-tool gap in %+v", empty)
	}
	if stage.Section != SectionLifecycle || !slices.Equal(stage.Supported, []string{"claude"}) || !strings.Contains(stage.Fix, "on: before-tool") || !strings.Contains(stage.Fix, "Exit 2 blocks the call.") {
		t.Errorf("before-tool gap: %+v", stage)
	}

	hook := entry(spec.KindHook, "guard", ".agnostic-ai/hooks/guard.yaml", map[string]any{"on": "before-tool"})
	gaps := Build(Inputs{Cfg: cfg, Bundle: spec.Bundle{Hooks: []spec.Entry{hook}}}).Health.Gaps
	if _, ok := gapByID(gaps, "stage:before-tool"); ok {
		t.Error("a stage with a hook is not a gap")
	}
	if _, ok := gapByID(gaps, "kind:hook"); ok {
		t.Error("hooks are listed by stage, never as a kind")
	}
}

func TestBuild_ListsTheRecommendedBuiltInWhenItIsOff(t *testing.T) {
	gap, ok := gapByID(Build(Inputs{Cfg: &config.Config{Targets: []string{"claude"}}}).Health.Gaps, "builtin:handoff")
	if !ok {
		t.Fatal("no handoff gap")
	}
	if gap.Section != SectionSettings || gap.Label != "Handoff" || !strings.Contains(gap.Fix, "builtins") {
		t.Errorf("handoff gap: %+v", gap)
	}
}

func TestBuild_ListsNoGapsForTheUserConfiguration(t *testing.T) {
	v := Build(Inputs{Cfg: &config.Config{Targets: []string{"claude"}}, Global: true})
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"health":{"gaps":[]}`) {
		t.Errorf("global view: %s", data)
	}
}

func TestBuild_CountsOnlyTheProjectsOwnSpecs(t *testing.T) {
	skill := entry(spec.KindSkill, "handoff", "/cache/builtins/handoff/skills/handoff/SKILL.md", nil)
	skill.Layer = builtinLayer
	hook := entry(spec.KindHook, "memory", "/cache/builtins/memory/hooks/memory.yaml", map[string]any{"on": "session-start"})
	hook.Layer = builtinLayer
	gaps := Build(Inputs{Cfg: &config.Config{Targets: []string{"claude"}, Builtins: []string{"handoff", "memory"}}, Bundle: spec.Bundle{Skills: []spec.Entry{skill}, Hooks: []spec.Entry{hook}}}).Health.Gaps
	for _, id := range []string{"kind:skill", "stage:session-start"} {
		if _, ok := gapByID(gaps, id); !ok {
			t.Errorf("a built-in spec filled %s", id)
		}
	}
}

func TestBuild_ANativeHookFillsTheStageItsEventMapsTo(t *testing.T) {
	native := entry(spec.KindHook, "guard", ".agnostic-ai/hooks/guard.yaml", map[string]any{"event": "PreToolUse", "matcher": "Bash"})
	gaps := Build(Inputs{Cfg: &config.Config{Targets: []string{"claude"}}, Bundle: spec.Bundle{Hooks: []spec.Entry{native}}}).Health.Gaps
	if _, ok := gapByID(gaps, "stage:before-tool"); ok {
		t.Error("a PreToolUse hook leaves before-tool reported as empty")
	}
	if _, ok := gapByID(gaps, "stage:after-tool"); !ok {
		t.Error("a PreToolUse hook filled another stage")
	}
}

func TestBuild_TheHookPromptReplacesTheScaffoldsNativeFields(t *testing.T) {
	gap, _ := gapByID(Build(Inputs{Cfg: &config.Config{Targets: []string{"claude"}}}).Health.Gaps, "stage:stop")
	if !strings.Contains(gap.Fix, "remove its `event` and `matcher` lines") {
		t.Errorf("stop fix prompt: %q", gap.Fix)
	}
}

func TestBuild_TheBuiltInPromptNamesTheFileWhoseListWins(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}}
	shared, _ := gapByID(Build(Inputs{Cfg: cfg}).Health.Gaps, "builtin:handoff")
	if !strings.Contains(shared.Fix, "`builtins` list in agnostic-ai.yaml") {
		t.Errorf("shared list prompt: %q", shared.Fix)
	}
	local, _ := gapByID(Build(Inputs{Cfg: cfg, BuiltinsLocal: true, BuiltinsTeam: []string{"handoff"}}).Health.Gaps, "builtin:handoff")
	if !strings.Contains(local.Fix, "`builtins` list in agnostic-ai.local.yaml") {
		t.Errorf("local list prompt: %q", local.Fix)
	}
}

func TestBuild_CountsTheImprovementsAppliedFromTheGapRules(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}, Builtins: []string{"handoff"}}
	rule := entry(spec.KindRule, "testing", ".agnostic-ai/rules/testing.md", nil)
	before := Build(Inputs{Cfg: cfg, Bundle: spec.Bundle{Rules: []spec.Entry{rule}}}).Health
	if before.Improvements == nil {
		t.Fatal("no improvements count")
	}
	if got := *before.Improvements; got.Applied != 2 || got.Total-got.Applied != len(before.Gaps) {
		t.Errorf("improvements %+v with %d gaps, want 2 applied and one gap per missing improvement", got, len(before.Gaps))
	}

	skill := entry(spec.KindSkill, "release", ".agnostic-ai/skills/release/SKILL.md", nil)
	after := Build(Inputs{Cfg: cfg, Bundle: spec.Bundle{Rules: []spec.Entry{rule}, Skills: []spec.Entry{skill}}}).Health
	if after.Improvements.Applied != 3 || after.Improvements.Total != before.Improvements.Total {
		t.Errorf("after adding a skill: %+v, before %+v", *after.Improvements, *before.Improvements)
	}
}

func TestBuild_CountsNoImprovementsWithoutTools(t *testing.T) {
	got := Build(Inputs{Cfg: &config.Config{}}).Health.Improvements
	if got == nil || got.Applied != 0 || got.Total != 0 {
		t.Errorf("no tools, improvements %+v", got)
	}
}
