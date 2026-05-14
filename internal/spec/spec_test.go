package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func TestSplitFrontmatter_NoFrontmatter(t *testing.T) {
	meta, body, err := splitFrontmatter([]byte("hello world"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(meta) != 0 {
		t.Fatalf("expected empty meta, got %v", meta)
	}
	if body != "hello world" {
		t.Fatalf("body mismatch: %q", body)
	}
}

func TestSplitFrontmatter_WithFrontmatter(t *testing.T) {
	input := []byte("---\nname: foo\ndescription: bar\n---\nbody here\n")
	meta, body, err := splitFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta["name"] != "foo" {
		t.Fatalf("expected name=foo, got %v", meta["name"])
	}
	if body != "body here\n" {
		t.Fatalf("body mismatch: %q", body)
	}
}

func TestSplitFrontmatter_EmptyMeta(t *testing.T) {
	input := []byte("---\n---\nbody only\n")
	meta, body, err := splitFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(meta) != 0 {
		t.Fatalf("expected empty meta, got %v", meta)
	}
	if body != "body only\n" {
		t.Fatalf("body mismatch: %q", body)
	}
}

func TestSplitFrontmatter_MalformedYAML(t *testing.T) {
	input := []byte("---\n: : :\n---\nbody\n")
	if _, _, err := splitFrontmatter(input); err == nil {
		t.Fatal("expected error on malformed yaml")
	}
}

func TestFilter(t *testing.T) {
	entries := []Entry{
		{Kind: KindAgent, Name: "a"},
		{Kind: KindRule, Name: "r"},
		{Kind: KindAgent, Name: "b"},
	}
	if got := Filter(entries, KindAgent); len(got) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(got))
	}
	if got := Filter(entries, KindHook); len(got) != 0 {
		t.Fatalf("expected 0 hooks, got %d", len(got))
	}
}

func TestLoadAll_ParsesEachKind(t *testing.T) {
	dir := t.TempDir()

	mustWrite(t, filepath.Join(dir, "agents", "a1.md"),
		"---\nname: a1\n---\nagent body")
	mustWrite(t, filepath.Join(dir, "skills", "s1", "SKILL.md"),
		"---\nname: s1\n---\nskill body")
	mustWrite(t, filepath.Join(dir, "rules", "r1.md"),
		"---\nname: r1\nalwaysApply: true\n---\nrule body")
	mustWrite(t, filepath.Join(dir, "hooks", "h1.yaml"),
		"name: h1\nevent: PostToolUse\nmatcher: \"*\"\ncommand: \"echo\"\n")

	cfg := defaultsForTest()
	entries, err := LoadAll(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}

	want := map[Kind]int{
		KindAgent: 1,
		KindSkill: 1,
		KindRule:  1,
		KindHook:  1,
	}
	for kind, n := range want {
		if got := len(Filter(entries, kind)); got != n {
			t.Errorf("kind %s: expected %d, got %d", kind, n, got)
		}
	}
}

func TestLoadAll_NameFromFilenameIfMissing(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "rules", "implicit-name.md"), "body without frontmatter")

	cfg := defaultsForTest()
	entries, err := LoadAll(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "implicit-name" {
		t.Fatalf("expected entry named implicit-name, got %+v", entries)
	}
}

func TestLoadAll_SkillNameFromParentDirWhenFrontmatterMissing(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "skills", "my-skill", "SKILL.md"), "skill body without frontmatter")

	cfg := defaultsForTest()
	entries, err := LoadAll(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "my-skill" {
		t.Errorf("expected name=my-skill (parent dir), got %q", entries[0].Name)
	}
	if entries[0].Kind != KindSkill {
		t.Errorf("expected kind=skill, got %q", entries[0].Kind)
	}
}

func TestLoadAll_MissingDirsAreSkipped(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultsForTest()
	entries, err := LoadAll(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestLoadAll_DerivesScopeFromLayout(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "rules", "root.md"), "body")
	mustWrite(t, filepath.Join(dir, "rules", "backend", "auth.md"), "body")
	mustWrite(t, filepath.Join(dir, "rules", "backend", "api", "limits.md"), "body")
	mustWrite(t, filepath.Join(dir, "skills", "validator", "SKILL.md"), "skill body")
	mustWrite(t, filepath.Join(dir, "skills", "backend", "loader.md"), "body")

	cfg := defaultsForTest()
	cfg.Sources.Skills = "skills"
	entries, err := LoadAll(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"root":   "",
		"auth":   "backend",
		"limits": "backend/api",
		"loader": "backend",
	}
	for _, e := range entries {
		exp, ok := want[e.Name]
		if !ok {
			continue
		}
		if e.Scope != exp {
			t.Errorf("entry %q scope = %q, want %q", e.Name, e.Scope, exp)
		}
	}
	for _, e := range entries {
		if e.Name == "validator" && e.Scope != "" {
			t.Errorf("nested SKILL.md should have empty scope, got %q", e.Scope)
		}
	}
}

