package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const ellipsisEnvironmentFile = ".ellipsis/environments/api-environment.yaml"

const ellipsisEnvironmentSettings = `x-ellipsis:
  environment:
    name: api-environment
    repository:
      name: api-repo
`

const ellipsisToolsServer = `name: internal-tools
type: http
url: https://tools.example.com/mcp
headers:
  Authorization: Bearer ${TOOLS_TOKEN}
`

// ellipsisEnvironmentProject lays out the acceptance scenario on top of the
// review project: one selected repository and environment, an environment
// spec that installs dependencies, and an HTTP MCP server with a secret
// reference.
func ellipsisEnvironmentProject(t *testing.T, config string) string {
	t.Helper()
	dir := ellipsisProject(t, config)
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "ellipsis-environment.yaml"), ellipsisEnvironmentSettings)
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "environments", "dev.yaml"), "name: dev\ninstall: npm ci\nsetup: npm run build --if-present\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "internal-tools.yaml"), ellipsisToolsServer)
	return dir
}

func TestEllipsisEnvironmentSync_PlanPreviewsThenSyncWritesTheFiles(t *testing.T) {
	dir := ellipsisEnvironmentProject(t, ellipsisOnly)
	before := snapshotProject(t, dir)
	planned, err := cliOutput(t, "sync", "--plan", "--json")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !strings.Contains(planned, `"path": "`+ellipsisEnvironmentFile+`"`) || !strings.Contains(planned, `"action": "create"`) {
		t.Errorf("plan does not preview %s:\n%s", ellipsisEnvironmentFile, planned)
	}
	shown, err := cliOutput(t, "sync", "--dry-run")
	if err != nil || !strings.Contains(shown, "--- "+ellipsisEnvironmentFile+" ---") || !strings.Contains(shown, "cd /sandbox/api-repo") {
		t.Errorf("dry run does not show the file (%v):\n%s", err, shown)
	}
	assertProjectUnchanged(t, before, snapshotProject(t, dir))

	execCLI(t, "sync")
	got := readFile(t, filepath.Join(dir, ellipsisEnvironmentFile))
	for _, want := range []string{
		"kind: environment",
		"name: api-environment",
		"- name: api-repo",
		"build_base:\n    run: |\n      cd /sandbox/api-repo\n      npm ci\n",
		"after_checkout: |\n    cd /sandbox/api-repo\n    npm run build --if-present\n",
		"- name: internal-tools",
		"url: https://tools.example.com/mcp",
		"Authorization: Bearer ${TOOLS_TOKEN}",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("%s lacks %q:\n%s", ellipsisEnvironmentFile, want, got)
		}
	}
	if review := readFile(t, filepath.Join(dir, ellipsisFile)); !strings.Contains(review, "environment: api-environment") {
		t.Errorf("the review file does not reference the environment:\n%s", review)
	}
	if _, err := os.Stat(filepath.Join(dir, ".mcp.json")); !os.IsNotExist(err) {
		t.Errorf("Ellipsis ignores .mcp.json, so none may be written (%v)", err)
	}
}

func TestEllipsisEnvironmentSync_SecretValuesNeverReachFilesOrPreviews(t *testing.T) {
	const secret = "ELLIPSIS_LOCAL_SECRET_2188"
	t.Setenv("TOOLS_TOKEN", secret)
	t.Setenv("REPO_TOOLS_TOKEN", secret)
	dir := ellipsisEnvironmentProject(t, ellipsisOnly)
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "repo-tools.yaml"),
		"name: repo-tools\ncommand: repo-tools\nenv:\n  API_TOKEN: ${REPO_TOOLS_TOKEN}\n  LOG_LEVEL: !literal debug\n")
	for _, args := range [][]string{{"sync", "--dry-run"}, {"sync", "--plan"}, {"sync", "--plan", "--json"}, {"sync", "--dry-run", "--json"}} {
		out, err := cliOutput(t, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if strings.Contains(out, secret) {
			t.Errorf("%v leaked the local secret", args)
		}
	}
	shown, _ := cliOutput(t, "sync", "--dry-run")
	if !strings.Contains(shown, "Bearer ${TOOLS_TOKEN}") || !strings.Contains(shown, "API_TOKEN: ${REPO_TOOLS_TOKEN}") {
		t.Errorf("the preview should show the references:\n%s", shown)
	}
	execCLI(t, "sync")
	if got := readFile(t, filepath.Join(dir, ellipsisEnvironmentFile)); strings.Contains(got, secret) || !strings.Contains(got, "LOG_LEVEL: debug") {
		t.Errorf("secret reached the file, or the plain setting is missing:\n%s", got)
	}
}

