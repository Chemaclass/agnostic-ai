package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestImportGoose_RoundTripFixedPoint emits a kit-sink bundle to goose,
// wipes the source specs, imports the emitted tree back, then re-emits.
// The second emit must byte-match the first: import reconstructs rules
// (from the inlined `## Rules` block in AGENTS.md), agents (from
// `.agents/agents/`), skills (from `.agents/skills/`), hooks (from the
// plugin's `hooks/hooks.json`), and reviews (from `.agents/REVIEW.md`).
func TestImportGoose_RoundTripFixedPoint(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [goose]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "# Project\n\nTop-level instructions.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r1.md"),
		"---\nname: r1\n---\n\nrule one body\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md"),
		"---\nname: reviewer\ndescription: Reviews code\nmodel: gpt-5.5\n---\n\nagent body\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "my-skill", "SKILL.md"),
		"---\nname: my-skill\ndescription: An example skill\n---\n\nSkill body here.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "edited.yaml"),
		"name: edited\nevent: PostToolUse\nmatcher: Edit\ncommand: echo edited\ntimeout: 12\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"),
		"name: guard\nevent: PreToolUse\nmatcher: Bash\ncommand: [\"echo a\", \"echo b\"]\nx-goose:\n  on_failure: block\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "reviews", "root.md"),
		"---\nname: root\n---\n\nCheck changed code for correctness.\n")

	execCLI(t, "sync", "-t", "goose")
	first := snapshotEmitted(t, dir)
	for _, want := range []string{
		"AGENTS.md",
		".agents/agents/reviewer.md",
		".agents/skills/my-skill/SKILL.md",
		".agents/plugins/agnostic-ai/plugin.json",
		".agents/plugins/agnostic-ai/hooks/hooks.json",
		".agents/REVIEW.md",
	} {
		if _, ok := first[want]; !ok {
			t.Fatalf("first emit produced no %s: %v", want, keys(first))
		}
	}

	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
		t.Fatalf("wipe source specs: %v", err)
	}
	execCLI(t, "import", "goose")

	rule := readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r1.md"))
	if !strings.Contains(rule, "rule one body") {
		t.Errorf("rule not reconstructed:\n%s", rule)
	}
	agent := readFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md"))
	for _, want := range []string{"description: Reviews code", "model: gpt-5.5", "agent body"} {
		if !strings.Contains(agent, want) {
			t.Errorf("agent lost %q:\n%s", want, agent)
		}
	}
	skill := readFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "my-skill", "SKILL.md"))
	if !strings.Contains(skill, "description: An example skill") || !strings.Contains(skill, "Skill body here.") {
		t.Errorf("skill not reconstructed:\n%s", skill)
	}
	review := readFile(t, filepath.Join(dir, ".agnostic-ai", "reviews", "review.md"))
	if !strings.Contains(review, "Check changed code for correctness.") {
		t.Errorf("review not reconstructed:\n%s", review)
	}
	hooks := readHookSpecs(t, filepath.Join(dir, ".agnostic-ai", "hooks"))
	if len(hooks) != 2 {
		t.Fatalf("expected 2 hook specs, got %d: %v", len(hooks), keys(hooks))
	}
	joined := strings.Join(sortedValues(hooks), "\n")
	for _, want := range []string{
		"event: PostToolUse", "matcher: Edit", "timeout: 12",
		"event: PreToolUse", "- echo a", "- echo b",
		"on_failure: block", "target: goose",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("imported hook specs missing %q:\n%s", want, joined)
		}
	}

	execCLI(t, "sync", "-t", "goose")
	assertEmittedEqual(t, first, snapshotEmitted(t, dir))
}

// TestImportGoose_HintsFileRoundTripsWhenOptedIn covers the
// `outputs.goose.rules-file` project: rules reach both AGENTS.md and
// `.goosehints`, and the pair must not import every rule twice.
func TestImportGoose_HintsFileRoundTripsWhenOptedIn(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"),
		"version: 1\ntargets: [goose]\noutputs:\n  goose:\n    rules-file: .goosehints\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r1.md"),
		"---\nname: r1\n---\n\nrule one body\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r2.md"),
		"---\nname: r2\n---\n\nrule two body\n")

	execCLI(t, "sync", "-t", "goose")
	first := snapshotEmitted(t, dir)
	if _, ok := first[".goosehints"]; !ok {
		t.Fatalf("first emit produced no hints file: %v", keys(first))
	}

	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
		t.Fatalf("wipe source specs: %v", err)
	}
	execCLI(t, "import", "goose")

	entries, err := os.ReadDir(filepath.Join(dir, ".agnostic-ai", "rules"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected 2 rules after import, got %d: %v", len(entries), names)
	}

	execCLI(t, "sync", "-t", "goose")
	assertEmittedEqual(t, first, snapshotEmitted(t, dir))
}

// TestImportGoose_HandAuthoredHintsWithoutAgentsFile covers a project
// that never synced: `.goosehints` is the only rules source, so the
// AGENTS.md-first read has to fall through to it.
func TestImportGoose_HandAuthoredHintsWithoutAgentsFile(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [goose]\n")
	writeFile(t, filepath.Join(dir, ".goosehints"), "## house-style\n\nBe terse.\n")

	execCLI(t, "import", "goose")

	got := readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "house-style.md"))
	if !strings.Contains(got, "Be terse.") {
		t.Errorf("hand-authored hints not imported:\n%s", got)
	}
}

// TestImportGoose_PluginSkillsImport covers the plugin's other
// component: a skills directory bundled under a plugin root is a skill
// Goose loads, so it has to import like any other.
func TestImportGoose_PluginSkillsImport(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [goose]\n")
	writeFile(t, filepath.Join(dir, ".agents", "plugins", "team", "skills", "review", "SKILL.md"),
		"---\nname: review\ndescription: Review changes\n---\n\nReview body.\n")

	execCLI(t, "import", "goose")

	got := readFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "review", "SKILL.md"))
	if !strings.Contains(got, "Review body.") {
		t.Errorf("plugin skill not imported:\n%s", got)
	}
}

// TestImportGoose_KnownSourceWiredIn guards the wiring: goose was an
// emit-only target with no import route at all (#894).
func TestImportGoose_KnownSourceWiredIn(t *testing.T) {
	if !isKnownImportSource("goose") {
		t.Error("goose should be a known import source")
	}
	sources := importSources()
	if !strings.Contains(sources, "goose") {
		t.Errorf("importSources() missing %q: %s", "goose", sources)
	}
}
