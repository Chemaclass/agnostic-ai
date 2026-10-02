package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestLoad_LinkedSourceRelativeRootsRetainLexicalPaths(t *testing.T) {
	checkLinkedSourceRoots(t, false)
}

func TestLoad_LinkedSourceAbsoluteRootsRetainLexicalPaths(t *testing.T) {
	checkLinkedSourceRoots(t, true)
}

func checkLinkedSourceRoots(t *testing.T, absolute bool) {
	t.Helper()
	project := t.TempDir()
	testutil.Chdir(t, project)
	external := t.TempDir()
	mustWrite(t, filepath.Join(external, "rules", "backend", "style.md"), "---\nname: style\n---\nUse plain words.\n")
	mustWrite(t, filepath.Join(external, "agents", "backend", "reviewer.md"), "---\nname: reviewer\n---\nReview carefully.\n")
	mustWrite(t, filepath.Join(external, "skills", "backend", "helper", "SKILL.md"), "---\nname: helper\n---\nHelp with edits.\n")
	mustWrite(t, filepath.Join(external, "skills", "backend", "helper", "references", "guide.md"), "Bundled reference.\n")
	mustWrite(t, filepath.Join(external, "mcps", "docs.yaml"), "name: docs\ncommand: npx\n")
	if err := os.MkdirAll(filepath.Join(project, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "linked"), 0o755); err != nil {
		t.Fatal(err)
	}
	aliases := map[string]string{}
	for _, kind := range []string{"rules", "agents", "skills", "mcps"} {
		alias := filepath.Join("linked", kind)
		if absolute {
			alias = filepath.Join(project, alias)
		}
		testutil.DirectoryAlias(t, filepath.Join(external, kind), alias)
		aliases[kind] = alias
	}
	b, err := LoadBundle(".", &config.Config{Sources: config.Sources{
		Rules: aliases["rules"], Agents: aliases["agents"], Skills: aliases["skills"], MCPs: aliases["mcps"],
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		kind    string
		entries []Entry
		name    string
		path    string
		scope   string
		body    string
	}{
		{"rules", b.Rules, "style", filepath.Join(aliases["rules"], "backend", "style.md"), "backend", "Use plain words.\n"},
		{"agents", b.Agents, "reviewer", filepath.Join(aliases["agents"], "backend", "reviewer.md"), "backend", "Review carefully.\n"},
		{"skills", b.Skills, "helper", filepath.Join(aliases["skills"], "backend", "helper", "SKILL.md"), "backend", "Help with edits.\n"},
		{"mcps", b.MCPs, "docs", filepath.Join(aliases["mcps"], "docs.yaml"), "", ""},
	} {
		if len(want.entries) != 1 {
			t.Errorf("linked %s entries = %#v, want one", want.kind, want.entries)
			continue
		}
		got := want.entries[0]
		if got.Name != want.name || got.Path != want.path || got.Scope != want.scope || got.Body != want.body || got.Layer != "project" {
			t.Errorf("linked %s = %#v, want name=%q path=%q scope=%q body=%q layer=project", want.kind, got, want.name, want.path, want.scope, want.body)
		}
		if want.kind == "skills" && got.SkillAssetDir() != filepath.Dir(want.path) {
			t.Errorf("skill assets = %q, want lexical folder %q", got.SkillAssetDir(), filepath.Dir(want.path))
		}
		if want.kind == "mcps" && got.Meta["command"] != "npx" {
			t.Errorf("linked MCP command = %v, want npx", got.Meta["command"])
		}
	}
}

func TestLoad_LinkedSourceRootSkillKeepsAssetsOutOfSkillEntries(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	external := t.TempDir()
	mustWrite(t, filepath.Join(external, "SKILL.md"), "---\nname: helper\n---\nHelp with edits.\n")
	mustWrite(t, filepath.Join(external, "references", "guide.md"), "Reference, not another skill.\n")
	mustWrite(t, filepath.Join(external, "scripts", "check.sh"), "echo check\n")
	alias := "linked-skill"
	testutil.DirectoryAlias(t, external, alias)
	b, err := LoadBundle(".", &config.Config{Sources: config.Sources{Skills: alias}})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Skills) != 1 {
		t.Fatalf("linked root skills = %#v, want one folder skill", b.Skills)
	}
	got := b.Skills[0]
	if got.Name != "helper" || got.Path != filepath.Join(alias, "SKILL.md") || got.SkillAssetDir() != alias || got.Scope != "" {
		t.Errorf("linked root skill = %#v, want lexical path and assets under %q", got, alias)
	}
}

func TestExtendEntry_LinkedSourceSkillUsesOwnAssets(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	baseDir, localDir := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(baseDir, "SKILL.md"), "Shared skill.\n")
	mustWrite(t, filepath.Join(baseDir, "check.sh"), "echo shared\n")
	mustWrite(t, filepath.Join(localDir, "SKILL.md"), "Local skill.\n")
	alias := "linked-local-skill"
	testutil.DirectoryAlias(t, localDir, alias)
	base := Entry{Kind: KindSkill, Name: "helper", Path: filepath.Join(baseDir, "SKILL.md"), Body: "Shared skill.\n"}
	over := Entry{Kind: KindSkill, Name: "helper", Path: filepath.Join(alias, "SKILL.md"), Body: "Local skill.\n"}
	if got := extendEntry(base, over); got.SkillAssetDir() != baseDir {
		t.Errorf("body-only extension assets = %q, want shared folder %q", got.SkillAssetDir(), baseDir)
	}
	testutil.DirectoryAlias(t, baseDir, filepath.Join(localDir, "nested-link"))
	if got := extendEntry(base, over); got.SkillAssetDir() != baseDir {
		t.Errorf("extension containing only a nested link lost shared assets: %q", got.SkillAssetDir())
	}
	mustWrite(t, filepath.Join(localDir, "scripts", "check.sh"), "echo local\n")
	got := extendEntry(base, over)
	if got.SkillAssetDir() != alias || got.Path != over.Path || got.Body != over.Body {
		t.Errorf("extension with assets = %#v, want local lexical asset folder %q", got, alias)
	}
}
