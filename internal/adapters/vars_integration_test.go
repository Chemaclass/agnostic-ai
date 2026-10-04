package adapters

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// The point of the feature: one spec body, different paths per target.
func TestExpandBundleVars_ResolvesPerTarget(t *testing.T) {
	body := "Put new skills in {{$SKILLS_DIR}}."
	for target, want := range map[string]string{
		"claude":  "Put new skills in .claude/skills.",
		"codex":   "Put new skills in .agents/skills.",
		"copilot": "Put new skills in .github/skills.",
		"trae":    "Put new skills in .trae/skills.",
	} {
		b := spec.NewBundle([]spec.Entry{{Kind: spec.KindSkill, Name: "r", Body: body}})
		got := expandBundleVars(b, &config.Config{}, target).Skills[0].Body
		if got != want {
			t.Errorf("%s: got %q, want %q", target, got, want)
		}
	}
}

// A config override must win, or the spec body would name a directory
// the tree does not use.
func TestExpandBundleVars_HonorsOutputOverride(t *testing.T) {
	cfg := &config.Config{
		Outputs: map[string]config.Output{"claude": {SkillsDir: "custom/skills"}},
	}
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindSkill, Name: "r", Body: "see {{$SKILLS_DIR}}"}})

	if got := expandBundleVars(b, cfg, "claude").Skills[0].Body; got != "see custom/skills" {
		t.Errorf("override ignored, got %q", got)
	}
}

// Expansion must not mutate the caller's bundle, or the first target
// synced would freeze its paths into every later target's output.
func TestExpandBundleVars_DoesNotMutateSource(t *testing.T) {
	body := "see {{$SKILLS_DIR}}"
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindSkill, Name: "r", Body: body}})

	expandBundleVars(b, &config.Config{}, "claude")

	if b.Skills[0].Body != body {
		t.Errorf("source bundle mutated: %q", b.Skills[0].Body)
	}
	if got := expandBundleVars(b, &config.Config{}, "codex").Skills[0].Body; !strings.Contains(got, ".agents/skills") {
		t.Errorf("second target expanded from a mutated source: %q", got)
	}
}

// A target with no surface keeps the token rather than blanking it.
func TestExpandBundleVars_UnsupportedTargetKeepsToken(t *testing.T) {
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindSkill, Name: "r", Body: "see {{$SKILLS_DIR}}"}})

	if got := expandBundleVars(b, &config.Config{}, "jules").Skills[0].Body; got != "see {{$SKILLS_DIR}}" {
		t.Errorf("expected the token verbatim, got %q", got)
	}
}

// A rule keeps its variables until emit knows where it lands, then its
// own rule file gets the target's paths.
func TestRuleFile_ExpandsVariablesForItsTarget(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude", "codex"}}
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "r", Body: "see {{$SKILLS_DIR}}"}})

	out, _, err := emit.PrepareScopedRules(expandBundleVars(b, cfg, "claude"), cfg, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Rules) != 1 || out.Rules[0].Body != "see .claude/skills" {
		t.Errorf("want the claude path in its rule file, got %+v", out.Rules)
	}
}