func TestEllipsisEnvironmentSync_LiteralSecretInAnMCPSpecIsRefused(t *testing.T) {
	const secret = "ELLIPSIS_LITERAL_SECRET_2188"
	dir := ellipsisEnvironmentProject(t, ellipsisOnly)
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "internal-tools.yaml"),
		"name: internal-tools\ntype: http\nurl: https://tools.example.com/mcp\nheaders:\n  Authorization: Bearer "+secret+"\n")
	out, err := cliOutput(t, "sync")
	if err == nil {
		t.Fatalf("sync wrote a literal secret:\n%s", out)
	}
	if strings.Contains(out+err.Error(), secret) {
		t.Errorf("the refusal printed the secret:\n%s%v", out, err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ellipsisEnvironmentFile)); !os.IsNotExist(statErr) {
		t.Errorf("a file was written despite the refusal (%v)", statErr)
	}
}

func TestEllipsisEnvironmentSync_EditingASourceFailsCheckUntilSync(t *testing.T) {
	dir := ellipsisEnvironmentProject(t, ellipsisOnly)
	execCLI(t, "sync")
	if _, err := runCLI(t, "sync", "--check"); err != nil {
		t.Fatalf("check right after sync: %v", err)
	}
	edits := []struct {
		path, from, to string
	}{
		{filepath.Join(dir, ".agnostic-ai", "environments", "dev.yaml"), "install: npm ci", "install: npm install"},
		{filepath.Join(dir, ".agnostic-ai", "mcps", "internal-tools.yaml"), "tools.example.com", "tools.example.org"},
		{filepath.Join(dir, ".agnostic-ai", "settings", "ellipsis-environment.yaml"), "name: api-repo", "name: web-repo"},
	}
	for _, e := range edits {
		old := readFile(t, e.path)
		writeFile(t, e.path, strings.Replace(old, e.from, e.to, 1))
		if _, err := runCLI(t, "sync", "--check"); err == nil {
			t.Errorf("sync --check passed after editing %s", e.path)
		}
		execCLI(t, "sync")
		if _, err := runCLI(t, "sync", "--check"); err != nil {
			t.Errorf("sync --check after re-sync of %s: %v", e.path, err)
		}
	}
	if got := readFile(t, filepath.Join(dir, ellipsisEnvironmentFile)); !strings.Contains(got, "npm install") || !strings.Contains(got, "tools.example.org") || !strings.Contains(got, "cd /sandbox/web-repo") {
		t.Errorf("the edits did not reach the file:\n%s", got)
	}
}

func TestEllipsisEnvironmentSync_RepeatedSyncIsStable(t *testing.T) {
	dir := ellipsisEnvironmentProject(t, ellipsisOnly)
	execCLI(t, "sync")
	execCLI(t, "sync")
	settled := snapshotProject(t, dir)
	execCLI(t, "sync")
	execCLI(t, "sync")
	again := snapshotProject(t, dir)
	// The state file records the sync time; every other file must stay put.
	delete(settled, filepath.Join(".agnostic-ai", ".sync-state"))
	delete(again, filepath.Join(".agnostic-ai", ".sync-state"))
	assertProjectUnchanged(t, settled, again)
}

func TestEllipsisEnvironmentSync_UpdatingASourceChangesOnlyTheExpectedOutput(t *testing.T) {
	dir := ellipsisEnvironmentProject(t, "version: 1\ntargets: [cursor, ellipsis]\ngitignore:\n  enabled: false\n")
	execCLI(t, "sync")
	before := snapshotEmitted(t, dir)
	path := filepath.Join(dir, ".agnostic-ai", "environments", "dev.yaml")
	writeFile(t, path, strings.Replace(readFile(t, path), "install: npm ci", "install: npm install", 1))
	execCLI(t, "sync")
	after := snapshotEmitted(t, dir)
	for p, was := range before {
		if p == ellipsisEnvironmentFile {
			if after[p] == was {
				t.Errorf("%s did not change", p)
			}
			continue
		}
		if strings.HasPrefix(p, ".cursor/") && after[p] != was {
			// Cursor reads the environment too, and its own file follows the same source.
			continue
		}
		if after[p] != was {
			t.Errorf("%s changed with an unrelated edit", p)
		}
	}
}

