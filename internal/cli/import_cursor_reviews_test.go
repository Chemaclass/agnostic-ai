package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Doctor sent users to `import cursor` for BUGBOT.md, but the importer
// skipped it, so the advice repeated forever (#1276). Root and nested
// files import with their scope, and sync writes them back unchanged.
func TestImportCursor_ImportsRootAndNestedBugbotFiles(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [cursor]\n")
	const rootReview = "# Review rules\n\n- Check errors.\n"
	const apiReview = "# API review\n\n- Money uses integer minor units.\n"
	rootPath := filepath.Join(dir, ".cursor", "BUGBOT.md")
	apiPath := filepath.Join(dir, "services", "api", ".cursor", "BUGBOT.md")
	writeFile(t, rootPath, rootReview)
	writeFile(t, apiPath, apiReview)

	execCLI(t, "import", "cursor")

	reviews := filepath.Join(dir, ".agnostic-ai", "reviews")
	if got := readFile(t, filepath.Join(reviews, "review.md")); got != "---\nname: review\n---\n\n"+rootReview {
		t.Errorf("root review spec = %q", got)
	}
	if got := readFile(t, filepath.Join(reviews, "services-api.md")); got != "---\nname: services-api\nscope: services/api\n---\n\n"+apiReview {
		t.Errorf("scoped review spec = %q", got)
	}

	execCLI(t, "sync", "-t", "cursor")
	for path, want := range map[string]string{rootPath: rootReview, apiPath: apiReview} {
		got := readFile(t, path)
		if !header.Has(got) || header.Strip(got) != want {
			t.Errorf("sync changed %s: %q", path, got)
		}
	}
}

// A BUGBOT.md sync wrote is not user content. Specs sharing a scope
// concatenate into it, so reading it back would emit them twice.
func TestImportCursor_SkipsGeneratedBugbot(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [cursor]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "reviews", "security.md"), "---\nname: security\n---\n\n- Flag secrets in logs.\n")
	execCLI(t, "sync", "-t", "cursor")
	execCLI(t, "import", "cursor")

	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "reviews", "review.md")); !os.IsNotExist(err) {
		t.Errorf("a generated BUGBOT.md must not import back into a spec: %v", err)
	}
}

func TestCursorReviewSpecName_DerivesFromScope(t *testing.T) {
	used := map[string]int{}
	var got []string
	for _, scope := range []string{"", "services/api", "review", "services-api"} {
		got = append(got, cursorReviewSpecName(used, scope))
	}
	want := []string{"review", "services-api", "review-2", "services-api-2"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("names = %v, want %v", got, want)
	}
}
