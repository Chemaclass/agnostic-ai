package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestLocalMarkdownLinks_FindsInlineAndReferenceLinks(t *testing.T) {
	doc := strings.Join([]string{
		"---",
		"name: deploy",
		"description: see [x](frontmatter.md)",
		"---",
		"Read [setup](references/setup.md) first.",
		"Then ![diagram](img/flow.png \"Flow\") and [spaced](<my notes.md>).",
		"Encoded [enc](my%20file.md) and [frag](guide.md#install).",
		"[label]: refs/label.md \"Title\"",
		"  [angle]: <refs/angle file.md>",
		"[^note]: footnote text",
	}, "\n")

	got := localMarkdownLinks(doc)
	want := []markdownLink{
		{Line: 5, Dest: "references/setup.md"},
		{Line: 6, Dest: "img/flow.png"},
		{Line: 6, Dest: "my notes.md"},
		{Line: 7, Dest: "my file.md"},
		{Line: 7, Dest: "guide.md"},
		{Line: 8, Dest: "refs/label.md"},
		{Line: 9, Dest: "refs/angle file.md"},
	}
	assertLinks(t, got, want)
}

func TestLocalMarkdownLinks_IgnoresCodeAndNonLocalDestinations(t *testing.T) {
	doc := strings.Join([]string{
		"Inline `[code](inline.md)` span and ``[double](double.md)``.",
		"```md",
		"[fenced](fenced.md)",
		"```",
		"~~~~",
		"[tilde](tilde.md)",
		"```",
		"[still fenced](tilde2.md)",
		"~~~~",
		"",
		"    [indented](indented.md)",
		"",
		"[web](https://example.com/x.md) [mail](mailto:a@b.c)",
		"[proto](//cdn.example.com/x.md) [abs](/etc/passwd)",
		"[frag](#section) [empty]() [query](?x=1)",
		"[ref]: https://example.com/ref.md",
		"[kept](kept.md)",
	}, "\n")

	assertLinks(t, localMarkdownLinks(doc), []markdownLink{{Line: 17, Dest: "kept.md"}})
}

func TestLocalMarkdownLinks_ChecksIndentedLinesInsideLists(t *testing.T) {
	doc := "- step one\n\n    see [nested](nested.md)\n"

	assertLinks(t, localMarkdownLinks(doc), []markdownLink{{Line: 3, Dest: "nested.md"}})
}

func TestLocalMarkdownLinks_KeepsParenthesesBalancedInDestination(t *testing.T) {
	doc := "See [v](docs/guide(v2).md) now.\n"

	assertLinks(t, localMarkdownLinks(doc), []markdownLink{{Line: 1, Dest: "docs/guide(v2).md"}})
}

func assertLinks(t *testing.T, got, want []markdownLink) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d links %+v, want %d %+v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i].Line != want[i].Line || got[i].Dest != want[i].Dest {
			t.Errorf("link %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// setupReferencesProject writes a project with one folder skill whose
// SKILL.md and nested document link to bundled reference files.
func setupReferencesProject(t *testing.T, targets string) string {
	t.Helper()
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	write := func(p, body string) {
		t.Helper()
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("agnostic-ai.yaml", "version: 1\ntargets: ["+targets+"]\n")
	write(".agnostic-ai/skills/deploy/SKILL.md", "---\nname: deploy\ndescription: deploy\n---\n"+
		"Follow [setup](references/setup.md#install) and [the guide](<docs/nested guide.md>).\n"+
		"```\n[not a link](missing-in-code.md)\n```\n"+
		"See [site](https://example.com/missing.md).\n")
	write(".agnostic-ai/skills/deploy/references/setup.md", "setup\n")
	write(".agnostic-ai/skills/deploy/docs/nested guide.md", "Back to [setup][s].\n\n[s]: ../references/setup.md\n")
	return dir
}

func runDoctor(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"doctor"}, args...))
	err := root.Execute()
	return out.String(), err
}

func syncProject(t *testing.T) {
	t.Helper()
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync: %v", err)
	}
}