func TestEllipsisEnvironmentSync_ExistingTargetsKeepTheirOutput(t *testing.T) {
	dir := ellipsisEnvironmentProject(t, "version: 1\ntargets: [cursor]\ngitignore:\n  enabled: false\n")
	execCLI(t, "sync")
	alone := snapshotEmitted(t, dir)
	for _, p := range []string{".cursor", "BUGBOT.md", "services"} {
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
	for _, p := range []string{ellipsisFile, ellipsisEnvironmentFile} {
		if _, ok := both[p]; !ok {
			t.Errorf("ellipsis wrote no %s: %v", p, keys(both))
		}
		delete(both, p)
	}
	if len(both) != len(alone) {
		t.Errorf("ellipsis added files other than its two: %v vs %v", keys(both), keys(alone))
	}
}

func TestEllipsisEnvironmentSync_TargetFilterAndOutputOverride(t *testing.T) {
	dir := ellipsisEnvironmentProject(t, "version: 1\ntargets: [ellipsis]\ngitignore:\n  enabled: false\noutputs:\n  ellipsis:\n    environment-file: .ellipsis/environments/ci.yaml\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "cursor-only.yaml"), "name: cursor-only\ntargets: [cursor]\ntype: http\nurl: https://cursor.example.com/mcp\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "environments", "cursor-only.yaml"), "name: cursor-env\ntargets: [cursor]\ninstall: make cursor-only\n")
	execCLI(t, "sync")
	got := readFile(t, filepath.Join(dir, ".ellipsis", "environments", "ci.yaml"))
	if strings.Contains(got, "cursor") || !strings.Contains(got, "name: internal-tools") {
		t.Errorf("filter or override not honored:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ellipsisEnvironmentFile)); !os.IsNotExist(err) {
		t.Errorf("default path written despite the override (%v)", err)
	}
}

func TestEllipsisEnvironmentSync_NoSelectionWritesNoEnvironmentFile(t *testing.T) {
	dir := ellipsisEnvironmentProject(t, ellipsisOnly)
	if err := os.Remove(filepath.Join(dir, ".agnostic-ai", "settings", "ellipsis-environment.yaml")); err != nil {
		t.Fatal(err)
	}
	notes := captureNotes(t)
	out, err := cliOutput(t, "sync")
	if err != nil {
		t.Fatalf("sync must not fail by default: %v", err)
	}
	if !strings.Contains(out+notes.String(), "x-ellipsis.environment") {
		t.Errorf("sync did not name the missing selection:\n%s%s", out, notes)
	}
	if _, err := os.Stat(filepath.Join(dir, ".ellipsis", "environments")); !os.IsNotExist(err) {
		t.Errorf("no selection must create no environment file (%v)", err)
	}
	if got := readFile(t, filepath.Join(dir, ellipsisFile)); strings.Contains(got, "environment:") {
		t.Errorf("the review must not reference an environment nobody selected:\n%s", got)
	}
}

func TestEllipsisEnvironmentSync_RemovedSelectionLeavesNoStaleFile(t *testing.T) {
	dir := ellipsisEnvironmentProject(t, ellipsisOnly)
	execCLI(t, "sync")
	if _, err := os.Stat(filepath.Join(dir, ellipsisEnvironmentFile)); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, ".agnostic-ai", "settings", "ellipsis-environment.yaml")); err != nil {
		t.Fatal(err)
	}
	execCLI(t, "sync")
	if _, err := os.Stat(filepath.Join(dir, ellipsisEnvironmentFile)); !os.IsNotExist(err) {
		t.Errorf("stale environment file stays after its selection is gone (%v)", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ellipsisFile)); err != nil {
		t.Errorf("the review file must stay: %v", err)
	}
}

func TestEllipsisEnvironmentSync_RenamedEnvironmentRemovesTheOldFile(t *testing.T) {
	dir := ellipsisEnvironmentProject(t, ellipsisOnly)
	execCLI(t, "sync")
	path := filepath.Join(dir, ".agnostic-ai", "settings", "ellipsis-environment.yaml")
	writeFile(t, path, strings.Replace(ellipsisEnvironmentSettings, "api-environment", "web-environment", 1))
	execCLI(t, "sync")
	if _, err := os.Stat(filepath.Join(dir, ellipsisEnvironmentFile)); !os.IsNotExist(err) {
		t.Errorf("the old environment file stays (%v)", err)
	}
	if got := readFile(t, filepath.Join(dir, ".ellipsis", "environments", "web-environment.yaml")); !strings.Contains(got, "name: web-environment") {
		t.Errorf("renamed file missing:\n%s", got)
	}
	if got := readFile(t, filepath.Join(dir, ellipsisFile)); !strings.Contains(got, "environment: web-environment") {
		t.Errorf("the review must follow the rename:\n%s", got)
	}
}

func TestEllipsisEnvironmentSync_OnUnsupportedErrorStopsOnAnIncompatibleServer(t *testing.T) {
	dir := ellipsisEnvironmentProject(t, "version: 1\ntargets: [ellipsis]\non-unsupported: error\ngitignore:\n  enabled: false\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "events.yaml"), "name: events\ntype: sse\nurl: https://events.example.com/sse\n")
	_, err := runCLI(t, "sync")
	if err == nil || !strings.Contains(err.Error(), "events") || !strings.Contains(err.Error(), "sse") {
		t.Fatalf("want an error naming the server and its transport, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ellipsisEnvironmentFile)); !os.IsNotExist(statErr) {
		t.Errorf("file written despite the error (%v)", statErr)
	}
}

func TestEllipsisEnvironmentSync_IncompatibleServerIsNotedAndTheRestIsWritten(t *testing.T) {
	dir := ellipsisEnvironmentProject(t, ellipsisOnly)
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "events.yaml"), "name: events\ntype: sse\nurl: https://events.example.com/sse\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "host.yaml"), "name: host\ntype: http\nurl: https://${API_HOST}/mcp\n")
	notes := captureNotes(t)
	out, err := cliOutput(t, "sync")
	if err != nil {
		t.Fatalf("sync must not fail by default: %v", err)
	}
	got := readFile(t, filepath.Join(dir, ellipsisEnvironmentFile))
	if strings.Contains(got, "events") || strings.Contains(got, "API_HOST") || !strings.Contains(got, "name: internal-tools") {
		t.Errorf("incompatible servers must be left out and the rest kept:\n%s", got)
	}
	for _, want := range []string{"events", "sse", "host", "API_HOST"} {
		if !strings.Contains(out+notes.String(), want) {
			t.Errorf("sync did not mention %q:\n%s%s", want, out, notes)
		}
	}
}

func TestEllipsisEnvironmentSync_ConflictingDefinitionsWriteNoFile(t *testing.T) {
	dir := ellipsisEnvironmentProject(t, ellipsisOnly)
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "ellipsis-environment.yaml"), ellipsisEnvironmentSettings+`    hooks:
      build_base:
        run: make deps
`)
	notes := captureNotes(t)
	out, err := cliOutput(t, "sync")
	if err != nil {
		t.Fatalf("sync must not fail by default: %v", err)
	}
	if !strings.Contains(out+notes.String(), "build_base") {
		t.Errorf("sync did not name the conflicting stage:\n%s%s", out, notes)
	}
	if _, err := os.Stat(filepath.Join(dir, ellipsisEnvironmentFile)); !os.IsNotExist(err) {
		t.Errorf("a conflicting definition must write no file (%v)", err)
	}
}

