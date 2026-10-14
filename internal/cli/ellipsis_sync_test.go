package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const ellipsisFile = ".ellipsis/code-review.yaml"

const ellipsisSettings = `x-ellipsis:
  pull_requests:
    draft: false
  reviewer:
    name: project-review
    claude_code:
      model: claude-opus-5-5
  budget:
    session: 3
`

// ellipsisProject lays out the acceptance scenario: a root review, a review
// scoped to services/api, and one explicitly configured reviewer.
func ellipsisProject(t *testing.T, config string) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), config)
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "reviews", "general.md"), "---\nname: general\n---\n\nFlag handlers that query the database directly.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "reviews", "services", "api", "contracts.md"), "---\nname: contracts\n---\n\nReject a change that renames a public response field.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "ellipsis.yaml"), ellipsisSettings)
	return dir
}

const ellipsisOnly = "version: 1\ntargets: [ellipsis]\ngitignore:\n  enabled: false\n"

// cliOutput runs one command and returns everything it printed, whether
// through the command's writer or straight to stdout.
func cliOutput(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out string
	var err error
	printed := captureStdout(t, func() { out, err = runCLI(t, args...) })
	return out + printed, err
}

func TestEllipsisSync_PlanPreviewsThenSyncWritesBothGuidanceSets(t *testing.T) {
	dir := ellipsisProject(t, ellipsisOnly)
	before := snapshotProject(t, dir)
	planned, err := cliOutput(t, "sync", "--plan", "--json")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !strings.Contains(planned, `"path": "`+ellipsisFile+`"`) || !strings.Contains(planned, `"action": "create"`) {
		t.Errorf("plan does not preview %s:\n%s", ellipsisFile, planned)
	}
	shown, err := cliOutput(t, "sync", "--dry-run")
	if err != nil || !strings.Contains(shown, "--- "+ellipsisFile+" ---") || !strings.Contains(shown, "## Checks for changes under services/api/") {
		t.Errorf("dry run does not show the file (%v):\n%s", err, shown)
	}
	assertProjectUnchanged(t, before, snapshotProject(t, dir))

	execCLI(t, "sync")
	got := readFile(t, filepath.Join(dir, ellipsisFile))
	for _, want := range []string{
		"kind: code_review",
		"Flag handlers that query the database directly.",
		"## Checks for changes under services/api/",
		"only when the pull request changes a file under `services/api/`",
		"Reject a change that renames a public response field.",
		"model: claude-opus-5-5",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("%s lacks %q:\n%s", ellipsisFile, want, got)
		}
	}
}

func TestEllipsisSync_EditingEitherSourceFailsCheckUntilSync(t *testing.T) {
	dir := ellipsisProject(t, ellipsisOnly)
	execCLI(t, "sync")
	if _, err := runCLI(t, "sync", "--check"); err != nil {
		t.Fatalf("check right after sync: %v", err)
	}
	for _, path := range []string{
		filepath.Join(dir, ".agnostic-ai", "reviews", "general.md"),
		filepath.Join(dir, ".agnostic-ai", "reviews", "services", "api", "contracts.md"),
	} {
		old := readFile(t, path)
		writeFile(t, path, old+"\nAlso check error handling.\n")
		if _, err := runCLI(t, "sync", "--check"); err == nil {
			t.Errorf("sync --check passed after editing %s", path)
		}
		execCLI(t, "sync")
		if _, err := runCLI(t, "sync", "--check"); err != nil {
			t.Errorf("sync --check after re-sync of %s: %v", path, err)
		}
	}
}

