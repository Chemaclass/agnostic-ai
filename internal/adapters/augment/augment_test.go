package augment

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
	if got := New().Name(); got != "augment" {
		t.Errorf("Name() = %q, want %q", got, "augment")
	}
}

// The project-root AGENTS.md (with rule bodies inlined) is written
// centrally by sync; this adapter never writes it, and without the
// rules-file opt-in it writes nothing at all.
func TestEmit_NoFilesByDefault(t *testing.T) {
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
	if _, err := os.Stat(filepath.Join(dir, ".augment-guidelines")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write .augment-guidelines without the rules-file opt-in, err=%v", err)
	}
}

// outputs.augment.rules-file opts into the legacy concatenated
// `.augment-guidelines`-style document.
func TestEmit_RulesFile_WritesConcatenatedRules(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"augment": {RulesFile: ".augment-guidelines"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "security", Path: "rules/security.md", Body: "Never expose secrets."},
		{Kind: spec.KindRule, Name: "commits", Path: "rules/commits.md", Body: "Use conventional commits."},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment-guidelines"))
	for _, want := range []string{"Never expose secrets.", "Use conventional commits."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// An agent or skill spec targeted at augment has no native surface, so
// it must not leak into the opt-in rules document: only rule bodies
// belong there.
func TestEmit_RulesFile_ExcludesAgentsAndSkills(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"augment": {RulesFile: ".augment-guidelines"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
		{Kind: spec.KindAgent, Name: "helper", Path: "agents/helper.md", Body: "agent body should not appear"},
		{Kind: spec.KindSkill, Name: "sk1", Path: "skills/sk1/SKILL.md", Body: "skill body should not appear"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment-guidelines"))
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
		Outputs: map[string]config.Output{"augment": {RulesFile: ".augment-guidelines"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".augment-guidelines")); !os.IsNotExist(err) {
		t.Errorf("expected no .augment-guidelines for a bundle with no rules, err=%v", err)
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
