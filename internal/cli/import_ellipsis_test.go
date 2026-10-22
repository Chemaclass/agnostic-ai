package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const handWrittenEllipsis = `# Review every push to the API.
ellipsis:
  kind: code_review
  name: api-review
  description: Review every push to the API
  enabled: false
  metadata:
    team: api

pull_requests:
  draft: false
  base: [main]

reviewers:
  - name: bugs
    paths: ['services/**']
    claude_code:
      prompt: |
        Find bugs in this pull request.
        Mark a comment blocking only when the change must not merge as is.
      model: claude-opus-5-5
    environment: api-environment
    budget:
      session: 5

output:
  on_pass: approve
  on_fail: request_changes
  summary: true

budget:
  session: 3
`

func ellipsisImportProject(t *testing.T, native string) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), ellipsisOnly)
	writeFile(t, filepath.Join(dir, ellipsisFile), native)
	return dir
}

// parsedReview reads a code-review file as data, with each prompt trimmed:
// a block scalar keeps a trailing newline the spec body does not.
func parsedReview(t *testing.T, path string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(readFile(t, path)), &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	reviewers, _ := doc["reviewers"].([]any)
	for _, r := range reviewers {
		reviewer, _ := r.(map[string]any)
		for _, engine := range []string{"claude_code", "codex", "gemini"} {
			if block, ok := reviewer[engine].(map[string]any); ok {
				block["prompt"] = strings.TrimSpace(block["prompt"].(string))
			}
		}
	}
	return doc
}

func TestImportEllipsis_HandWrittenFileKeepsEveryField(t *testing.T) {
	dir := ellipsisImportProject(t, handWrittenEllipsis)
	want := parsedReview(t, filepath.Join(dir, ellipsisFile))

	execCLI(t, "import", "ellipsis")

	review := readFile(t, filepath.Join(dir, ".agnostic-ai", "reviews", "review.md"))
	if !strings.Contains(review, "Find bugs in this pull request.") || strings.Contains(review, "claude-opus") {
		t.Errorf("the prompt alone belongs in the review spec:\n%s", review)
	}
	settings := readFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "ellipsis.yaml"))
	for _, want := range []string{"x-ellipsis:", "api-review", "Review every push to the API", "enabled: false", "team: api", "api-environment", "approve", "request_changes", "services/**", "claude-opus-5-5"} {
		if !strings.Contains(settings, want) {
			t.Errorf("settings spec lost %q:\n%s", want, settings)
		}
	}
	if strings.Contains(settings, "kind:") {
		t.Errorf("kind is implied and must not be copied:\n%s", settings)
	}
	if strings.Contains(settings, "Find bugs") {
		t.Errorf("the prompt must not be copied into settings:\n%s", settings)
	}

	execCLI(t, "sync")
	got := parsedReview(t, filepath.Join(dir, ellipsisFile))
	wantYAML, _ := yaml.Marshal(want)
	gotYAML, _ := yaml.Marshal(got)
	if string(wantYAML) != string(gotYAML) {
		t.Errorf("import then sync changed the file's meaning\n--- want ---\n%s\n--- got ---\n%s", wantYAML, gotYAML)
	}
	if !strings.Contains(readFile(t, filepath.Join(dir, ellipsisFile)), "enabled: false") {
		t.Error("a paused file must stay paused after import and sync")
	}
	if _, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after import and sync: %v", err)
	}
	if out, err := runCLI(t, "lint"); err != nil {
		t.Errorf("lint after import: %v\n%s", err, out)
	}
}

