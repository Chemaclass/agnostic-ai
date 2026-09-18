package goose

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

func pluginSkillsConfig() *config.Config {
	return &config.Config{Outputs: map[string]config.Output{
		"goose": {SkillsDir: ".agents/plugins/agnostic-ai/skills"},
	}}
}

// Goose discovers a plugin by its manifest, so a bundle of skills and no
// hooks needs `plugin.json` too (target-audit 2026-09-18, #862).
func TestEmit_SkillsInPluginTree_WritesManifestWithoutHooks(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{Kind: spec.KindSkill, Name: "review", Body: "Review the diff."}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), pluginSkillsConfig(), false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents/plugins/agnostic-ai/skills/review/SKILL.md")); err != nil {
		t.Fatalf("expected the skill folder: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".agents/plugins/agnostic-ai/plugin.json"))
	if err != nil {
		t.Fatalf("missing plugin.json for a skills-only bundle: %v", err)
	}
	for _, want := range []string{`"name": "agnostic-ai"`, `"version": "1.0.0"`} {
		if !strings.Contains(string(got), want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// A plugin carrying both components is one plugin, so the two writers
// agree on a single manifest.
func TestEmit_SkillsAndHooksInOnePlugin_WriteOneManifest(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "review", Body: "Review the diff."},
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "PreToolUse", "command": "echo hi"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), pluginSkillsConfig(), false); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		".agents/plugins/agnostic-ai/plugin.json",
		".agents/plugins/agnostic-ai/hooks/hooks.json",
		".agents/plugins/agnostic-ai/skills/review/SKILL.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("expected %s: %v", rel, err)
		}
	}
	found := testutil.WalkRel(t, dir)
	manifests := 0
	for _, p := range found {
		if filepath.Base(p) == "plugin.json" {
			manifests++
		}
	}
	if manifests != 1 {
		t.Errorf("wrote %d manifests, want 1 (files: %v)", manifests, found)
	}
}

// The default skills tree is not a plugin, so a plain skill spec never
// writes a surprise manifest.
func TestEmit_SkillsOutsidePluginTree_WritesNoManifest(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{Kind: spec.KindSkill, Name: "review", Body: "Review the diff."}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents/plugins")); !os.IsNotExist(err) {
		t.Errorf("unexpected plugin tree for the default skills dir, err=%v", err)
	}
}

// Goose loads a plugin's skills from its `skills/` component directory.
// A skills dir under the plugin root but outside that component is not
// a plugin component, so no manifest claims it as one.
func TestPluginSkillsRoot_OnlyMatchesTheSkillsComponent(t *testing.T) {
	cases := map[string]string{
		".agents/plugins/agnostic-ai/skills":       ".agents/plugins/agnostic-ai",
		".agents/plugins/team-kit/skills":          ".agents/plugins/team-kit",
		".agents/plugins/agnostic-ai/skills/extra": "",
		".agents/plugins/agnostic-ai":              "",
		".agents/plugins/agnostic-ai/not-skills":   "",
		".agents/skills":                           "",
		"docs/team-skills":                         "",
		".agents/plugins":                          "",
	}
	for in, want := range cases {
		if got := pluginSkillsRoot(in); got != want {
			t.Errorf("pluginSkillsRoot(%q) = %q, want %q", in, got, want)
		}
	}
}
