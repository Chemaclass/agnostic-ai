package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A claude-only project (no codex tree) must not gain a `target:` tag,
// preserving byte-identical round-trip for single-tool repos.
func TestImportFromClaude_NoCodexTree_NoTargetTag(t *testing.T) {
	dir := t.TempDir()
	body := "---\nname: explorer\nmodel: sonnet\n---\nbody\n"
	writeFile(t, filepath.Join(dir, ".claude", "agents", "explorer.md"), body)
	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "agents", "explorer.md"))
	if strings.Contains(got, "target:") {
		t.Errorf("unexpected target tag in claude-only repo:\n%s", got)
	}
}

// When the project has both tools but the codex side lacks a matching
// agent, the claude-imported spec gains `target: claude` so sync no
// longer cross-emits it.
func TestImportFromClaude_CodexLacksAgent_AddsTargetClaude(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude", "agents", "explorer.md"),
		"---\nname: explorer\n---\nbody\n")
	writeFile(t, filepath.Join(dir, ".codex", "agents", "phel.toml"),
		"name = \"phel\"\ndeveloper_instructions = \"x\"\n")

	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "agents", "explorer.md"))
	if !strings.Contains(got, "target: claude") {
		t.Errorf("expected `target: claude` in scoped agent:\n%s", got)
	}
}

// When codex carries a matching agent (under either canonical form) the
// imported spec must stay un-scoped so cross-emit keeps working.
func TestImportFromClaude_CodexHasAgent_NoTargetTag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude", "agents", "changelog-keeper.md"),
		"---\nname: changelog-keeper\n---\nbody\n")
	// Codex stores the same agent under the underscore convention.
	writeFile(t, filepath.Join(dir, ".codex", "agents", "changelog_keeper.toml"),
		"name = \"changelog_keeper\"\n")

	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "agents", "changelog-keeper.md"))
	if strings.Contains(got, "target:") {
		t.Errorf("did not expect target tag, both tools have spec:\n%s", got)
	}
}

func TestImportFromClaude_CodexLacksSkill_AddsTargetClaude(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude", "skills", "validator", "SKILL.md"),
		"---\nname: validator\n---\nbody\n")
	// Force codex tree to exist via a sibling skill so the gate fires.
	writeFile(t, filepath.Join(dir, ".codex", "skills", "other", "SKILL.md"),
		"---\nname: other\n---\nx\n")

	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "skills", "validator", "SKILL.md"))
	if !strings.Contains(got, "target: claude") {
		t.Errorf("expected `target: claude` in scoped skill:\n%s", got)
	}
}

func TestImportFromClaude_CodexHasSkill_NoTargetTag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude", "skills", "shared", "SKILL.md"),
		"---\nname: shared\n---\nbody\n")
	writeFile(t, filepath.Join(dir, ".codex", "skills", "shared", "SKILL.md"),
		"---\nname: shared\n---\nx\n")

	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "skills", "shared", "SKILL.md"))
	if strings.Contains(got, "target:") {
		t.Errorf("did not expect target tag in shared skill:\n%s", got)
	}
}

func TestImportFromCodex_NoClaudeTree_NoTargetTag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".codex", "agents", "phel.toml"),
		"name = \"phel\"\ndeveloper_instructions = \"x\"\n")
	if err := importFromCodex(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "agents", "phel.md"))
	if strings.Contains(got, "target:") {
		t.Errorf("unexpected target tag in codex-only repo:\n%s", got)
	}
}

func TestImportFromCodex_ClaudeLacksAgent_AddsTargetCodex(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".codex", "agents", "phel.toml"),
		"name = \"phel\"\ndeveloper_instructions = \"x\"\n")
	writeFile(t, filepath.Join(dir, ".claude", "agents", "explorer.md"),
		"---\nname: explorer\n---\nbody\n")
	if err := importFromCodex(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "agents", "phel.md"))
	if !strings.Contains(got, "target: codex") {
		t.Errorf("expected `target: codex` in scoped agent:\n%s", got)
	}
}

func TestImportFromCodex_ClaudeHasAgent_NoTargetTag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".codex", "agents", "changelog_keeper.toml"),
		"name = \"changelog_keeper\"\ndeveloper_instructions = \"x\"\n")
	writeFile(t, filepath.Join(dir, ".claude", "agents", "changelog-keeper.md"),
		"---\nname: changelog-keeper\n---\nbody\n")
	if err := importFromCodex(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "agents", "changelog-keeper.md"))
	if strings.Contains(got, "target:") {
		t.Errorf("did not expect target tag, both tools have spec:\n%s", got)
	}
}

func TestImportFromCodex_ClaudeLacksSkill_AddsTargetCodex(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".codex", "skills", "phel", "SKILL.md"),
		"---\nname: phel\n---\nbody\n")
	writeFile(t, filepath.Join(dir, ".claude", "skills", "other", "SKILL.md"),
		"---\nname: other\n---\nx\n")
	if err := importFromCodex(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "skills", "phel", "SKILL.md"))
	if !strings.Contains(got, "target: codex") {
		t.Errorf("expected `target: codex` in scoped skill:\n%s", got)
	}
}

func TestAddTargetFrontmatter_Idempotent(t *testing.T) {
	body := "---\nname: x\ntarget: claude\n---\nbody\n"
	got := addTargetFrontmatter(body, "claude")
	if got != body {
		t.Errorf("addTargetFrontmatter mutated already-scoped body:\n%s", got)
	}
}

func TestAddTargetFrontmatter_AppendsToExistingFrontmatter(t *testing.T) {
	body := "---\nname: x\n---\nbody\n"
	got := addTargetFrontmatter(body, "claude")
	want := "---\nname: x\ntarget: claude\n---\nbody\n"
	if got != want {
		t.Errorf("addTargetFrontmatter:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestAddTargetFrontmatter_NoFrontmatter_Synthesizes(t *testing.T) {
	body := "bare body\n"
	got := addTargetFrontmatter(body, "codex")
	want := "---\ntarget: codex\n---\n\nbare body\n"
	if got != want {
		t.Errorf("addTargetFrontmatter:\ngot:  %q\nwant: %q", got, want)
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
