package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportFromCursor_TranslatesRules(t *testing.T) {
	dir := t.TempDir()
	mdc := `---
description: Always use Conventional Commits.
globs: "**/*"
alwaysApply: true
---

Use ` + "`feat:`" + `, ` + "`fix:`" + `, etc.
`
	writeFile(t, filepath.Join(dir, ".cursor", "rules", "conventional-commits.mdc"), mdc)

	if err := importFromCursor(dir, rootSources()); err != nil {
		t.Fatal(err)
	}

	out, err := os.ReadFile(filepath.Join(dir, "rules", "conventional-commits.md"))
	if err != nil {
		t.Fatalf("missing rules/conventional-commits.md: %v", err)
	}
	got := string(out)
	for _, want := range []string{
		"name: conventional-commits",
		"description: Always use Conventional Commits.",
		"alwaysApply: true",
		"Use `feat:`, `fix:`, etc.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output, got:\n%s", want, got)
		}
	}
}

func TestImportFromCursor_WalksNestedSubdirectories(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".cursor", "rules", "basics", "product.mdc"),
		"---\ndescription: product basics\n---\n\nproduct body\n")
	writeFile(t, filepath.Join(dir, ".cursor", "rules", "best-practices", "frontend.mdc"),
		"---\ndescription: frontend practices\n---\n\nfrontend body\n")
	writeFile(t, filepath.Join(dir, ".cursor", "rules", "top.mdc"),
		"---\ndescription: top level\n---\n\ntop body\n")

	if err := importFromCursor(dir, rootSources()); err != nil {
		t.Fatal(err)
	}

	cases := map[string][]string{
		filepath.Join("rules", "basics", "product.md"):          {"name: product", "product body"},
		filepath.Join("rules", "best-practices", "frontend.md"): {"name: frontend", "frontend body"},
		filepath.Join("rules", "top.md"):                        {"name: top", "top body"},
	}
	for rel, wants := range cases {
		out, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
		got := string(out)
		for _, want := range wants {
			if !strings.Contains(got, want) {
				t.Errorf("%s: expected %q, got:\n%s", rel, want, got)
			}
		}
	}
}

func TestImportFromCursor_NoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".cursor", "rules", "loose.mdc"), "Just plain body.\n")

	if err := importFromCursor(dir, rootSources()); err != nil {
		t.Fatal(err)
	}

	out, err := os.ReadFile(filepath.Join(dir, "rules", "loose.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if !strings.Contains(got, "name: loose") {
		t.Errorf("expected name: loose in frontmatter, got:\n%s", got)
	}
	if !strings.Contains(got, "Just plain body.") {
		t.Errorf("expected body preserved, got:\n%s", got)
	}
}

func TestImportFromCursor_PreservesUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	mdc := `---
description: scoped
globs: "src/**/*.ts"
alwaysApply: false
customKey: keep-me
---

body
`
	writeFile(t, filepath.Join(dir, ".cursor", "rules", "scoped.mdc"), mdc)

	if err := importFromCursor(dir, rootSources()); err != nil {
		t.Fatal(err)
	}

	out, err := os.ReadFile(filepath.Join(dir, "rules", "scoped.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, want := range []string{
		"globs: src/**/*.ts",
		"customKey: keep-me",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output, got:\n%s", want, got)
		}
	}
}

// TestImportFromCursor_DropsCatchAllGlobs guards round-trip stability: a
// catch-all globs value (hand-authored, or left over from a sync
// predating #536, when the adapter itself used to fill an empty globs
// with "**/*") must not carry back into the source, or it would flip an
// always-apply rule's representation each cycle (#429). Catch-all globs
// must not appear in the imported spec.
func TestImportFromCursor_DropsCatchAllGlobs(t *testing.T) {
	dir := t.TempDir()
	// Mirrors what the cursor adapter emits for an always-apply rule:
	// empty description, catch-all globs.
	mdc := `---
description:
globs: "**/*"
alwaysApply: true
---

Always rule body.
`
	writeFile(t, filepath.Join(dir, ".cursor", "rules", "always.mdc"), mdc)

	if err := importFromCursor(dir, rootSources()); err != nil {
		t.Fatal(err)
	}

	out, err := os.ReadFile(filepath.Join(dir, "rules", "always.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if strings.Contains(got, "globs:") {
		t.Errorf("catch-all globs leaked into source:\n%s", got)
	}
	if strings.Contains(got, "null") {
		t.Errorf("empty description emitted as null:\n%s", got)
	}
	if !strings.Contains(got, "alwaysApply: true") || !strings.Contains(got, "Always rule body.") {
		t.Errorf("expected alwaysApply and body preserved, got:\n%s", got)
	}
}

func TestImportFromCursor_NoCursorDir(t *testing.T) {
	dir := t.TempDir()
	if err := importFromCursor(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "rules"))
	if len(entries) != 0 {
		t.Errorf("expected empty rules/, got %d entries", len(entries))
	}
}

func TestImportFromCursor_SkipsNonMdc(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".cursor", "rules", "keep.mdc"), "body\n")
	writeFile(t, filepath.Join(dir, ".cursor", "rules", "ignore.txt"), "ignored\n")

	if err := importFromCursor(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "keep.md")); err != nil {
		t.Errorf("expected keep.md, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "ignore.md")); err == nil {
		t.Error("did not expect ignore.md")
	}
}

