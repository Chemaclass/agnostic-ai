package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Entry is a local alias so the test file can name the spec entry type
// without colliding with the cli package's own types. Keeps the round-
// trip assertion below readable.
type Entry = spec.Entry

func TestFenceDivergent_Identical(t *testing.T) {
	body := "Same body.\n"
	got := fenceDivergent(body, body, "claude", "codex")
	if got != body {
		t.Errorf("expected unchanged body, got %q", got)
	}
}

func TestFenceDivergent_OnlyClaude(t *testing.T) {
	got := fenceDivergent("claude content", "", "claude", "codex")
	want := "::target claude\nclaude content\n::end\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestFenceDivergent_OnlyCodex(t *testing.T) {
	got := fenceDivergent("", "codex content", "claude", "codex")
	want := "::target codex\ncodex content\n::end\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestFenceDivergent_SharedPrefixAndSuffix(t *testing.T) {
	claude := "# Skill\n\nShared intro.\n\n## Workflow\n1. claude step\n2. claude step 2\n\nShared outro."
	codex := "# Skill\n\nShared intro.\n\n## Workflow\n1. codex step\n\nShared outro."
	got := fenceDivergent(claude, codex, "claude", "codex")
	if !strings.Contains(got, "# Skill") {
		t.Errorf("lost common prefix:\n%s", got)
	}
	if !strings.Contains(got, "## Workflow") {
		t.Errorf("lost common middle (workflow heading):\n%s", got)
	}
	if !strings.Contains(got, "Shared outro.") {
		t.Errorf("lost common suffix:\n%s", got)
	}
	if !strings.Contains(got, "::target claude\n1. claude step\n2. claude step 2\n::end") {
		t.Errorf("missing claude fence:\n%s", got)
	}
	if !strings.Contains(got, "::target codex\n1. codex step\n::end") {
		t.Errorf("missing codex fence:\n%s", got)
	}
}

func TestFenceDivergent_TotalDivergence(t *testing.T) {
	got := fenceDivergent("Alpha.", "Beta.", "claude", "codex")
	if !strings.Contains(got, "::target claude\nAlpha.\n::end") {
		t.Errorf("missing claude fence:\n%s", got)
	}
	if !strings.Contains(got, "::target codex\nBeta.\n::end") {
		t.Errorf("missing codex fence:\n%s", got)
	}
	if strings.Contains(got, "Alpha.\nBeta.") {
		t.Errorf("bodies should be in separate fences:\n%s", got)
	}
}

// renderBodyForTarget keeps every blank line outside a fence, so any
// stitching padding the auto-fencer adds between fences leaks into the
// emit as a stray blank between sections (#306). Stack the fences and
// confirm rendering for each target reads as if the fences were never
// there.
func TestFenceDivergent_TightStitch_NoStrayBlankOnRender(t *testing.T) {
	claudeBody := "## Header\nshared lead.\n\n## Claude Only\nclaude detail.\n\n## Footer\n... shared tail ...\n"
	codexBody := "## Header\nshared lead.\n\n## Codex Only\ncodex detail.\n\n## Footer\n... shared tail ...\n"
	merged := fenceDivergent(claudeBody, codexBody, "claude", "codex")
	e := Entry{Body: merged}
	if got := e.BodyFor("claude"); got != claudeBody {
		t.Errorf("claude round-trip drift:\ngot:  %q\nwant: %q", got, claudeBody)
	}
	if got := e.BodyFor("codex"); got != codexBody {
		t.Errorf("codex round-trip drift:\ngot:  %q\nwant: %q", got, codexBody)
	}
}

func TestMergeSkillBodies_IdenticalNoOp(t *testing.T) {
	dir := t.TempDir()
	body := "---\nname: test\n---\nSame body.\n"
	claudePath := filepath.Join(dir, "claude.md")
	codexPath := filepath.Join(dir, "codex.md")
	writeFile(t, claudePath, body)
	writeFile(t, codexPath, body)
	if err := mergeSkillBodies(claudePath, codexPath); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, claudePath)
	if got != body {
		t.Errorf("identical bodies should not be rewritten:\ngot %q\nwant %q", got, body)
	}
}

func TestMergeSkillBodies_DivergentRewrites(t *testing.T) {
	dir := t.TempDir()
	claudePath := filepath.Join(dir, "claude.md")
	codexPath := filepath.Join(dir, "codex.md")
	writeFile(t, claudePath,
		"---\nname: test\ndescription: x\n---\n# Skill\n\nIntro.\n\n## Steps\n1. claude only\n\nOutro.\n")
	writeFile(t, codexPath,
		"---\nname: test\n---\n# Skill\n\nIntro.\n\n## Steps\n1. codex only\n\nOutro.\n")
	if err := mergeSkillBodies(claudePath, codexPath); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, claudePath)
	if !strings.Contains(got, "description: x") {
		t.Errorf("claude frontmatter must survive:\n%s", got)
	}
	if !strings.Contains(got, "::target claude\n1. claude only\n::end") {
		t.Errorf("missing claude fence:\n%s", got)
	}
	if !strings.Contains(got, "::target codex\n1. codex only\n::end") {
		t.Errorf("missing codex fence:\n%s", got)
	}
}

