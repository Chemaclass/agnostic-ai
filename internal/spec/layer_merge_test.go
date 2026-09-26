package spec

import (
	"path/filepath"
	"reflect"
	"testing"
)

func loadExtending(t *testing.T, base, local string) Bundle {
	t.Helper()
	src := defaultsForTest().Sources
	src.MCPs = "mcps"
	b, err := LoadLayered([]Layer{
		{Name: "project", Root: base, Sources: src},
		{Name: "project-user", Root: local, Sources: src, Extends: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestLoadLayered_LocalLayerOverridesOneFieldAndExtendsTheBody(t *testing.T) {
	t.Parallel()
	base, local := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(base, "skills", "custom", "SKILL.md"),
		"---\nname: custom\ndescription: Shared skill\nmodel:\n  claude: sonnet\n  codex: gpt\n---\nfoo for\n")
	mustWrite(t, filepath.Join(local, "skills", "custom", "SKILL.md"),
		"---\nname: custom\nmodel:\n  claude: opus\n---\n::parent\nbar baz\n")

	got := loadExtending(t, base, local).Skills[0]
	if want := map[string]any{"claude": "opus", "codex": "gpt"}; !reflect.DeepEqual(got.Meta["model"], want) {
		t.Errorf("model = %v, want %v", got.Meta["model"], want)
	}
	if got.Meta["description"] != "Shared skill" {
		t.Errorf("description = %v, want the shared one", got.Meta["description"])
	}
	if got.Body != "foo for\nbar baz\n" {
		t.Errorf("body = %q, want the shared body then the local lines", got.Body)
	}
	if got.Layer != "project-user" {
		t.Errorf("layer = %q, want project-user", got.Layer)
	}
}

func TestLoadLayered_LocalLayerKeepsTheSharedBodyWhenItsOwnIsEmpty(t *testing.T) {
	t.Parallel()
	base, local := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(base, "agents", "reviewer.md"), "---\nname: reviewer\nmodel: sonnet\n---\nReview carefully.\n")
	mustWrite(t, filepath.Join(local, "agents", "reviewer.md"), "---\nname: reviewer\nmodel: opus\n---\n")

	got := loadExtending(t, base, local).Agents[0]
	if got.Meta["model"] != "opus" || got.Body != "Review carefully.\n" {
		t.Errorf("got model %v body %q", got.Meta["model"], got.Body)
	}
}

func TestLoadLayered_LocalLayerBodyWithoutParentReplacesTheSharedOne(t *testing.T) {
	t.Parallel()
	base, local := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(base, "rules", "style.md"), "---\nname: style\n---\nShared.\n")
	mustWrite(t, filepath.Join(local, "rules", "style.md"), "---\nname: style\n---\nMine.\n")

	if got := loadExtending(t, base, local).Rules[0].Body; got != "Mine.\n" {
		t.Errorf("body = %q, want the local body", got)
	}
}

func TestLoadLayered_LocalLayerParentCanWrapTheSharedBody(t *testing.T) {
	t.Parallel()
	base, local := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(base, "rules", "style.md"), "---\nname: style\n---\nShared.\n")
	mustWrite(t, filepath.Join(local, "rules", "style.md"), "---\nname: style\n---\nBefore.\n::parent\nAfter.\n")

	if got := loadExtending(t, base, local).Rules[0].Body; got != "Before.\nShared.\nAfter.\n" {
		t.Errorf("body = %q", got)
	}
}

func TestLoadLayered_LocalLayerNullDeletesAField(t *testing.T) {
	t.Parallel()
	base, local := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(base, "agents", "reviewer.md"), "---\nname: reviewer\ntools: [Read, Bash]\nmodel: sonnet\n---\nBody.\n")
	mustWrite(t, filepath.Join(local, "agents", "reviewer.md"), "---\nname: reviewer\ntools: null\n---\n")

	got := loadExtending(t, base, local).Agents[0]
	if _, ok := got.Meta["tools"]; ok {
		t.Errorf("tools kept: %v", got.Meta["tools"])
	}
	for _, k := range got.MetaKeys {
		if k == "tools" {
			t.Errorf("tools kept in key order: %v", got.MetaKeys)
		}
	}
	if got.Meta["model"] != "sonnet" {
		t.Errorf("model = %v, want the shared one", got.Meta["model"])
	}
}