func TestImportCmd_CursorRoutes(t *testing.T) {
	dir := t.TempDir()
	writeMinimalConfig(t, dir, ".agnostic-ai")
	mdc := `---
description: routed
---

body
`
	writeFile(t, filepath.Join(dir, ".cursor", "rules", "routed.mdc"), mdc)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"import", "cursor"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "rules", "routed.md")); err != nil {
		t.Error("expected .agnostic-ai/rules/routed.md after import cursor")
	}
}

// Native cursor agents, skills (folder + bundled assets), and commands
// round-trip into the source dirs so sync can re-emit them everywhere.
func TestImportFromCursor_AgentsSkillsAndCommands(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".cursor", "agents", "reviewer.md"), "---\nname: reviewer\ndescription: d\n---\nreview\n")
	writeFile(t, filepath.Join(dir, ".cursor", "skills", "alpha", "SKILL.md"), "---\nname: alpha\ndescription: d\n---\nbody\n")
	writeFile(t, filepath.Join(dir, ".cursor", "skills", "alpha", "scripts", "run.sh"), "#!/bin/sh\n")
	writeFile(t, filepath.Join(dir, ".cursor", "commands", "deploy.md"), "---\ndescription: ship\n---\n\nRun it.\n")
	silence(t)

	if err := importFromCursor(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		filepath.Join(dir, "agents", "reviewer.md"),
		filepath.Join(dir, "skills", "alpha", "SKILL.md"),
		filepath.Join(dir, "skills", "alpha", "scripts", "run.sh"),
		filepath.Join(dir, "commands", "deploy.md"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s after import cursor: %v", p, err)
		}
	}
}

// Cursor's Skill directories table lists `.agents/skills/` and
// `.cursor/skills/` both as project-level, and both are discovered in
// nested subdirectories. Reading only `.cursor/skills` dropped every
// skill in a repo on the shared layout (#854).
func TestImportFromCursor_ImportsEveryProjectSkillPathWithPrecedence(t *testing.T) {
	dir := t.TempDir()
	for _, path := range cursorSkillsDirs {
		name := strings.TrimPrefix(filepath.Dir(path), ".")
		writeFile(t, filepath.Join(dir, path, name, "SKILL.md"),
			"---\nname: "+name+"\n---\n\n"+name+" body\n")
		writeFile(t, filepath.Join(dir, path, "shared", "SKILL.md"),
			"---\nname: shared\n---\n\nfrom "+name+"\n")
	}
	writeFile(t, filepath.Join(dir, "apps", "web", ".agents", "skills", "deploy-web", "SKILL.md"),
		"---\nname: deploy-web\n---\n\nnested body\n")
	silence(t)

	if err := importFromCursor(dir, rootSources()); err != nil {
		t.Fatal(err)
	}

	for _, path := range cursorSkillsDirs {
		name := strings.TrimPrefix(filepath.Dir(path), ".")
		got := readFile(t, filepath.Join(dir, "skills", name, "SKILL.md"))
		if !strings.Contains(got, name+" body") {
			t.Errorf("%s skill not imported:\n%s", path, got)
		}
	}
	nested := readFile(t, filepath.Join(dir, "skills", "apps", "web", "deploy-web", "SKILL.md"))
	if !strings.Contains(nested, "nested body") {
		t.Errorf("nested .agents/skills scope not preserved:\n%s", nested)
	}
	shared := readFile(t, filepath.Join(dir, "skills", "shared", "SKILL.md"))
	if !strings.Contains(shared, "from cursor") {
		t.Errorf(".cursor/skills should win a same-name collision:\n%s", shared)
	}
}

// Cursor emits a skill's name, description and its own five optional
// keys, so importing the emitted folder used to delete the rest of the
// spec. The imported keys and body win; the others stay, and bundled
// assets are still copied byte-for-byte.
func TestImportFromCursor_SkillImportKeepsSpecOnlyFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "skills", "alpha", "SKILL.md"),
		"---\nname: alpha\ndescription: old\nargument-hint: \"[n]\"\nallowed-tools: Read\n---\n\nold body\n")
	writeFile(t, filepath.Join(dir, ".cursor", "skills", "alpha", "SKILL.md"),
		"---\nname: alpha\ndescription: new\n---\n\nnew body\n")
	writeFile(t, filepath.Join(dir, ".cursor", "skills", "alpha", "scripts", "run.sh"), "#!/bin/sh\necho hi\n")
	silence(t)

	if err := importFromCursor(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	want := "---\nname: alpha\ndescription: new\nargument-hint: \"[n]\"\nallowed-tools: Read\n---\n\nnew body\n"
	if got := readFile(t, filepath.Join(dir, "skills", "alpha", "SKILL.md")); got != want {
		t.Errorf("skill spec:\ngot:\n%s\nwant:\n%s", got, want)
	}
	if got := readFile(t, filepath.Join(dir, "skills", "alpha", "scripts", "run.sh")); got != "#!/bin/sh\necho hi\n" {
		t.Errorf("bundled asset = %q, want the cursor bytes", got)
	}
}

// The same guarantee on the flat agent surface, which cursor emits
// without an agent's `effort` or `memory`.
func TestImportFromCursor_AgentImportKeepsSpecOnlyFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agents", "reviewer.md"),
		"---\nname: reviewer\ndescription: old\neffort: high\n---\n\nold body\n")
	writeFile(t, filepath.Join(dir, ".cursor", "agents", "reviewer.md"),
		"---\nname: reviewer\ndescription: new\n---\n\nnew body\n")
	silence(t)

	if err := importFromCursor(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	want := "---\nname: reviewer\ndescription: new\neffort: high\n---\n\nnew body\n"
	if got := readFile(t, filepath.Join(dir, "agents", "reviewer.md")); got != want {
		t.Errorf("agent spec:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