func TestImportEllipsis_ReviewerWithoutAnEnvironmentStaysInTheBasicSandbox(t *testing.T) {
	review := `ellipsis:
  kind: code_review
  name: default
reviewers:
  - name: bugs
    claude_code:
      prompt: Find bugs in this pull request.
      model: claude-opus-5-5
`
	environment := `ellipsis:
  kind: environment
  name: api-with-tools
repositories:
  - name: api
mcp_servers:
  - name: internal-tools
    url: https://tools.example.com/mcp
`
	dir := ellipsisImportProject(t, review)
	writeFile(t, filepath.Join(dir, ".ellipsis", "environments", "api-with-tools.yaml"), environment)
	want := parsedReview(t, filepath.Join(dir, ellipsisFile))

	execCLI(t, "import", "ellipsis")
	execCLI(t, "sync")

	got := parsedReview(t, filepath.Join(dir, ellipsisFile))
	wantYAML, _ := yaml.Marshal(want)
	gotYAML, _ := yaml.Marshal(got)
	if string(wantYAML) != string(gotYAML) {
		t.Errorf("a reviewer without an environment must keep running in the basic sandbox\n--- want ---\n%s\n--- got ---\n%s", wantYAML, gotYAML)
	}
	if _, err := os.Stat(filepath.Join(dir, ".ellipsis", "environments", "api-with-tools.yaml")); err != nil {
		t.Errorf("the environment must still be written: %v", err)
	}
	if _, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after import and sync: %v", err)
	}
	if out, err := runCLI(t, "lint"); err != nil {
		t.Errorf("lint after import: %v\n%s", err, out)
	}
}

func TestImportEllipsis_ReviewerWithoutAnEnvironmentAndNoEnvironmentFileGetsNoNull(t *testing.T) {
	dir := ellipsisImportProject(t, strings.Replace(handWrittenEllipsis, "    environment: api-environment\n", "", 1))
	execCLI(t, "import", "ellipsis")
	if settings := readFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "ellipsis.yaml")); strings.Contains(settings, "environment") {
		t.Errorf("with no environment imported, the reviewer needs no environment key:\n%s", settings)
	}
}

func TestImportEllipsis_SecondImportAndSyncAreStable(t *testing.T) {
	dir := ellipsisImportProject(t, handWrittenEllipsis)
	execCLI(t, "import", "ellipsis")
	execCLI(t, "sync")
	first := snapshotProject(t, dir)

	execCLI(t, "import", "ellipsis")
	execCLI(t, "sync")
	assertProjectUnchanged(t, withoutState(first), withoutState(snapshotProject(t, dir)))
}

// withoutState keeps the files a user can see: folders, the command lock,
// the ledger, and the managed ignore file are runtime state.
func withoutState(snap map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range snap {
		if strings.HasPrefix(v, "<dir") || filepath.Base(k) == ".gitignore" ||
			strings.HasSuffix(k, ".sync-state") || strings.HasSuffix(k, ".command-lock") {
			continue
		}
		out[k] = v
	}
	return out
}

func TestImportEllipsis_SecondImportOfAHeaderlessFileAddsNothing(t *testing.T) {
	dir := ellipsisImportProject(t, handWrittenEllipsis)
	execCLI(t, "import", "ellipsis")
	before := snapshotProject(t, dir)
	execCLI(t, "import", "ellipsis")
	assertProjectUnchanged(t, withoutState(before), withoutState(snapshotProject(t, dir)))
	entries, err := os.ReadDir(filepath.Join(dir, ".agnostic-ai", "reviews"))
	if err != nil || len(entries) != 1 {
		t.Errorf("want one review spec, got %v (%v)", entries, err)
	}
}

func TestImportEllipsis_DryRunWritesNothingAndListsTheSpecs(t *testing.T) {
	dir := ellipsisImportProject(t, handWrittenEllipsis)
	before := snapshotProject(t, dir)
	out, err := cliOutput(t, "import", "ellipsis", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	assertProjectUnchanged(t, withoutState(before), withoutState(snapshotProject(t, dir)))
	for _, want := range []string{"reviews/review.md", "settings/ellipsis.yaml"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry run does not list %s:\n%s", want, out)
		}
	}
}

func TestImportEllipsis_GeneratedFileImportsNothing(t *testing.T) {
	dir := ellipsisProject(t, ellipsisOnly)
	execCLI(t, "sync")
	before := snapshotProject(t, dir)
	execCLI(t, "import", "ellipsis")
	assertProjectUnchanged(t, withoutState(before), withoutState(snapshotProject(t, dir)))
}