func TestEllipsisEnvironmentSync_UnsafePathsAreRefused(t *testing.T) {
	dir := ellipsisEnvironmentProject(t, "version: 1\ntargets: [ellipsis]\non-unsupported: error\ngitignore:\n  enabled: false\noutputs:\n  ellipsis:\n    environment-file: ../outside.yaml\n")
	_, err := runCLI(t, "sync")
	if err == nil || !strings.Contains(err.Error(), "outputs.ellipsis.environment-file") {
		t.Fatalf("want an error naming outputs.ellipsis.environment-file, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(dir), "outside.yaml")); !os.IsNotExist(statErr) {
		t.Errorf("a file was written outside the project (%v)", statErr)
	}
}

func TestEllipsisEnvironmentSync_DryRunLeavesFilesUnchanged(t *testing.T) {
	dir := ellipsisEnvironmentProject(t, ellipsisOnly)
	before := snapshotProject(t, dir)
	captureStdout(t, func() {
		for _, args := range [][]string{{"sync", "--dry-run"}, {"sync", "--plan"}, {"sync", "--check"}} {
			_, _ = runCLI(t, args...)
		}
	})
	assertProjectUnchanged(t, before, snapshotProject(t, dir))
}

func TestEllipsisEnvironmentSync_GitignoreCommitKeepsTheFileTrackable(t *testing.T) {
	for _, kind := range []string{"ellipsis:environments", "ellipsis:mcps"} {
		t.Run(kind, func(t *testing.T) {
			dir := ellipsisEnvironmentProject(t, "version: 1\ntargets: [ellipsis, cursor]\ngitignore:\n  enabled: true\n  commit: ["+kind+"]\n")
			execCLI(t, "sync")
			ignore := readFile(t, filepath.Join(dir, ".gitignore"))
			if strings.Contains(ignore, ellipsisEnvironmentFile) || strings.Contains(ignore, "/.ellipsis/\n") {
				t.Errorf("a committed environment file must stay out of the ignore block:\n%s", ignore)
			}
			if !strings.Contains(ignore, "/.ellipsis/code-review.yaml") {
				t.Errorf("the review file is not committed by %s, so it stays ignored:\n%s", kind, ignore)
			}
		})
	}

	dir := ellipsisEnvironmentProject(t, "version: 1\ntargets: [ellipsis]\ngitignore:\n  enabled: true\n")
	execCLI(t, "sync")
	if ignore := readFile(t, filepath.Join(dir, ".gitignore")); !strings.Contains(ignore, "/.ellipsis/") {
		t.Errorf("without commit, the generated folder is ignored:\n%s", ignore)
	}
}

func TestEllipsisEnvironmentSync_ReviewsCommitAlsoKeepsTheReferencedEnvironment(t *testing.T) {
	dir := ellipsisEnvironmentProject(t, "version: 1\ntargets: [ellipsis]\ngitignore:\n  enabled: true\n  commit: [ellipsis:reviews]\n")
	execCLI(t, "sync")
	if ignore := readFile(t, filepath.Join(dir, ".gitignore")); strings.Contains(ignore, ".ellipsis") {
		t.Errorf("the review names the environment, so both files must stay trackable:\n%s", ignore)
	}
}

func TestEllipsisEnvironmentSync_CheckSeesAnEnvironmentFileDeletedByHand(t *testing.T) {
	dir := ellipsisEnvironmentProject(t, ellipsisOnly)
	execCLI(t, "sync")
	if err := os.Remove(filepath.Join(dir, ellipsisEnvironmentFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "sync", "--check"); err == nil {
		t.Error("sync --check passed with the environment file missing")
	}
}

func TestEllipsisEnvironmentSync_ProjectWithoutAnyEllipsisSettingsKeepsReviewBehavior(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), ellipsisOnly)
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "reviews", "general.md"), "---\nname: general\n---\n\nCheck.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "ellipsis.yaml"), ellipsisSettings)
	execCLI(t, "sync")
	if got := readFile(t, filepath.Join(dir, ellipsisFile)); strings.Contains(got, "environment:") {
		t.Errorf("a review-only project must not gain an environment key:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".ellipsis", "environments")); !os.IsNotExist(err) {
		t.Errorf("a review-only project must not gain an environment file (%v)", err)
	}
}

