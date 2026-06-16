package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func TestSplitFrontmatter_NoFrontmatter(t *testing.T) {
	meta, keys, _, body, err := splitFrontmatter([]byte("hello world"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(meta) != 0 {
		t.Fatalf("expected empty meta, got %v", meta)
	}
	if len(keys) != 0 {
		t.Fatalf("expected empty keys, got %v", keys)
	}
	if body != "hello world" {
		t.Fatalf("body mismatch: %q", body)
	}
}

func TestSplitFrontmatter_WithFrontmatter(t *testing.T) {
	input := []byte("---\nname: foo\ndescription: bar\n---\nbody here\n")
	meta, keys, _, body, err := splitFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta["name"] != "foo" {
		t.Fatalf("expected name=foo, got %v", meta["name"])
	}
	if len(keys) != 2 || keys[0] != "name" || keys[1] != "description" {
		t.Fatalf("expected ordered keys [name description], got %v", keys)
	}
	if body != "body here\n" {
		t.Fatalf("body mismatch: %q", body)
	}
}

func TestSplitFrontmatter_EmptyMeta(t *testing.T) {
	input := []byte("---\n---\nbody only\n")
	meta, _, _, body, err := splitFrontmatter(input)
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
	if _, _, _, _, err := splitFrontmatter(input); err == nil {
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

// A markdown asset that lives next to a folder-based skill's SKILL.md is a
// bundled asset, not a skill of its own. It must not be promoted to a
// top-level skill (#431).
func TestLoadAll_MarkdownAssetInsideSkillNotPromoted(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "skills", "alpha", "SKILL.md"), "---\nname: alpha\n---\nskill body")
	mustWrite(t, filepath.Join(dir, "skills", "alpha", "examples.md"), "extra markdown asset")
	mustWrite(t, filepath.Join(dir, "skills", "alpha", "docs", "notes.md"), "nested markdown asset")

	cfg := defaultsForTest()
	entries, err := LoadAll(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}

	skills := Filter(entries, KindSkill)
	if len(skills) != 1 {
		names := make([]string, len(skills))
		for i, s := range skills {
			names[i] = s.Name
		}
		t.Fatalf("expected 1 skill, got %d: %v", len(skills), names)
	}
	if skills[0].Name != "alpha" {
		t.Errorf("expected skill alpha, got %q", skills[0].Name)
	}
}

// Flat-file skills, including ones nested in a scope subdir, stay skills.
// They have no SKILL.md ancestor, so the asset-detection must not eat them.
func TestLoadAll_FlatFileSkillsSurviveAssetDetection(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "skills", "alpha.md"), "flat skill")
	mustWrite(t, filepath.Join(dir, "skills", "backend", "loader.md"), "scoped flat skill")
	mustWrite(t, filepath.Join(dir, "skills", "beta", "SKILL.md"), "folder skill")

	cfg := defaultsForTest()
	cfg.Sources.Skills = "skills"
	entries, err := LoadAll(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, s := range Filter(entries, KindSkill) {
		got[s.Name] = true
	}
	for _, name := range []string{"alpha", "loader", "beta"} {
		if !got[name] {
			t.Errorf("expected skill %q to load, got %v", name, got)
		}
	}
	if len(got) != 3 {
		t.Errorf("expected exactly 3 skills, got %d: %v", len(got), got)
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

func TestEntry_EmitsTo(t *testing.T) {
	cases := []struct {
		name   string
		meta   map[string]any
		target string
		want   bool
	}{
		{"no scoping → all targets", nil, "claude", true},
		{"empty target → permissive", map[string]any{"target": "claude"}, "", true},
		{"target match", map[string]any{"target": "claude"}, "claude", true},
		{"target mismatch", map[string]any{"target": "codex"}, "claude", false},
		{"targets list match", map[string]any{"targets": []any{"claude", "codex"}}, "codex", true},
		{"targets list miss", map[string]any{"targets": []any{"claude"}}, "codex", false},
		{"targets []string match", map[string]any{"targets": []string{"gemini"}}, "gemini", true},
		{"targets []string miss", map[string]any{"targets": []string{"gemini"}}, "claude", false},
		{"empty target string falls through to permissive", map[string]any{"target": ""}, "claude", true},
		{"empty targets list permissive", map[string]any{"targets": []any{}}, "claude", true},
		// target-exclude (single string): emits everywhere except the
		// named target. Mirrors target: but negated. Closes #292.
		{"target-exclude string blocks named", map[string]any{"target-exclude": "gemini"}, "gemini", false},
		{"target-exclude string passes other", map[string]any{"target-exclude": "gemini"}, "claude", true},
		// targets-exclude (list): blocks every listed target, emits to
		// the rest.
		{"targets-exclude []any blocks listed", map[string]any{"targets-exclude": []any{"gemini", "aider"}}, "aider", false},
		{"targets-exclude []any passes unlisted", map[string]any{"targets-exclude": []any{"gemini", "aider"}}, "claude", true},
		{"targets-exclude []string blocks listed", map[string]any{"targets-exclude": []string{"copilot"}}, "copilot", false},
		// Empty exclude list is permissive (do not block anything).
		{"empty targets-exclude is permissive", map[string]any{"targets-exclude": []any{}}, "claude", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := Entry{Kind: KindHook, Meta: tc.meta}
			if got := e.EmitsTo(tc.target); got != tc.want {
				t.Errorf("EmitsTo(%q) = %v, want %v (meta=%v)", tc.target, got, tc.want, tc.meta)
			}
		})
	}
}

// Bundle.For filters every kind by EmitsTo so adapters see only the
// entries scoped to their target. Mirrors Bundle.HooksFor across
// agents/skills/rules/mcps/commands. Closes #292.
func TestBundle_For(t *testing.T) {
	b := Bundle{
		Agents: []Entry{
			{Kind: KindAgent, Name: "a1"},
			{Kind: KindAgent, Name: "a2", Meta: map[string]any{"target": "codex"}},
		},
		Skills: []Entry{
			{Kind: KindSkill, Name: "s1", Meta: map[string]any{"targets": []any{"claude"}}},
		},
		Rules: []Entry{
			{Kind: KindRule, Name: "r1"},
			{Kind: KindRule, Name: "r2", Meta: map[string]any{"target-exclude": "claude"}},
		},
		MCPs: []Entry{
			{Kind: KindMCP, Name: "m1", Meta: map[string]any{"targets-exclude": []any{"codex"}}},
		},
		Commands: []Entry{
			{Kind: KindCommand, Name: "c1", Meta: map[string]any{"target": "claude"}},
		},
		Hooks: []Entry{
			{Kind: KindHook, Name: "h1"},
			{Kind: KindHook, Name: "h2", Meta: map[string]any{"target": "codex"}},
		},
	}
	claude := b.For("claude")
	if names := agentNames(claude.Agents); !equalSlices(names, []string{"a1"}) {
		t.Errorf("claude agents = %v, want [a1]", names)
	}
	if names := agentNames(claude.Skills); !equalSlices(names, []string{"s1"}) {
		t.Errorf("claude skills = %v, want [s1]", names)
	}
	if names := agentNames(claude.Rules); !equalSlices(names, []string{"r1"}) {
		t.Errorf("claude rules = %v, want [r1] (r2 excluded)", names)
	}
	if names := agentNames(claude.MCPs); !equalSlices(names, []string{"m1"}) {
		t.Errorf("claude mcps = %v, want [m1]", names)
	}
	if names := agentNames(claude.Commands); !equalSlices(names, []string{"c1"}) {
		t.Errorf("claude commands = %v, want [c1]", names)
	}
	if names := agentNames(claude.Hooks); !equalSlices(names, []string{"h1"}) {
		t.Errorf("claude hooks = %v, want [h1]", names)
	}

	codex := b.For("codex")
	if names := agentNames(codex.Agents); !equalSlices(names, []string{"a1", "a2"}) {
		t.Errorf("codex agents = %v, want [a1 a2]", names)
	}
	if names := agentNames(codex.Skills); !equalSlices(names, []string{}) {
		t.Errorf("codex skills = %v, want []", names)
	}
	if names := agentNames(codex.MCPs); !equalSlices(names, []string{}) {
		t.Errorf("codex mcps = %v, want [] (excluded)", names)
	}

	if all := b.For(""); len(all.Agents) != 2 || len(all.Hooks) != 2 {
		t.Errorf("empty target = identity copy, got agents=%d hooks=%d", len(all.Agents), len(all.Hooks))
	}
}

func agentNames(es []Entry) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, e.Name)
	}
	return out
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBundle_HooksFor(t *testing.T) {
	all := Bundle{Hooks: []Entry{
		{Kind: KindHook, Name: "a"},
		{Kind: KindHook, Name: "b", Meta: map[string]any{"target": "claude"}},
		{Kind: KindHook, Name: "c", Meta: map[string]any{"target": "codex"}},
		{Kind: KindHook, Name: "d", Meta: map[string]any{"targets": []any{"claude", "gemini"}}},
	}}
	got := all.HooksFor("claude")
	gotNames := make([]string, len(got))
	for i, h := range got {
		gotNames[i] = h.Name
	}
	want := []string{"a", "b", "d"}
	if len(got) != len(want) {
		t.Fatalf("HooksFor(claude) = %v, want %v", gotNames, want)
	}
	for i, n := range want {
		if gotNames[i] != n {
			t.Errorf("at %d: got %q, want %q", i, gotNames[i], n)
		}
	}
	if untouched := all.HooksFor(""); len(untouched) != 4 {
		t.Errorf("empty target should return all hooks, got %d", len(untouched))
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
