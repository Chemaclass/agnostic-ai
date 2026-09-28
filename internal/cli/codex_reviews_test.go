package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const codexReviewBody = "Flag handlers that query the database directly."

func setupCodexReviewProject(t *testing.T, targets string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+targets+"]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "Project instructions.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "reviews", "api.md"), "---\nscope: services/api\n---\n\n"+codexReviewBody+"\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "reviews", "review.md"), "Check that errors carry the path.\n")
	return dir
}

// A scoped review reaches Bugbot and Codex code review with the same
// text, and an unscoped one lands in the root AGENTS.md (#1341).
func TestSync_CodexReviewsLandInAgentsMD(t *testing.T) {
	dir := setupCodexReviewProject(t, "codex, cursor")
	testutil.Chdir(t, dir)
	silence(t)

	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	bugbot := readFileString(t, filepath.Join(dir, "services", "api", ".cursor", "BUGBOT.md"))
	scoped := readFileString(t, filepath.Join(dir, "services", "api", "AGENTS.md"))
	if !strings.Contains(bugbot, codexReviewBody) || !strings.Contains(scoped, "## Code Review Rules\n\n"+codexReviewBody) {
		t.Errorf("BUGBOT.md:\n%s\nAGENTS.md:\n%s", bugbot, scoped)
	}
	rootAgents := readFileString(t, filepath.Join(dir, "AGENTS.md"))
	if !strings.Contains(rootAgents, "## Code Review Rules\n\nCheck that errors carry the path.") || strings.Contains(rootAgents, codexReviewBody) {
		t.Errorf("root AGENTS.md:\n%s", rootAgents)
	}
	if err := runSync(t, "--check"); err != nil {
		t.Errorf("sync --check after sync: %v", err)
	}
}

func TestSync_NoReviewSectionWithoutCodex(t *testing.T) {
	dir := setupCodexReviewProject(t, "cursor, amp")
	testutil.Chdir(t, dir)
	silence(t)

	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "services", "api", "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("scoped AGENTS.md written without codex: %v", err)
	}
	if strings.Contains(readFileString(t, filepath.Join(dir, "AGENTS.md")), "Code Review Rules") {
		t.Error("root AGENTS.md carries a review section without codex")
	}
}

// import codex reads the synced section back into a review spec instead
// of a rule, and reads a hand-written `## Review guidelines` too.
func TestImportCodex_ReadsReviewSectionsBack(t *testing.T) {
	dir := setupCodexReviewProject(t, "codex")
	testutil.Chdir(t, dir)
	silence(t)
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", "reviews")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "web", "AGENTS.md"), "## Styling\n\nUse tokens.\n\n## Review guidelines\n\nFlag inline colors.\n")

	src := config.Sources{Rules: ".agnostic-ai/rules", Reviews: ".agnostic-ai/reviews", Agents: ".agnostic-ai/agents", Skills: ".agnostic-ai/skills", Hooks: ".agnostic-ai/hooks", MCPs: ".agnostic-ai/mcps", Commands: ".agnostic-ai/commands", Settings: ".agnostic-ai/settings"}
	if err := importFromCodex(dir, src); err != nil {
		t.Fatal(err)
	}
	reviews := filepath.Join(dir, ".agnostic-ai", "reviews")
	api := readFileString(t, filepath.Join(reviews, "services-api.md"))
	if !strings.Contains(api, "scope: services/api") || !strings.Contains(api, codexReviewBody) {
		t.Errorf("services-api review:\n%s", api)
	}
	if root := readFileString(t, filepath.Join(reviews, "review.md")); !strings.Contains(root, "Check that errors carry the path.") {
		t.Errorf("root review:\n%s", root)
	}
	web := readFileString(t, filepath.Join(reviews, "web.md"))
	if !strings.Contains(web, "scope: web") || !strings.Contains(web, "Flag inline colors.") {
		t.Errorf("web review:\n%s", web)
	}
	entries, err := os.ReadDir(filepath.Join(dir, ".agnostic-ai", "rules"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		body := readFileString(t, filepath.Join(dir, ".agnostic-ai", "rules", e.Name()))
		if strings.Contains(body, "Flag inline colors.") || strings.Contains(body, codexReviewBody) {
			t.Errorf("review text imported as rule %s:\n%s", e.Name(), body)
		}
	}
}