func TestEllipsisEnvironmentSync_BrokenSelectionKeepsTheDeployedFile(t *testing.T) {
	dir := ellipsisEnvironmentProject(t, ellipsisOnly)
	execCLI(t, "sync")
	deployed := readFile(t, filepath.Join(dir, ellipsisEnvironmentFile))
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "ellipsis-environment.yaml"), "x-ellipsis:\n  environment:\n    name: api-environment\n")
	notes := captureNotes(t)
	out, err := cliOutput(t, "sync")
	if err != nil {
		t.Fatalf("sync must not fail by default: %v", err)
	}
	if !strings.Contains(out+notes.String(), "x-ellipsis.environment.repository") {
		t.Errorf("sync did not name the missing repository:\n%s%s", out, notes)
	}
	if got := readFile(t, filepath.Join(dir, ellipsisEnvironmentFile)); got != deployed {
		t.Errorf("a broken selection must keep the deployed file:\n%s", got)
	}
	if got := readFile(t, filepath.Join(dir, ellipsisFile)); !strings.Contains(got, "environment: api-environment") {
		t.Errorf("the review must keep pointing at the deployed environment:\n%s", got)
	}
}

func TestEllipsisEnvironmentSync_VariableValueThatLooksLikeACredentialStopsTheSync(t *testing.T) {
	const secret = "pw-2188"
	dir := ellipsisEnvironmentProject(t, ellipsisOnly)
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "ellipsis-environment.yaml"), ellipsisEnvironmentSettings+`    variables:
      - name: DATABASE_URL
        value: postgres://admin:`+secret+`@db/app
`)
	out, err := cliOutput(t, "sync")
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("want an error naming DATABASE_URL, got %v", err)
	}
	if strings.Contains(out+err.Error(), secret) {
		t.Errorf("the refusal printed the value:\n%s%v", out, err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ellipsisEnvironmentFile)); !os.IsNotExist(statErr) {
		t.Errorf("a file was written despite the refusal (%v)", statErr)
	}
}

