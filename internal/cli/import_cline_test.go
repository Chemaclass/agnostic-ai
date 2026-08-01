package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestImportCline_RoundTripFixedPoint emits a bundle to cline, wipes the
// source specs, imports the emitted tree back, then re-emits. The
// second emit must byte-match the first: import reconstructs rules and
// agents (from `.clinerules/`, reclassified by filename prefix) and
// skills (from `.cline/skills/`, Cline's native SKILL.md folder tree).
func TestImportCline_RoundTripFixedPoint(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [cline]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r1.md"),
		"---\nname: r1\n---\n\nrule one body\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md"),
		"---\nname: reviewer\n---\n\nagent body\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "my-skill", "SKILL.md"),
		"---\nname: my-skill\ndescription: An example skill\n---\n\nSkill body here.\n")

	execCLI(t, "sync", "-t", "cline")
	first := snapshotEmitted(t, dir)
	if _, ok := first[".cline/skills/my-skill/SKILL.md"]; !ok {
		t.Fatalf("first emit produced no skill folder: %v", keys(first))
	}
	if _, ok := first[".clinerules/r1.md"]; !ok {
		t.Fatalf("first emit produced no rule file: %v", keys(first))
	}

	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
		t.Fatalf("wipe source specs: %v", err)
	}
	execCLI(t, "import", "cline")

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

	execCLI(t, "sync", "-t", "cline")
	second := snapshotEmitted(t, dir)
	assertEmittedEqual(t, first, second)
}

// TestImportCline_LegacyFlatSkillStillImports covers a project synced
// before skills moved to a native folder: a flat
// `.clinerules/skill-<name>.md` still round-trips as a skill.
func TestImportCline_LegacyFlatSkillStillImports(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [cline]\n")
	writeFile(t, filepath.Join(dir, ".clinerules", "skill-legacy.md"),
		"# Skill: legacy\n\nlegacy skill body\n")

	execCLI(t, "import", "cline")

	got := readFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "legacy.md"))
	if !strings.Contains(got, "legacy skill body") {
		t.Errorf("legacy flat skill not imported:\n%s", got)
	}
}

// TestImportCline_KnownSourceWiredIn guards the wiring: cline moved off
// the generic rulesDirImporters map onto its own dedicated importer and
// must stay a known source.
func TestImportCline_KnownSourceWiredIn(t *testing.T) {
	if !isKnownImportSource("cline") {
		t.Error("cline should be a known import source")
	}
	sources := importSources()
	if !strings.Contains(sources, "cline") {
		t.Errorf("importSources() missing %q: %s", "cline", sources)
	}
}
