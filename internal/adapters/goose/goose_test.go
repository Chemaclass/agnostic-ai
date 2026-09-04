package goose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestName(t *testing.T) {
	if got := New().Name(); got != "goose" {
		t.Errorf("Name() = %q, want %q", got, "goose")
	}
}

// The project-root AGENTS.md (with rule bodies inlined) is written
// centrally by sync; this adapter never writes it, and without the
// rules-file opt-in a rule-only bundle produces no rules output at
// all.
func TestEmit_NoRulesFileByDefault(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write AGENTS.md, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".goosehints")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write .goosehints without the rules-file opt-in, err=%v", err)
	}
}

// Skills write to the shared `.agents/skills/` tree unconditionally,
// unlike rules: no rules-file opt-in is needed (#632).
func TestEmit_Skill_WritesSkillFolderWithoutRulesFileOptIn(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "release-checklist", Meta: map[string]any{"description": "Prepare a release."}, Body: "Check the changelog."},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/skills/release-checklist/SKILL.md"))
	for _, want := range []string{"name: release-checklist", "description: Prepare a release.", "Check the changelog."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestEmit_SkillsDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{Outputs: map[string]config.Output{"goose": {SkillsDir: "custom/skills"}}}
	entries := []spec.Entry{{Kind: spec.KindSkill, Name: "s1", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/skills/s1/SKILL.md")); err != nil {
		t.Errorf("expected override dir to hold the skill file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents/skills/s1/SKILL.md")); !os.IsNotExist(err) {
		t.Errorf("expected no output at the default skills dir, err=%v", err)
	}
}

// outputs.goose.rules-file opts into the legacy concatenated
// `.goosehints`-style document.
func TestEmit_RulesFile_WritesConcatenatedRules(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"goose": {RulesFile: ".goosehints"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "security", Path: "rules/security.md", Body: "Never expose secrets."},
		{Kind: spec.KindRule, Name: "commits", Path: "rules/commits.md", Body: "Use conventional commits."},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".goosehints"))
	for _, want := range []string{"Never expose secrets.", "Use conventional commits."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// An agent spec has no native goose surface, and a skill spec routes
// to its own native folder (see TestEmit_Skill_WritesSkillFolderWithoutRulesFileOptIn),
// so neither belongs in the opt-in rules document: only rule bodies do.
func TestEmit_RulesFile_ExcludesAgentsAndSkills(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"goose": {RulesFile: ".goosehints"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
		{Kind: spec.KindAgent, Name: "helper", Path: "agents/helper.md", Body: "agent body should not appear"},
		{Kind: spec.KindSkill, Name: "sk1", Path: "skills/sk1/SKILL.md", Body: "skill body should not appear"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".goosehints"))
	if !strings.Contains(got, "rule body") {
		t.Errorf("missing rule body in %s", got)
	}
	for _, absent := range []string{"agent body should not appear", "skill body should not appear"} {
		if strings.Contains(got, absent) {
			t.Errorf("unexpected %q in %s", absent, got)
		}
	}
}

func TestEmit_EmptyBundle_WritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)

	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected an empty directory, got %v", entries)
	}
}

// The rules-file opt-in is itself a no-op when the bundle carries no
// rules, agents, or skills: MergedDocument short-circuits before
// writing.
func TestEmit_RulesFile_NoRulesWritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"goose": {RulesFile: ".goosehints"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".goosehints")); !os.IsNotExist(err) {
		t.Errorf("expected no .goosehints for a bundle with no rules, err=%v", err)
	}
}

// A rule carrying a source-layout or frontmatter scope routes into a
// nested `<scope>/.goosehints` file instead of flattening into the root
// document: Goose discovers additional hint files by name as it walks
// into subdirectories (see the package doc, #608).
func TestEmit_RulesFile_ScopedRuleWritesNestedGoosehints(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"goose": {RulesFile: ".goosehints"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "root-rule", Path: "rules/root-rule.md", Body: "root body"},
		{Kind: spec.KindRule, Name: "auth", Scope: "backend/api", Body: "scoped body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}

	root := readFile(t, filepath.Join(dir, ".goosehints"))
	if !strings.Contains(root, "root body") {
		t.Errorf("missing root body in root .goosehints:\n%s", root)
	}
	if strings.Contains(root, "scoped body") {
		t.Errorf("scoped rule leaked into root .goosehints:\n%s", root)
	}

	scoped := readFile(t, filepath.Join(dir, "backend/api/.goosehints"))
	if !strings.Contains(scoped, "scoped body") {
		t.Errorf("missing scoped body in backend/api/.goosehints:\n%s", scoped)
	}
	if strings.Contains(scoped, "root body") {
		t.Errorf("root rule leaked into backend/api/.goosehints:\n%s", scoped)
	}
}

// Rules sharing the same scope concatenate into that scope's single
// nested file, the same "one file per scope" shape the root document
// already used for root-scoped rules.
func TestEmit_RulesFile_SameScopeRulesConcatenateIntoOneFile(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"goose": {RulesFile: ".goosehints"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "auth", Scope: "backend", Body: "auth rule"},
		{Kind: spec.KindRule, Name: "db", Scope: "backend", Body: "db rule"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "backend/.goosehints"))
	for _, want := range []string{"auth rule", "db rule"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in backend/.goosehints:\n%s", want, got)
		}
	}
}

// A scoped rule still requires the rules-file opt-in: the fix routes
// scope within the existing opt-in document, it does not turn on a new
// default write (adapter-pattern: never write a surprise file).
func TestEmit_ScopedRuleWithoutRulesFileOptInWritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "auth", Scope: "backend", Body: "scoped body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "backend/.goosehints")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write backend/.goosehints without the rules-file opt-in, err=%v", err)
	}
	entries2, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries2) != 0 {
		t.Errorf("expected an empty directory, got %v", entries2)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
