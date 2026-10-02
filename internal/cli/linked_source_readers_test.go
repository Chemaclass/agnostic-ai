package cli

import (
	"bytes"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func linkedSourceSkill(t *testing.T) (spec.Entry, string) {
	t.Helper()
	testutil.TempCwd(t)
	physical := t.TempDir()
	writeTestFile(t, filepath.Join(physical, "SKILL.md"), skillSpec("demo", ""))
	writeTestFile(t, filepath.Join(physical, "references", "guide.txt"), "Initial asset.\n")
	testutil.DirectoryAlias(t, physical, "skill-link")
	return spec.Entry{
		Kind: spec.KindSkill,
		Name: "demo",
		Path: filepath.Join("skill-link", "SKILL.md"),
		Meta: map[string]any{"description": "Demo skill."},
		Body: "Steps.\n",
	}, physical
}

func TestSpecEntryFiles_LinkedSourceKeepsAssetProvenance(t *testing.T) {
	e, _ := linkedSourceSkill(t)
	got := specEntryFiles(".", e, func(string) bool { return true })
	want := []string{"skill-link/SKILL.md", "skill-link/references/guide.txt"}
	if !slices.Equal(got, want) {
		t.Errorf("spec files = %v, want %v", got, want)
	}
}

func TestEntrySum_LinkedSourceAssetEditChangesSum(t *testing.T) {
	e, physical := linkedSourceSkill(t)
	before := entrySum(e)
	writeTestFile(t, filepath.Join(physical, "references", "guide.txt"), "Updated asset.\n")
	if entrySum(e) == before {
		t.Error("editing a linked-root skill asset did not change the reported spec sum")
	}
}

func TestSkillAssets_LinkedSourceDetectsGlobalDifference(t *testing.T) {
	project, _ := linkedSourceSkill(t)
	globalRoot := t.TempDir()
	writeTestFile(t, filepath.Join(globalRoot, "SKILL.md"), skillSpec("demo", ""))
	writeTestFile(t, filepath.Join(globalRoot, "references", "guide.txt"), "Initial asset.\n")
	global := project
	global.Path = filepath.Join(globalRoot, "SKILL.md")
	if got := compareSkillAssets(project, global); got != contentIdentical {
		t.Errorf("same linked and global assets = %v, want identical", got)
	}
	writeTestFile(t, filepath.Join(globalRoot, "references", "guide.txt"), "Changed asset.\n")
	if got := compareSkillAssets(project, global); got != contentDiffers {
		t.Errorf("changed global asset = %v, want differs", got)
	}
	assets, err := skillAssets(project)
	if err != nil {
		t.Fatal(err)
	}
	if got := assets["references/guide.txt"].path; got != filepath.Join("skill-link", "references", "guide.txt") {
		t.Errorf("asset path = %q, want lexical linked path", got)
	}
}

func TestAddGlobalSkill_LinkedSourceCopiesNestedAsset(t *testing.T) {
	e, physical := linkedSourceSkill(t)
	outside := t.TempDir()
	writeTestFile(t, filepath.Join(outside, "outside.txt"), "Do not copy.\n")
	testutil.DirectoryAlias(t, outside, filepath.Join(physical, "nested-link"))
	files := map[string][]byte{}
	err := addGlobalSkill("native/demo", e, "claude", false, nil,
		func(path string, data []byte, _ fs.FileMode) error {
			files[filepath.ToSlash(path)] = data
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if got := files["native/demo/references/guide.txt"]; string(got) != "Initial asset.\n" {
		t.Errorf("global asset = %q, want Initial asset", got)
	}
	if !bytes.Contains(files["native/demo/SKILL.md"], []byte("Steps.")) {
		t.Errorf("global SKILL.md missing rendered body: %s", files["native/demo/SKILL.md"])
	}
	if _, ok := files["native/demo/nested-link/outside.txt"]; ok {
		t.Error("global sync traversed a nested directory link")
	}
}

func TestExplainInputs_LinkedSourceFindsReviewIncludes(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	physical := t.TempDir()
	writeTestFile(t, filepath.Join(physical, "web.md"), "---\nname: web\n---\n@apps/web/README.md\n")
	writeTestFile(t, "apps/web/README.md", "Web notes.\n")
	testutil.DirectoryAlias(t, physical, "reviews-link")
	writeTestFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nsources:\n  reviews: reviews-link\n")
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"explain", "--inputs"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "apps/web/README.md\n") {
		t.Errorf("review include missing from explained inputs:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "reviews-link/**\n") {
		t.Errorf("lexical review source missing from explained inputs:\n%s", out.String())
	}
}
