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

// setupReferencesProject writes a project with one folder skill whose
// SKILL.md and nested document link to bundled reference files.
func setupReferencesProject(t *testing.T, targets string) string {
	t.Helper()
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	write := func(p, body string) { writeReferenceProjectFile(t, dir, p, body) }
	write("agnostic-ai.yaml", "version: 1\ntargets: ["+targets+"]\n")
	write(".agnostic-ai/skills/deploy/SKILL.md", "---\nname: deploy\ndescription: deploy\n---\n"+
		"Follow [setup](references/setup.md#install) and [the guide](<docs/nested guide.md>).\n"+
		"```\n[not a link](missing-in-code.md)\n```\n"+
		"See [site](https://example.com/missing.md).\n")
	write(".agnostic-ai/skills/deploy/references/setup.md", "setup\n")
	write(".agnostic-ai/skills/deploy/docs/nested guide.md", "Back to [setup][s].\n\n[s]: ../references/setup.md\n")
	return dir
}

// writeReferenceProjectFile writes a file under dir, creating parents.
func writeReferenceProjectFile(t *testing.T, dir, p, body string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
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
		".agnostic-ai/skills/deploy/SKILL.md:8 links to missing references/setup.md",
		".agnostic-ai/skills/deploy/docs/nested guide.md:3 links to missing ../references/setup.md",
		"targets: claude",
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
	if err == nil || !strings.Contains(codexOut, ".agnostic-ai/skills/deploy/SKILL.md:8 links to missing references/setup.md") ||
		!strings.Contains(codexOut, "targets: codex") {
		t.Errorf("codex run must report the broken reference (err=%v):\n%s", err, codexOut)
	}
}