func TestImportFromCodex_SkillBodyDiverges_AddsFences(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude", "skills", "test", "SKILL.md"),
		"---\nname: test\ndescription: from claude\n---\n# Test\n\nShared intro.\n\n## Scope\n- claude only\n\nShared outro.\n")
	writeFile(t, filepath.Join(dir, ".codex", "skills", "test", "SKILL.md"),
		"---\nname: test\n---\n# Test\n\nShared intro.\n\n## Scope\n- codex only\n\nShared outro.\n")

	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if err := importFromCodex(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "skills", "test", "SKILL.md"))
	if !strings.Contains(got, "description: from claude") {
		t.Errorf("claude frontmatter must win:\n%s", got)
	}
	if !strings.Contains(got, "::target claude\n- claude only\n::end") {
		t.Errorf("missing claude fence:\n%s", got)
	}
	if !strings.Contains(got, "::target codex\n- codex only\n::end") {
		t.Errorf("missing codex fence:\n%s", got)
	}
	if !strings.Contains(got, "Shared intro.") || !strings.Contains(got, "Shared outro.") {
		t.Errorf("shared sections lost:\n%s", got)
	}
}

func TestImportFromCodex_AgentBodyDiverges_AddsFences(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude", "agents", "explorer.md"),
		"---\nname: explorer\ndescription: from claude\n---\nShared intro.\n\nclaude step.\n\nShared outro.\n")
	writeFile(t, filepath.Join(dir, ".codex", "agents", "explorer.toml"),
		`name = "explorer"`+"\n"+
			`description = "from codex"`+"\n"+
			`developer_instructions = """Shared intro.

codex step.

Shared outro."""`+"\n")

	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if err := importFromCodex(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "agents", "explorer.md"))
	if !strings.Contains(got, "description: from claude") {
		t.Errorf("claude frontmatter must win:\n%s", got)
	}
	if !strings.Contains(got, "::target claude\nclaude step.\n::end") {
		t.Errorf("missing claude fence:\n%s", got)
	}
	if !strings.Contains(got, "::target codex\ncodex step.\n::end") {
		t.Errorf("missing codex fence:\n%s", got)
	}
}

// Divergent frontmatter description across the two tools is recorded
// under `x-codex.description` so each emit reproduces its source-of-
// truth (#304). Without this routing, the codex side silently inherited
// the claude description on every re-sync.
func TestImportFromCodex_AgentDescriptionDiverges_RoutesViaXCodex(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude", "agents", "keeper.md"),
		"---\nname: keeper\ndescription: claude says X\nmodel: haiku\n---\nbody\n")
	writeFile(t, filepath.Join(dir, ".codex", "agents", "keeper.toml"),
		`name = "keeper"`+"\n"+
			`description = "codex says Y"`+"\n"+
			`developer_instructions = "body"`+"\n")

	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if err := importFromCodex(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "agents", "keeper.md"))
	if !strings.Contains(got, "description: claude says X") {
		t.Errorf("claude description must survive at top level:\n%s", got)
	}
	if !strings.Contains(got, "description: codex says Y") {
		t.Errorf("codex description must land under x-codex:\n%s", got)
	}
	// claude has model: haiku but codex doesn't → expect x-codex.model: null
	// so codex emit drops the key.
	if !strings.Contains(got, "model: null") {
		t.Errorf("x-codex.model should mark deletion for codex:\n%s", got)
	}
}

// Divergent skill frontmatter description also routes via x-codex on
// import. #304 — without this, every codex skill description got
// overwritten with claude's value after one round-trip.
func TestImportFromCodex_SkillDescriptionDiverges_RoutesViaXCodex(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude", "skills", "test", "SKILL.md"),
		"---\nname: test\ndescription: claude says X\n---\nbody\n")
	writeFile(t, filepath.Join(dir, ".codex", "skills", "test", "SKILL.md"),
		"---\nname: test\ndescription: codex says Y\n---\nbody\n")

	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if err := importFromCodex(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "skills", "test", "SKILL.md"))
	if !strings.Contains(got, "description: claude says X") {
		t.Errorf("claude description must survive at top level:\n%s", got)
	}
	if !strings.Contains(got, "description: codex says Y") {
		t.Errorf("codex description must land under x-codex:\n%s", got)
	}
}

func TestImportFromCodex_AgentBodyIdentical_NoFences(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude", "agents", "explorer.md"),
		"---\nname: explorer\n---\nShared body.\n")
	writeFile(t, filepath.Join(dir, ".codex", "agents", "explorer.toml"),
		`name = "explorer"`+"\n"+
			`developer_instructions = "Shared body."`+"\n")

	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if err := importFromCodex(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "agents", "explorer.md"))
	if strings.Contains(got, "::target") {
		t.Errorf("did not expect fences for identical body:\n%s", got)
	}
}