func TestEllipsisSync_ExistingTargetsKeepTheirOutput(t *testing.T) {
	dir := ellipsisProject(t, "version: 1\ntargets: [cursor]\ngitignore:\n  enabled: false\n")
	execCLI(t, "sync")
	alone := snapshotEmitted(t, dir)
	if err := os.RemoveAll(filepath.Join(dir, ".cursor")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"BUGBOT.md", "services"} {
		_ = os.RemoveAll(filepath.Join(dir, p))
	}
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [cursor, ellipsis]\ngitignore:\n  enabled: false\n")
	execCLI(t, "sync")
	both := snapshotEmitted(t, dir)
	for path, want := range alone {
		if both[path] != want {
			t.Errorf("%s changed when ellipsis joined the targets", path)
		}
	}
	if _, ok := both[ellipsisFile]; !ok {
		t.Errorf("ellipsis wrote no file: %v", keys(both))
	}
	delete(both, ellipsisFile)
	if len(both) != len(alone) {
		t.Errorf("ellipsis added files other than %s: %v vs %v", ellipsisFile, keys(both), keys(alone))
	}
}

func TestEllipsisSync_TargetFilterAndOutputOverride(t *testing.T) {
	dir := ellipsisProject(t, "version: 1\ntargets: [ellipsis]\ngitignore:\n  enabled: false\noutputs:\n  ellipsis:\n    review-file: ci/review.yaml\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "reviews", "cursor-only.md"), "---\nname: cursor-only\ntarget: cursor\n---\n\nCursor only guidance.\n")
	execCLI(t, "sync")
	got := readFile(t, filepath.Join(dir, "ci", "review.yaml"))
	if strings.Contains(got, "Cursor only guidance.") || !strings.Contains(got, "Flag handlers") {
		t.Errorf("filter or override not honored:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ellipsisFile)); !os.IsNotExist(err) {
		t.Errorf("default path written despite the override (%v)", err)
	}
}

func TestEllipsisSync_NoSelectedReviewsWritesNoFile(t *testing.T) {
	dir := ellipsisProject(t, ellipsisOnly)
	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", "reviews")); err != nil {
		t.Fatal(err)
	}
	execCLI(t, "sync")
	if _, err := os.Stat(filepath.Join(dir, ".ellipsis")); !os.IsNotExist(err) {
		t.Errorf("no reviews must create no .ellipsis folder (%v)", err)
	}
}

func TestEllipsisSync_RemovedReviewsLeaveNoStaleFile(t *testing.T) {
	dir := ellipsisProject(t, ellipsisOnly)
	execCLI(t, "sync")
	if _, err := os.Stat(filepath.Join(dir, ellipsisFile)); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", "reviews")); err != nil {
		t.Fatal(err)
	}
	execCLI(t, "sync")
	if _, err := os.Stat(filepath.Join(dir, ellipsisFile)); !os.IsNotExist(err) {
		t.Errorf("stale file stays after its reviews are gone (%v)", err)
	}
}

func TestEllipsisSync_MissingReviewerWritesNoFileAndNamesTheFix(t *testing.T) {
	dir := ellipsisProject(t, ellipsisOnly)
	if err := os.Remove(filepath.Join(dir, ".agnostic-ai", "settings", "ellipsis.yaml")); err != nil {
		t.Fatal(err)
	}
	notes := captureNotes(t)
	out, err := cliOutput(t, "sync")
	if err != nil {
		t.Fatalf("sync must not fail by default: %v", err)
	}
	if !strings.Contains(out+notes.String(), "x-ellipsis.reviewer") {
		t.Errorf("sync did not name the missing setting:\n%s%s", out, notes)
	}
	if _, err := os.Stat(filepath.Join(dir, ellipsisFile)); !os.IsNotExist(err) {
		t.Errorf("a file without a reviewer must not exist (%v)", err)
	}
}