func TestDoctorCheckReferences_PassesWhenCopiedReferencesResolve(t *testing.T) {
	setupReferencesProject(t, "claude")
	syncProject(t)

	out, err := runDoctor(t, "--check-references")
	if err != nil {
		t.Fatalf("doctor --check-references: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Skill references:") || !strings.Contains(out, "every local link") {
		t.Errorf("missing clean references section:\n%s", out)
	}
}

func TestDoctorCheckReferences_ReportsBrokenEmittedReferenceUntilRestored(t *testing.T) {
	dir := setupReferencesProject(t, "claude")
	syncProject(t)
	emitted := filepath.Join(dir, ".claude/skills/deploy/references/setup.md")
	saved, err := os.ReadFile(emitted)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(emitted); err != nil {
		t.Fatal(err)
	}

	out, err := runDoctor(t, "--check-references")
	if err == nil {
		t.Fatalf("doctor should fail on a broken reference:\n%s", out)
	}
	for _, want := range []string{
		"claude: .claude/skills/deploy/SKILL.md:8 links to missing references/setup.md",
		"source: .agnostic-ai/skills/deploy/SKILL.md",
		"claude: .claude/skills/deploy/docs/nested guide.md:3 links to missing ../references/setup.md",
		"source: .agnostic-ai/skills/deploy/docs/nested guide.md",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "missing-in-code.md") || strings.Contains(out, "example.com") {
		t.Errorf("code and external links must be ignored:\n%s", out)
	}

	if err := os.WriteFile(emitted, saved, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = runDoctor(t, "--check-references")
	if err != nil {
		t.Fatalf("finding should disappear once restored: %v\n%s", err, out)
	}
}

func TestDoctorCheckReferences_FailsOnReferenceMissingEverywhereWithoutDrift(t *testing.T) {
	dir := setupReferencesProject(t, "claude")
	skill := filepath.Join(dir, ".agnostic-ai/skills/deploy/SKILL.md")
	if err := os.WriteFile(skill, []byte("---\nname: deploy\ndescription: deploy\n---\nRead [gone](references/gone.md).\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	syncProject(t)

	if out, err := runDoctor(t); err != nil {
		t.Fatalf("plain doctor must stay clean without the flag: %v\n%s", err, out)
	}
	out, err := runDoctor(t, "--check-references")
	if err == nil || !strings.Contains(err.Error(), "1 broken skill reference") {
		t.Fatalf("want broken reference error, got %v\n%s", err, out)
	}
}

func TestDoctorCheckReferences_HonorsSelectedTargets(t *testing.T) {
	dir := setupReferencesProject(t, "claude, codex")
	syncProject(t)
	if err := os.Remove(filepath.Join(dir, ".agents/skills/deploy/references/setup.md")); err != nil {
		t.Fatal(err)
	}
	// Drift is not the subject here: the codex copy is missing, so only
	// the reference section is compared across the two runs.
	claudeOut, _ := runDoctor(t, "-t", "claude", "--check-references")
	if strings.Contains(claudeOut, "links to missing") {
		t.Errorf("claude run must not report codex findings:\n%s", claudeOut)
	}
	codexOut, err := runDoctor(t, "-t", "codex", "--check-references")
	if err == nil || !strings.Contains(codexOut, "codex: .agents/skills/deploy/SKILL.md:8 links to missing references/setup.md") {
		t.Errorf("codex run must report the broken reference (err=%v):\n%s", err, codexOut)
	}
}

func TestDoctorCheckReferences_JSONAddsReferencesAndWritesNothing(t *testing.T) {
	dir := setupReferencesProject(t, "claude")
	syncProject(t)
	if err := os.Remove(filepath.Join(dir, ".claude/skills/deploy/references/setup.md")); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, dir)
	state, err := os.ReadFile(filepath.Join(dir, ".agnostic-ai", ".sync-state"))
	if err != nil {
		t.Fatal(err)
	}

	out, err := runDoctor(t, "--json", "--check-references")
	if err == nil {
		t.Fatal("doctor --json --check-references should fail on findings")
	}
	var got struct {
		Command    string            `json:"command"`
		Writes     []json.RawMessage `json:"writes"`
		References []struct {
			Target      string `json:"target"`
			Source      string `json:"source"`
			Path        string `json:"path"`
			Line        int    `json:"line"`
			Destination string `json:"destination"`
		} `json:"references"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, out)
	}
	if got.Command != "doctor" || len(got.References) != 2 {
		t.Fatalf("want 2 reference findings, got %+v\n%s", got, out)
	}
	first := got.References[0]
	if first.Target != "claude" || first.Path != ".claude/skills/deploy/SKILL.md" || first.Line != 8 ||
		first.Destination != "references/setup.md" || first.Source != ".agnostic-ai/skills/deploy/SKILL.md" {
		t.Errorf("unexpected first finding: %+v", first)
	}
	if after := snapshotTree(t, dir); !reflect.DeepEqual(after, before) {
		t.Errorf("doctor --check-references changed the tree")
	}
	if after, _ := os.ReadFile(filepath.Join(dir, ".agnostic-ai", ".sync-state")); string(after) != string(state) {
		t.Errorf("doctor --check-references rewrote the sync state")
	}
}

func TestDoctorCheckReferences_AttributesFlattenedSkillWithoutBundledAssets(t *testing.T) {
	setupReferencesProject(t, "continue")
	syncProject(t)

	out, err := runDoctor(t, "--json", "--check-references")
	if err == nil {
		t.Fatalf("continue drops bundled assets, so the link must be reported:\n%s", out)
	}
	for _, want := range []string{
		`"path": ".continue/rules/skill-deploy.md"`,
		`"source": ".agnostic-ai/skills/deploy/SKILL.md"`,
		`"destination": "references/setup.md"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %s:\n%s", want, out)
		}
	}
}

func TestDoctorJSON_OmitsReferencesWithoutFlag(t *testing.T) {
	setupReferencesProject(t, "claude")
	syncProject(t)

	out, err := runDoctor(t, "--json")
	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, out)
	}
	if strings.Contains(out, "references") {
		t.Errorf("references key must stay absent without the flag:\n%s", out)
	}
	out, err = runDoctor(t, "--json", "--check-references")
	if err != nil {
		t.Fatalf("doctor --json --check-references: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"references": []`) {
		t.Errorf("clean run must emit an empty references list:\n%s", out)
	}
}
