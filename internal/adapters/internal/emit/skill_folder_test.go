package emit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSkillMarkdown_RendersNameDescriptionAndBody(t *testing.T) {
	s := spec.Entry{
		Kind: spec.KindSkill, Name: "deploy",
		Meta: map[string]any{"description": "Ship it."},
		Body: "Run the pipeline.\n",
	}
	got := SkillMarkdown(s, "codex")
	want := "---\nname: deploy\ndescription: Ship it.\n---\n\nRun the pipeline.\n"
	if got != want {
		t.Errorf("SkillMarkdown = %q, want %q", got, want)
	}
}

// Byte parity across the shared-tree targets is load-bearing: codex,
// amp, and zed dedupe one .agents/skills write only when equal.
func TestSkillMarkdown_IdenticalAcrossTargetsForPlainSpec(t *testing.T) {
	s := spec.Entry{
		Kind: spec.KindSkill, Name: "deploy",
		Meta: map[string]any{"description": "Ship it."},
		Body: "Run the pipeline.",
	}
	base := SkillMarkdown(s, "codex")
	for _, target := range []string{"amp", "zed", "gemini", "opencode", "copilot"} {
		if got := SkillMarkdown(s, target); got != base {
			t.Errorf("%s render diverges from codex for a plain spec:\n%q\nvs\n%q", target, got, base)
		}
	}
}

func TestSkillMarkdown_TargetMetaPassesThroughAndExcludes(t *testing.T) {
	s := spec.Entry{
		Kind: spec.KindSkill, Name: "deploy",
		Meta: map[string]any{
			"description": "Ship it.",
			"x-codex":     map[string]any{"custom": "v", "interface": map[string]any{"x": 1}},
		},
		Body: "body",
	}
	got := SkillMarkdown(s, "codex", "interface")
	if !strings.Contains(got, "custom: v") {
		t.Errorf("x-codex custom key should pass through:\n%s", got)
	}
	if strings.Contains(got, "interface:") {
		t.Errorf("excluded key must not reach the frontmatter:\n%s", got)
	}
}

func TestWriteSkillFolder_WritesSKILLMdAndAssets(t *testing.T) {
	dir := testutil.TempCwd(t)
	src := filepath.Join(dir, "srcskill")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: deploy\n---\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "helper.sh"), []byte("echo hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := spec.Entry{
		Kind: spec.KindSkill, Name: "deploy",
		Path: filepath.Join(src, "SKILL.md"),
		Body: "body",
	}
	if err := WriteSkillFolder(s, "zed", "out/skills", false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "out/skills/deploy/SKILL.md"))
	if err != nil {
		t.Fatalf("SKILL.md not written: %v", err)
	}
	if !strings.Contains(string(data), "name: deploy") {
		t.Errorf("SKILL.md missing frontmatter:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "out/skills/deploy/helper.sh")); err != nil {
		t.Errorf("asset should propagate: %v", err)
	}
}