func TestEffectiveScope_FrontmatterFallback(t *testing.T) {
	e := Entry{Meta: map[string]any{"scope": "/frontend/"}}
	if got := e.EffectiveScope(); got != "frontend" {
		t.Errorf("expected scope frontend, got %q", got)
	}
	e.Scope = "explicit"
	if got := e.EffectiveScope(); got != "explicit" {
		t.Errorf("layout scope should win, got %q", got)
	}
}

func TestParseYAML_HookFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hook.yaml")
	mustWrite(t, path, "name: foo\nevent: PreToolUse\ncommand: \"true\"\n")

	e, err := parseYAML(path)
	if err != nil {
		t.Fatal(err)
	}
	if e.Meta["name"] != "foo" {
		t.Fatalf("expected name=foo, got %v", e.Meta["name"])
	}
}

func TestLoadLayered_HigherLayerOverridesByName(t *testing.T) {
	base := t.TempDir()
	over := t.TempDir()

	mustWrite(t, filepath.Join(base, "rules", "shared.md"),
		"---\nname: shared\n---\nbase body")
	mustWrite(t, filepath.Join(base, "rules", "only-base.md"),
		"---\nname: only-base\n---\nbase only")
	mustWrite(t, filepath.Join(over, "rules", "shared.md"),
		"---\nname: shared\n---\nover body")
	mustWrite(t, filepath.Join(over, "rules", "only-over.md"),
		"---\nname: only-over\n---\nover only")

	src := defaultsForTest().Sources
	bundle, err := LoadLayered([]Layer{
		{Name: "base", Root: base, Sources: src},
		{Name: "over", Root: over, Sources: src},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(bundle.Rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(bundle.Rules))
	}
	got := map[string]Entry{}
	for _, e := range bundle.Rules {
		got[e.Name] = e
	}
	if got["shared"].Body != "over body" {
		t.Errorf("shared.Body = %q, want over body", got["shared"].Body)
	}
	if got["shared"].Layer != "over" {
		t.Errorf("shared.Layer = %q, want over", got["shared"].Layer)
	}
	if got["only-base"].Layer != "base" {
		t.Errorf("only-base.Layer = %q, want base", got["only-base"].Layer)
	}
	if got["only-over"].Layer != "over" {
		t.Errorf("only-over.Layer = %q, want over", got["only-over"].Layer)
	}
}

func TestLoadLayered_PreservesOrderForExistingNames(t *testing.T) {
	base := t.TempDir()
	over := t.TempDir()

	mustWrite(t, filepath.Join(base, "rules", "a.md"), "---\nname: a\n---\nbase a")
	mustWrite(t, filepath.Join(base, "rules", "b.md"), "---\nname: b\n---\nbase b")
	mustWrite(t, filepath.Join(over, "rules", "a.md"), "---\nname: a\n---\nover a")

	src := defaultsForTest().Sources
	bundle, err := LoadLayered([]Layer{
		{Name: "base", Root: base, Sources: src},
		{Name: "over", Root: over, Sources: src},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(bundle.Rules))
	}
	if bundle.Rules[0].Name != "a" || bundle.Rules[1].Name != "b" {
		t.Fatalf("order changed: %+v", bundle.Rules)
	}
	if bundle.Rules[0].Body != "over a" {
		t.Errorf("expected override body, got %q", bundle.Rules[0].Body)
	}
}

func TestLoadBundle_TagsLayerProject(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "rules", "x.md"), "---\nname: x\n---\nbody")
	bundle, err := LoadBundle(dir, defaultsForTest())
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Rules) != 1 || bundle.Rules[0].Layer != "project" {
		t.Fatalf("expected layer=project, got %+v", bundle.Rules)
	}
}

func defaultsForTest() *config.Config {
	return &config.Config{
		Sources: config.Sources{
			Agents: "agents",
			Skills: "skills",
			Rules:  "rules",
			Hooks:  "hooks",
		},
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
