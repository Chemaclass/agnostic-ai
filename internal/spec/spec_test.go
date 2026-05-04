package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func TestSplitFrontmatter_NoFrontmatter(t *testing.T) {
	meta, body := splitFrontmatter([]byte("hello world"))
	if len(meta) != 0 {
		t.Fatalf("expected empty meta, got %v", meta)
	}
	if body != "hello world" {
		t.Fatalf("body mismatch: %q", body)
	}
}

func TestSplitFrontmatter_WithFrontmatter(t *testing.T) {
	input := []byte("---\nname: foo\ndescription: bar\n---\nbody here\n")
	meta, body := splitFrontmatter(input)
	if meta["name"] != "foo" {
		t.Fatalf("expected name=foo, got %v", meta["name"])
	}
	if body != "body here\n" {
		t.Fatalf("body mismatch: %q", body)
	}
}

func TestSplitFrontmatter_EmptyMeta(t *testing.T) {
	input := []byte("---\n---\nbody only\n")
	meta, body := splitFrontmatter(input)
	if len(meta) != 0 {
		t.Fatalf("expected empty meta, got %v", meta)
	}
	if body != "body only\n" {
		t.Fatalf("body mismatch: %q", body)
	}
}

func TestSplitFrontmatter_MalformedYAML(t *testing.T) {
	input := []byte("---\n: : :\n---\nbody\n")
	meta, _ := splitFrontmatter(input)
	if len(meta) != 0 {
		t.Fatal("expected empty meta on malformed yaml")
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
