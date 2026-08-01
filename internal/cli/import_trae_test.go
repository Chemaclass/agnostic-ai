package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestImportTrae_RoundTripFixedPoint emits a bundle to trae, wipes the
// source specs, imports the emitted tree back, then re-emits. The
// second emit must byte-match the first: import reconstructs rules and
// agents (from `.trae/rules/`, reclassified by filename prefix), skills
// (from `.trae/skills/`, Trae's native SKILL.md folder tree), and
// commands (from `.trae/commands/`).
func TestImportTrae_RoundTripFixedPoint(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [trae]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r1.md"),
		"---\nname: r1\n---\n\nrule one body\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md"),
		"---\nname: reviewer\n---\n\nagent body\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "my-skill", "SKILL.md"),
		"---\nname: my-skill\ndescription: An example skill\n---\n\nSkill body here.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "commands", "deploy.md"),
		"---\nname: deploy\ndescription: Ship it\n---\n\nRun the deploy steps.\n")

	execCLI(t, "sync", "-t", "trae")
	first := snapshotEmitted(t, dir)
	if _, ok := first[".trae/skills/my-skill/SKILL.md"]; !ok {
		t.Fatalf("first emit produced no skill folder: %v", keys(first))
	}
	if _, ok := first[".trae/commands/deploy.md"]; !ok {
		t.Fatalf("first emit produced no command file: %v", keys(first))
	}
	if _, ok := first[".trae/rules/r1.md"]; !ok {
		t.Fatalf("first emit produced no rule file: %v", keys(first))
	}

	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
		t.Fatalf("wipe source specs: %v", err)
	}
	execCLI(t, "import", "trae")

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
	command := readFile(t, filepath.Join(dir, ".agnostic-ai", "commands", "deploy.md"))
	if !strings.Contains(command, "description: Ship it") || !strings.Contains(command, "Run the deploy steps.") {
		t.Errorf("command not reconstructed:\n%s", command)
	}

	execCLI(t, "sync", "-t", "trae")
	second := snapshotEmitted(t, dir)
	assertEmittedEqual(t, first, second)
}

// TestImportTrae_LegacyFlatSkillStillImports covers a project synced
// before skills moved to a native folder: a flat
// `.trae/rules/skill-<name>.md` still round-trips as a skill.
func TestImportTrae_LegacyFlatSkillStillImports(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [trae]\n")
	writeFile(t, filepath.Join(dir, ".trae", "rules", "skill-legacy.md"),
		"# Skill: legacy\n\nlegacy skill body\n")

	execCLI(t, "import", "trae")

	got := readFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "legacy.md"))
	if !strings.Contains(got, "legacy skill body") {
		t.Errorf("legacy flat skill not imported:\n%s", got)
	}
}

// TestImportTrae_KnownSourceWiredIn guards the wiring: trae moved off
// the generic rulesDirImporters map onto its own dedicated importer and
// must stay a known source.
func TestImportTrae_KnownSourceWiredIn(t *testing.T) {
	if !isKnownImportSource("trae") {
		t.Error("trae should be a known import source")
	}
	sources := importSources()
	if !strings.Contains(sources, "trae") {
		t.Errorf("importSources() missing %q: %s", "trae", sources)
	}
}