func TestEllipsisSync_OnUnsupportedErrorStopsOnMissingReviewer(t *testing.T) {
	dir := ellipsisProject(t, "version: 1\ntargets: [ellipsis]\non-unsupported: error\ngitignore:\n  enabled: false\n")
	if err := os.Remove(filepath.Join(dir, ".agnostic-ai", "settings", "ellipsis.yaml")); err != nil {
		t.Fatal(err)
	}
	_, err := runCLI(t, "sync")
	if err == nil || !strings.Contains(err.Error(), "x-ellipsis.reviewer") {
		t.Fatalf("want an error naming x-ellipsis.reviewer, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ellipsisFile)); !os.IsNotExist(statErr) {
		t.Errorf("file written despite the error (%v)", statErr)
	}
}

func TestEllipsisSync_DryRunLeavesFilesUnchanged(t *testing.T) {
	dir := ellipsisProject(t, ellipsisOnly)
	before := snapshotProject(t, dir)
	captureStdout(t, func() {
		for _, args := range [][]string{{"sync", "--dry-run"}, {"sync", "--plan"}, {"sync", "--check"}} {
			_, _ = runCLI(t, args...)
		}
	})
	assertProjectUnchanged(t, before, snapshotProject(t, dir))
}

func TestEllipsisSync_PreviewsHideCredentialLikeValues(t *testing.T) {
	dir := ellipsisProject(t, ellipsisOnly)
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "ellipsis.yaml"), `x-ellipsis:
  reviewer:
    name: project-review
    claude_code:
      model: claude-opus-5-5
    permissions:
      github:
        api_token: ELLIPSIS_PREVIEW_SECRET_2187
`)
	for _, args := range [][]string{{"sync", "--dry-run"}, {"sync", "--plan"}, {"sync", "--plan", "--json"}, {"sync", "--dry-run", "--json"}} {
		out, err := cliOutput(t, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if strings.Contains(out, "ELLIPSIS_PREVIEW_SECRET_2187") {
			t.Errorf("%v leaked a credential", args)
		}
	}
	shown, _ := cliOutput(t, "sync", "--dry-run")
	if !strings.Contains(shown, "--- "+ellipsisFile+" ---") || !strings.Contains(shown, "<redacted>") {
		t.Errorf("the preview should show the file with the value hidden:\n%s", shown)
	}
}

func TestEllipsisSync_GitignoreCommitKeepsTheFileTrackable(t *testing.T) {
	dir := ellipsisProject(t, "version: 1\ntargets: [ellipsis, cursor]\ngitignore:\n  enabled: true\n  commit: [ellipsis:reviews]\n")
	execCLI(t, "sync")
	ignore := readFile(t, filepath.Join(dir, ".gitignore"))
	if strings.Contains(ignore, ".ellipsis") {
		t.Errorf("a committed review file must stay out of the ignore block:\n%s", ignore)
	}
	if !strings.Contains(ignore, "BUGBOT.md") {
		t.Errorf("other targets' reviews stay ignored:\n%s", ignore)
	}

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [ellipsis]\ngitignore:\n  enabled: true\n")
	execCLI(t, "sync")
	if ignore := readFile(t, filepath.Join(dir, ".gitignore")); !strings.Contains(ignore, "/.ellipsis/") {
		t.Errorf("without commit, the generated folder is ignored:\n%s", ignore)
	}
}

func TestEllipsisSync_ReviewerWithProblemsKeepsTheDeployedFile(t *testing.T) {
	dir := ellipsisProject(t, ellipsisOnly)
	execCLI(t, "sync")
	path := filepath.Join(dir, ellipsisFile)
	before := readFile(t, path)
	settings := filepath.Join(dir, ".agnostic-ai", "settings", "ellipsis.yaml")
	writeFile(t, settings, "x-ellipsis:\n  reviewer:\n    name: project-review\n    claude_code: {}\n")
	notes := captureNotes(t)
	out, err := cliOutput(t, "sync")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := readFile(t, path); got != before {
		t.Errorf("a broken reviewer changed the deployed file:\n%s", got)
	}
	if strings.Contains(out, "removed") || !strings.Contains(notes.String(), "claude_code.model") {
		t.Errorf("sync should note the problem and remove nothing:\n%s\n%s", out, notes)
	}

	writeFile(t, settings, "x-ellipsis:\n  budget:\n    session: 3\n")
	execCLI(t, "sync")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("with no reviewer key the file is removed (%v)", err)
	}
}