// A broken link copied to several targets prints once, with every
// affected target listed on one line, instead of repeating per target
// (#1342).
func TestDoctorCheckReferences_GroupsFindingsBySourceAndListsTargets(t *testing.T) {
	dir := setupReferencesProject(t, "claude, codex")
	syncProject(t)
	if err := os.Remove(filepath.Join(dir, ".claude/skills/deploy/references/setup.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, ".agents/skills/deploy/references/setup.md")); err != nil {
		t.Fatal(err)
	}

	out, err := runDoctor(t, "--check-references")
	if err == nil {
		t.Fatalf("doctor should fail on a broken reference:\n%s", out)
	}
	if n := strings.Count(out, "links to missing references/setup.md"); n != 1 {
		t.Errorf("want one grouped line for the shared broken link, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "targets: claude, codex") {
		t.Errorf("want both targets listed on one line:\n%s", out)
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

// Continue's native skill folder and OpenCode's command form both keep
// every link to a bundled asset resolvable (#1043).
func TestDoctorCheckReferences_FlattenedAndNativeSkillsResolveBundledAssets(t *testing.T) {
	dir := setupReferencesProject(t, "continue, opencode")
	optIntoOpencodeSkillCommands(t, dir)
	syncProject(t)

	out, err := runDoctor(t, "--check-references")
	if err != nil {
		t.Fatalf("every bundled link must resolve: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".opencode/commands/skill-deploy.md")); err != nil {
		t.Fatalf("the command form must be checked too: %v", err)
	}
}

func TestDoctorCheckReferences_AttributesFlattenedSkillByRender(t *testing.T) {
	dir := setupReferencesProject(t, "opencode")
	optIntoOpencodeSkillCommands(t, dir)
	skill := filepath.Join(dir, ".agnostic-ai/skills/deploy/SKILL.md")
	if err := os.WriteFile(skill, []byte("---\nname: deploy\ndescription: deploy\n---\nRead [gone](references/gone.md).\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	syncProject(t)

	out, err := runDoctor(t, "--json", "--check-references")
	if err == nil {
		t.Fatalf("a link the skill does not bundle must be reported:\n%s", out)
	}
	for _, want := range []string{
		`"path": ".opencode/commands/skill-deploy.md"`,
		`"source": ".agnostic-ai/skills/deploy/SKILL.md"`,
		`"destination": "references/gone.md"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %s:\n%s", want, out)
		}
	}
}

// optIntoOpencodeSkillCommands turns on OpenCode's flattened skill
// command form in the project config setupReferencesProject wrote.
func optIntoOpencodeSkillCommands(t *testing.T, dir string) {
	t.Helper()
	cfg := filepath.Join(dir, "agnostic-ai.yaml")
	data, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, "outputs:\n  opencode:\n    emit-skills-as-commands: true\n"...)
	if err := os.WriteFile(cfg, data, 0o644); err != nil {
		t.Fatal(err)
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

// A skill commonly links a project file by its repo-relative path, such
// as `apps/engine/src/lib.ts`; the emitted copy sits next to the skill
// document instead, so the link never resolves there. Resolving from the
// project root as a fallback is the smaller change: it needs no new
// config key, and every project already has exactly one root (#1342).
func TestDoctorCheckReferences_ResolvesLinkFromProjectRoot(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeReferenceProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeReferenceProjectFile(t, dir, "src/lib.ts", "export {}\n")
	writeReferenceProjectFile(t, dir, ".agnostic-ai/skills/demo/SKILL.md",
		"---\nname: demo\ndescription: demo\n---\nSee [lib](src/lib.ts).\n")
	syncProject(t)

	out, err := runDoctor(t, "--check-references")
	if err != nil {
		t.Fatalf("a link resolving from the project root must pass: %v\n%s", err, out)
	}
}

// A link that resolves from neither the emitted document nor the project
// root still fails, even though it would match a same-named file at the
// root under a different subpath: only the exact repo-relative path
// counts.
func TestDoctorCheckReferences_ProjectRootFallbackStillFlagsUnresolvedLinks(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeReferenceProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeReferenceProjectFile(t, dir, ".agnostic-ai/skills/demo/SKILL.md",
		"---\nname: demo\ndescription: demo\n---\nSee [lib](src/lib.ts).\n")
	syncProject(t)

	out, err := runDoctor(t, "--check-references")
	if err == nil || !strings.Contains(out, "links to missing src/lib.ts") {
		t.Fatalf("a link missing everywhere must still be flagged (err=%v):\n%s", err, out)
	}
}

// The project-root fallback never looks outside the project: a `../`
// link stays broken even when a file of that name sits beside the
// checkout.
func TestDoctorCheckReferences_ProjectRootFallbackStaysInsideTheProject(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "repo")
	writeReferenceProjectFile(t, parent, "shared/notes.md", "outside\n")
	testutil.Chdir(t, mkdirAll(t, dir))
	silence(t)
	writeReferenceProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeReferenceProjectFile(t, dir, ".agnostic-ai/skills/demo/SKILL.md",
		"---\nname: demo\ndescription: demo\n---\nSee [notes](../shared/notes.md).\n")
	syncProject(t)

	out, err := runDoctor(t, "--check-references")
	if err == nil || !strings.Contains(out, "links to missing ../shared/notes.md") {
		t.Fatalf("a link leaving the project must stay flagged (err=%v):\n%s", err, out)
	}
}

// An ignore entry matches the destination as written, so a
// percent-encoded link is exempted by the text copied from the source.
func TestDoctorCheckReferences_IgnoreMatchesTheDestinationAsWritten(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeReferenceProjectFile(t, dir, "agnostic-ai.yaml",
		"version: 1\ntargets: [claude]\ndoctor:\n  check-references:\n    ignore: [\"gcp%2Durl\"]\n")
	writeReferenceProjectFile(t, dir, ".agnostic-ai/skills/demo/SKILL.md",
		"---\nname: demo\ndescription: demo\n---\nSee [GCP](gcp%2Durl).\n")
	syncProject(t)

	out, err := runDoctor(t, "--check-references")
	if err != nil {
		t.Fatalf("an ignore entry copied from the source must match: %v\n%s", err, out)
	}
}

// doctor.check-references.ignore exempts placeholder links, such as
// `url` in an example template, that can never resolve to a real file
// (#1342).
func TestDoctorCheckReferences_IgnoreListExemptsPlaceholderLinks(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeReferenceProjectFile(t, dir, "agnostic-ai.yaml",
		"version: 1\ntargets: [claude]\ndoctor:\n  check-references:\n    ignore: [\"url\"]\n")
	writeReferenceProjectFile(t, dir, ".agnostic-ai/skills/demo/SKILL.md",
		"---\nname: demo\ndescription: demo\n---\nSee [Logs](url).\n")
	syncProject(t)

	out, err := runDoctor(t, "--check-references")
	if err != nil {
		t.Fatalf("an ignored destination must not fail the check: %v\n%s", err, out)
	}
}

// Without the ignore entry, the same placeholder link fails, proving the
// previous test's pass comes from the config key and not from the link
// happening to resolve.
func TestDoctorCheckReferences_PlaceholderLinkFlaggedWithoutIgnoreList(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeReferenceProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeReferenceProjectFile(t, dir, ".agnostic-ai/skills/demo/SKILL.md",
		"---\nname: demo\ndescription: demo\n---\nSee [Logs](url).\n")
	syncProject(t)

	out, err := runDoctor(t, "--check-references")
	if err == nil || !strings.Contains(out, "links to missing url") {
		t.Fatalf("without the ignore entry the placeholder must still fail (err=%v):\n%s", err, out)
	}
}

// The exact reproduction from #1342: a repo-root link and an ignored
// placeholder link together must reach exit 0.
func TestDoctorCheckReferences_IssueRepro(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeReferenceProjectFile(t, dir, "agnostic-ai.yaml",
		"version: 1\ntargets: [claude]\ndoctor:\n  check-references:\n    ignore: [\"url\"]\n")
	writeReferenceProjectFile(t, dir, "src/lib.ts", "export {}\n")
	writeReferenceProjectFile(t, dir, ".agnostic-ai/skills/demo/SKILL.md",
		"---\nname: demo\ndescription: Demo.\n---\n\nSee [lib](src/lib.ts) and [Logs](url).\n")
	syncProject(t)

	out, err := runDoctor(t, "--check-references")
	if err != nil {
		t.Fatalf("doctor --check-references: %v\n%s", err, out)
	}
}

func mkdirAll(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}