func TestEllipsisEnvironmentSync_PlainVariableValuesStillSync(t *testing.T) {
	dir := ellipsisEnvironmentProject(t, ellipsisOnly)
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "ellipsis-environment.yaml"), ellipsisEnvironmentSettings+`    variables:
      - name: NODE_ENV
        value: test
      - name: PGHOST
        value: postgres://db/app
`)
	execCLI(t, "sync")
	if got := readFile(t, filepath.Join(dir, ellipsisEnvironmentFile)); !strings.Contains(got, "value: test") {
		t.Errorf("plain variables must be written:\n%s", got)
	}
}

func TestEllipsisEnvironmentSync_LiteralInAnXEllipsisOverrideNeverReachesTheFile(t *testing.T) {
	const secret = "ELLIPSIS_OVERRIDE_SECRET_2188"
	dir := ellipsisEnvironmentProject(t, ellipsisOnly)
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "repo-tools.yaml"),
		"name: repo-tools\ncommand: repo-tools\nx-ellipsis:\n  env:\n    API_TOKEN: "+secret+"\n")
	notes := captureNotes(t)
	out, err := cliOutput(t, "sync")
	if err != nil {
		t.Fatalf("sync must not fail by default: %v", err)
	}
	if strings.Contains(out+notes.String(), secret) {
		t.Errorf("sync printed the value:\n%s%s", out, notes)
	}
	if got := readFile(t, filepath.Join(dir, ellipsisEnvironmentFile)); strings.Contains(got, secret) || strings.Contains(got, "repo-tools") {
		t.Errorf("the override value must keep the server out of the committed file:\n%s", got)
	}
	if !strings.Contains(out+notes.String(), "repo-tools") {
		t.Errorf("sync did not name the server:\n%s%s", out, notes)
	}
}

func TestEllipsisEnvironmentSync_BrokenIdentityKeepsDeployedFiles(t *testing.T) {
	for name, settings := range map[string]string{
		"missing name":                    "x-ellipsis:\n  environment:\n    repository:\n      name: api-repo\n",
		"unsafe name":                     "x-ellipsis:\n  environment:\n    name: ../outside\n    repository:\n      name: api-repo\n",
		"renamed with missing repository": "x-ellipsis:\n  environment:\n    name: web-environment\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := ellipsisEnvironmentProject(t, ellipsisOnly)
			execCLI(t, "sync")
			environment := readFile(t, filepath.Join(dir, ellipsisEnvironmentFile))
			review := readFile(t, filepath.Join(dir, ellipsisFile))
			writeFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "ellipsis-environment.yaml"), settings)
			if _, err := runCLI(t, "sync"); err != nil {
				t.Errorf("a broken selection must preserve the deployed environment: %v", err)
			}
			got, err := os.ReadFile(filepath.Join(dir, ellipsisEnvironmentFile))
			if err != nil || string(got) != environment {
				t.Errorf("broken selection changed deployed environment: %v\n%s", err, got)
			}
			if got := readFile(t, filepath.Join(dir, ellipsisFile)); got != review || !strings.Contains(got, "environment: api-environment") {
				t.Errorf("broken selection changed deployed review or its environment link:\n%s", got)
			}
		})
	}
}
