package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const includeScope = "apps/engine/src/integrations/create-candidate"

func setupReviewIncludeProject(t *testing.T, targets, review string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+targets+"]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "Project instructions.\n")
	writeFile(t, filepath.Join(dir, includeScope, "README.md"), "# Create candidate\n\nCheck that the payload keeps its ids.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "reviews", "create-candidate.md"), review)
	return dir
}

func TestSync_ReviewIncludesAFileWithAnAtPathLine(t *testing.T) {
	dir := setupReviewIncludeProject(t, "cursor", "---\nscope: "+includeScope+"\ntarget: cursor\n---\n\n@"+includeScope+"/README.md\n")
	testutil.Chdir(t, dir)
	silence(t)

	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	bugbot := readFileString(t, filepath.Join(dir, filepath.FromSlash(includeScope), ".cursor", "BUGBOT.md"))
	if !strings.Contains(bugbot, "# Create candidate\n\nCheck that the payload keeps its ids.\n") {
		t.Errorf("BUGBOT.md lacks the README content:\n%s", bugbot)
	}
	if !strings.Contains(bugbot, "agnostic-ai") || strings.Contains(bugbot, "@"+includeScope) {
		t.Errorf("BUGBOT.md must carry the provenance header and no @path line:\n%s", bugbot)
	}
	if err := runSync(t, "--check"); err != nil {
		t.Errorf("sync --check right after sync: %v", err)
	}

	writeFile(t, filepath.Join(dir, includeScope, "README.md"), "# Create candidate\n\nCheck the new field.\n")
	if err := runSync(t, "--check"); err == nil {
		t.Error("sync --check must report drift once the README changes")
	}
}

func TestSync_ReviewIncludeOfAMissingFileFails(t *testing.T) {
	dir := setupReviewIncludeProject(t, "cursor", "---\nscope: "+includeScope+"\n---\n\n@"+includeScope+"/MISSING.md\n")
	testutil.Chdir(t, dir)
	silence(t)

	err := runSync(t)
	if err == nil || !strings.Contains(err.Error(), "MISSING.md") || !strings.Contains(err.Error(), "create-candidate.md") {
		t.Errorf("expected an error naming the review and the missing file, got %v", err)
	}
}

func TestSync_ReviewIncludeStaysInsideTheProject(t *testing.T) {
	dir := setupReviewIncludeProject(t, "cursor", "---\nscope: "+includeScope+"\n---\n\n@../outside.md\n")
	testutil.Chdir(t, dir)
	silence(t)

	err := runSync(t)
	if err == nil || !strings.Contains(err.Error(), "../outside.md") {
		t.Errorf("expected an error refusing a path outside the project, got %v", err)
	}
}

func TestSync_ReviewIncludeLeavesFencedAtLinesAlone(t *testing.T) {
	dir := setupReviewIncludeProject(t, "cursor", "---\nscope: "+includeScope+"\n---\n\nWrite includes like this:\n\n```\n@some/file.md\n```\n")
	testutil.Chdir(t, dir)
	silence(t)

	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	bugbot := readFileString(t, filepath.Join(dir, filepath.FromSlash(includeScope), ".cursor", "BUGBOT.md"))
	if !strings.Contains(bugbot, "@some/file.md") {
		t.Errorf("a fenced @path line must stay as written:\n%s", bugbot)
	}
}

func TestSync_ReviewIncludeReachesCodexReviewSection(t *testing.T) {
	dir := setupReviewIncludeProject(t, "codex", "---\nscope: "+includeScope+"\n---\n\n@"+includeScope+"/README.md\n")
	testutil.Chdir(t, dir)
	silence(t)

	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	agents := readFileString(t, filepath.Join(dir, filepath.FromSlash(includeScope), "AGENTS.md"))
	if !strings.Contains(agents, "## Code Review Rules\n\n# Create candidate") {
		t.Errorf("scoped AGENTS.md lacks the included README:\n%s", agents)
	}
}
