package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestImportJunie_RoundTripFixedPoint emits a bundle to junie, wipes the
// source specs, imports the emitted tree back, then re-emits. The
// second emit must byte-match the first: import reconstructs rules and
// agents (from `.junie/rules/`, reclassified by filename prefix) and
// skills (from `.junie/skills/`, Junie's native SKILL.md folder tree,
// target-audit 2026-08-01). Before that fix landed, skills synced into
// `.junie/skills/` were invisible to `import junie`, which only ever
// scanned `.junie/rules/`.
func TestImportJunie_RoundTripFixedPoint(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [junie]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r1.md"),
		"---\nname: r1\n---\n\nrule one body\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md"),
		"---\nname: reviewer\n---\n\nagent body\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "my-skill", "SKILL.md"),
		"---\nname: my-skill\ndescription: An example skill\n---\n\nSkill body here.\n")

	execCLI(t, "sync", "-t", "junie")
	first := snapshotEmitted(t, dir)
	if _, ok := first[".junie/skills/my-skill/SKILL.md"]; !ok {
		t.Fatalf("first emit produced no skill folder: %v", keys(first))
	}
	if _, ok := first[".junie/rules/r1.md"]; !ok {
		t.Fatalf("first emit produced no rule file: %v", keys(first))
	}

	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
		t.Fatalf("wipe source specs: %v", err)
	}
	execCLI(t, "import", "junie")

	rule := readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r1.md"))
	if !strings.Contains(rule, "rule one body") {
		t.Errorf("rule not reconstructed:\n%s", rule)
	}
	agent := readFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md"))
	if !strings.Contains(agent, "agent body") {
		t.Errorf("agent not reconstructed:\n%s", agent)
	}
	skill := readFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "my-skill", "SKILL.md"))
	if !strings.Contains(skill, "description: An example skill") || !strings.Contains(skill, "Skill body here.") {
		t.Errorf("skill not reconstructed:\n%s", skill)
	}

	execCLI(t, "sync", "-t", "junie")
	second := snapshotEmitted(t, dir)
	assertEmittedEqual(t, first, second)
}

// TestImportJunie_LegacyFlatSkillStillImports covers a project synced
// before Native Agent Skills shipped for Junie (2026-07-31): a flat
// `.junie/rules/skill-<name>.md` still round-trips as a skill.
func TestImportJunie_LegacyFlatSkillStillImports(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [junie]\n")
	writeFile(t, filepath.Join(dir, ".junie", "rules", "skill-legacy.md"),
		"# Skill: legacy\n\nlegacy skill body\n")

	execCLI(t, "import", "junie")

	got := readFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "legacy.md"))
	if !strings.Contains(got, "legacy skill body") {
		t.Errorf("legacy flat skill not imported:\n%s", got)
	}
}

// TestImportJunie_KnownSourceWiredIn guards the wiring: junie moved off
// the generic rulesDirImporters map onto its own dedicated importer and
// must stay a known source.
func TestImportJunie_KnownSourceWiredIn(t *testing.T) {
	if !isKnownImportSource("junie") {
		t.Error("junie should be a known import source")
	}
	sources := importSources()
	if !strings.Contains(sources, "junie") {
		t.Errorf("importSources() missing %q: %s", "junie", sources)
	}
}
