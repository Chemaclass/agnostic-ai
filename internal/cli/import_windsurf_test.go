package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestImportWindsurf_RoundTripFixedPoint emits a bundle to windsurf,
// wipes the source specs, imports the emitted tree back, then
// re-emits. The second emit must byte-match the first: import
// reconstructs rules and agents (from `.devin/rules/`, reclassified by
// filename prefix) and skills (from `.agents/skills/`, the shared
// cross-tool SKILL.md folder tree Devin Desktop scans).
func TestImportWindsurf_RoundTripFixedPoint(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [windsurf]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r1.md"),
		"---\nname: r1\n---\n\nrule one body\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md"),
		"---\nname: reviewer\n---\n\nagent body\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "my-skill", "SKILL.md"),
		"---\nname: my-skill\ndescription: An example skill\n---\n\nSkill body here.\n")

	execCLI(t, "sync", "-t", "windsurf")
	first := snapshotEmitted(t, dir)
	if _, ok := first[".agents/skills/my-skill/SKILL.md"]; !ok {
		t.Fatalf("first emit produced no skill folder: %v", keys(first))
	}
	if _, ok := first[".devin/rules/r1.md"]; !ok {
		t.Fatalf("first emit produced no rule file: %v", keys(first))
	}

	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
		t.Fatalf("wipe source specs: %v", err)
	}
	execCLI(t, "import", "windsurf")

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

	execCLI(t, "sync", "-t", "windsurf")
	second := snapshotEmitted(t, dir)
	assertEmittedEqual(t, first, second)
}

// TestImportWindsurf_LegacyFlatSkillStillImports covers a project
// synced before skills moved to the shared `.agents/skills/` tree: a
// flat `.devin/rules/skill-<name>.md` still round-trips as a skill.
func TestImportWindsurf_LegacyFlatSkillStillImports(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [windsurf]\n")
	writeFile(t, filepath.Join(dir, ".devin", "rules", "skill-legacy.md"),
		"# Skill: legacy\n\nlegacy skill body\n")

	execCLI(t, "import", "windsurf")

	got := readFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "legacy.md"))
	if !strings.Contains(got, "legacy skill body") {
		t.Errorf("legacy flat skill not imported:\n%s", got)
	}
}

// TestImportWindsurf_KnownSourceWiredIn guards the wiring: windsurf
// moved off the inline windsurfImportDir switch in runImport onto its
// own dedicated importer and must stay a known source.
func TestImportWindsurf_KnownSourceWiredIn(t *testing.T) {
	if !isKnownImportSource("windsurf") {
		t.Error("windsurf should be a known import source")
	}
	sources := importSources()
	if !strings.Contains(sources, "windsurf") {
		t.Errorf("importSources() missing %q: %s", "windsurf", sources)
	}
}