func TestLoadLayered_LocalLayerMergesMapsAndReplacesLists(t *testing.T) {
	t.Parallel()
	base, local := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(base, "mcps", "docs.yaml"), "name: docs\ncommand: npx\nargs: [a, b]\nenv:\n  A: \"1\"\n  B: \"2\"\n")
	mustWrite(t, filepath.Join(local, "mcps", "docs.yaml"), "name: docs\nargs: [c]\nenv:\n  B: \"3\"\n  C: \"4\"\n")

	got := loadExtending(t, base, local).MCPs[0]
	if !reflect.DeepEqual(got.Meta["args"], []any{"c"}) {
		t.Errorf("args = %v, want the local list", got.Meta["args"])
	}
	if want := map[string]any{"A": "1", "B": "3", "C": "4"}; !reflect.DeepEqual(got.Meta["env"], want) {
		t.Errorf("env = %v, want %v", got.Meta["env"], want)
	}
	if got.Meta["command"] != "npx" {
		t.Errorf("command = %v, want the shared one", got.Meta["command"])
	}
	if want := []string{"name", "command", "args", "env"}; !reflect.DeepEqual(got.MetaKeys, want) {
		t.Errorf("key order = %v, want %v", got.MetaKeys, want)
	}
}

func TestLoadLayered_LocalLayerNewNameDropsAStrayParentMarker(t *testing.T) {
	t.Parallel()
	base, local := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(local, "rules", "mine.md"), "---\nname: mine\n---\n::parent\nOnly mine.\n")

	if got := loadExtending(t, base, local).Rules[0].Body; got != "Only mine.\n" {
		t.Errorf("body = %q", got)
	}
}

func TestLoadLayered_LocalSkillKeepsSharedAssetsUnlessItShipsItsOwn(t *testing.T) {
	t.Parallel()
	base, local := t.TempDir(), t.TempDir()
	baseSkill := filepath.Join(base, "skills", "lint", "SKILL.md")
	mustWrite(t, baseSkill, "---\nname: lint\n---\nLint.\n")
	mustWrite(t, filepath.Join(base, "skills", "lint", "check.sh"), "echo shared\n")
	mustWrite(t, filepath.Join(local, "skills", "lint", "SKILL.md"), "---\nname: lint\ndescription: Mine\n---\n")

	localSkill := filepath.Join(local, "skills", "lint", "SKILL.md")
	got := loadExtending(t, base, local).Skills[0]
	if got.Path != localSkill {
		t.Errorf("path = %q, want the local file the author edits", got.Path)
	}
	if got.SkillAssetDir() != filepath.Dir(baseSkill) {
		t.Errorf("asset dir = %q, want the shared folder", got.SkillAssetDir())
	}

	mustWrite(t, filepath.Join(local, "skills", "lint", "check.sh"), "echo mine\n")
	if got := loadExtending(t, base, local).Skills[0].SkillAssetDir(); got != filepath.Dir(localSkill) {
		t.Errorf("asset dir = %q, want the local folder once it ships assets", got)
	}
}

func TestLoadLayered_NonExtendingLayerStillReplacesTheWholeEntry(t *testing.T) {
	t.Parallel()
	base, over := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(base, "agents", "reviewer.md"), "---\nname: reviewer\nmodel: sonnet\n---\nShared.\n")
	mustWrite(t, filepath.Join(over, "agents", "reviewer.md"), "---\nname: reviewer\n---\n")

	src := defaultsForTest().Sources
	b, err := LoadLayered([]Layer{{Name: "pack:x", Root: base, Sources: src}, {Name: "project", Root: over, Sources: src}})
	if err != nil {
		t.Fatal(err)
	}
	if got := b.Agents[0]; got.Meta["model"] != nil || got.Body != "" {
		t.Errorf("project over a pack must replace whole, got model %v body %q", got.Meta["model"], got.Body)
	}
}

// Two local files with one name are an authoring mistake that lint
// reports; sync still extends the shared spec exactly once, with the
// winner, so inherited fields stay and ::parent never leaks.
func TestLoadLayered_LocalDuplicateStillExtendsTheSharedSpec(t *testing.T) {
	t.Parallel()
	base, local := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(base, "rules", "style.md"), "---\nname: style\ndescription: Shared\n---\nShared.\n")
	mustWrite(t, filepath.Join(local, "rules", "a.md"), "---\nname: style\n---\nFirst.\n")
	mustWrite(t, filepath.Join(local, "rules", "b.md"), "---\nname: style\n---\n::parent\nSecond.\n")

	b := loadExtending(t, base, local)
	got := b.Rules[0]
	if got.Body != "Shared.\nSecond.\n" || got.Meta["description"] != "Shared" {
		t.Errorf("got description %v body %q", got.Meta["description"], got.Body)
	}
	if len(b.Shadowed) != 1 || filepath.Base(b.Shadowed[0].Path) != "a.md" {
		t.Errorf("shadowed = %+v, want a.md", b.Shadowed)
	}
}