func TestImportEllipsis_UnrepresentableFilesStopWithAnActionableMessage(t *testing.T) {
	twoReviewers := strings.Replace(handWrittenEllipsis, "budget:\n  session: 3\n", `budget:
  session: 3
`, 1)
	twoReviewers = strings.Replace(twoReviewers, "output:\n", `  - name: docs
    codex:
      prompt: Check the docs.
      model: gpt-6-sol

output:
`, 1)
	for name, tc := range map[string]struct{ native, want string }{
		"two reviewers":    {twoReviewers, "2 reviewers"},
		"another kind":     {strings.Replace(handWrittenEllipsis, "kind: code_review", "kind: agent", 1), "code_review"},
		"unknown field":    {handWrittenEllipsis + "triggers:\n  type: cron\n", "triggers"},
		"no prompt":        {strings.Replace(handWrittenEllipsis, "      prompt: |\n        Find bugs in this pull request.\n        Mark a comment blocking only when the change must not merge as is.\n", "", 1), "prompt"},
		"not a mapping":    {"- just\n- a list\n", "code-review.yaml"},
		"invalid YAML":     {"ellipsis: [unclosed\n", "code-review.yaml"},
		"no reviewers key": {"ellipsis:\n  kind: code_review\n  name: x\n", "reviewers"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := ellipsisImportProject(t, tc.native)
			before := snapshotProject(t, dir)
			_, err := runCLI(t, "import", "ellipsis")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error naming %q, got %v", tc.want, err)
			}
			assertProjectUnchanged(t, withoutState(before), withoutState(snapshotProject(t, dir)))
		})
	}
}

func TestImportEllipsis_ExistingDifferentSettingsStopWithoutOverwrite(t *testing.T) {
	dir := ellipsisImportProject(t, handWrittenEllipsis)
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "ellipsis.yaml"), "x-ellipsis:\n  budget:\n    session: 1\n")
	before := snapshotProject(t, dir)
	if _, err := runCLI(t, "import", "ellipsis"); err == nil {
		t.Fatal("import replaced a different settings spec without --overwrite")
	}
	assertProjectUnchanged(t, withoutState(before), withoutState(snapshotProject(t, dir)))
}

func TestImportEllipsis_IsAKnownSourceAndDetectsTheFolder(t *testing.T) {
	if !isKnownImportSource("ellipsis") || !strings.Contains(importSources(), "ellipsis") {
		t.Error("ellipsis should be a known import source")
	}
	dir := ellipsisImportProject(t, handWrittenEllipsis)
	got, _ := detectImportSources(dir)
	if len(got) != 1 || got[0] != "ellipsis" {
		t.Errorf("detectImportSources = %v, want [ellipsis]", got)
	}
}

func TestImportEllipsis_HonorsTheOutputOverride(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [ellipsis]\ngitignore:\n  enabled: false\noutputs:\n  ellipsis:\n    review-file: ci/review.yaml\n")
	writeFile(t, filepath.Join(dir, "ci", "review.yaml"), handWrittenEllipsis)
	execCLI(t, "import", "ellipsis")
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "reviews", "review.md")); err != nil {
		t.Errorf("override file not read: %v", err)
	}
}

func TestFindUnmanagedConfig_FlagsAHandWrittenEllipsisFileOnly(t *testing.T) {
	dir := ellipsisImportProject(t, handWrittenEllipsis)
	got, err := findUnmanagedConfig(dir, &config.Config{})
	if err != nil || len(got) != 1 || got[0].Path != ellipsisFile || got[0].Target != "ellipsis" {
		t.Fatalf("hand-written file: got %+v (%v)", got, err)
	}
	execCLI(t, "import", "ellipsis")
	execCLI(t, "sync")
	got, err = findUnmanagedConfig(dir, &config.Config{})
	if err != nil || len(got) != 0 {
		t.Errorf("a generated file must not be flagged: %+v (%v)", got, err)
	}
}

func TestImportEllipsis_KeepsAGeminiReviewer(t *testing.T) {
	native := strings.Replace(handWrittenEllipsis, "claude_code:", "gemini:", 1)
	native = strings.Replace(native, "claude-opus-5-5", "gemini-4-argon", 1)
	dir := ellipsisImportProject(t, native)
	execCLI(t, "import", "ellipsis")
	settings := readFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "ellipsis.yaml"))
	for _, want := range []string{"gemini:", "gemini-4-argon"} {
		if !strings.Contains(settings, want) {
			t.Errorf("settings spec lost %q:\n%s", want, settings)
		}
	}
	if strings.Contains(settings, "Find bugs") {
		t.Errorf("the prompt must not be copied into settings:\n%s", settings)
	}
}
