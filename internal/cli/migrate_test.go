package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// migrationFixture copies testdata/migrate/<id> into a fresh project
// directory and makes it the working directory.
func migrationFixture(t *testing.T, id string) string {
	t.Helper()
	src, err := filepath.Abs(filepath.Join("testdata", "migrate", id))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("migration %s has no fixture at testdata/migrate/%s: %v", id, id, err)
	}
	dir := testutil.TempCwd(t)
	if err := copyTree(src, dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Every registered migration keeps what sync writes: sync, migrate, then
// sync --check passes. A second run finds nothing to rewrite.
func TestMigrate_EveryMigrationKeepsSyncedOutputAndIsIdempotent(t *testing.T) {
	for _, m := range specMigrations {
		t.Run(m.ID, func(t *testing.T) {
			migrationFixture(t, m.ID)
			silence(t)
			captureLogOut(t)
			mustSync(t)

			out, err := runCLI(t, "migrate", "--only", m.Group)
			if err != nil || !strings.Contains(out, m.ID+": ") {
				t.Fatalf("migrate: %v\n%s", err, out)
			}
			if out, err := runCLI(t, "sync", "--check", "--gitignore=off"); err != nil {
				t.Errorf("sync --check after migrate: %v\n%s", err, out)
			}
			for _, p := range planMigrations(".", []specMigration{m}) {
				if p.planErr != nil || len(p.changes) > 0 {
					t.Errorf("a second run must find nothing to rewrite, only skips: %+v", p.changes)
				}
			}
		})
	}
}

func TestMigrate_RegistryIDsAreGroupPrefixedAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range specMigrations {
		if seen[m.ID] || !strings.HasPrefix(m.ID, m.Group+"-") || m.Release == "" || m.Summary == "" || m.Plan == nil {
			t.Errorf("migration %+v must have a unique <group>-<what> ID, a release, a summary, and a plan", m)
		}
		seen[m.ID] = true
	}
}

func TestMigrate_ConfigFileNameRenamesAndKeepsBytesAndMode(t *testing.T) {
	dir := migrationFixture(t, "config-file-name")
	legacy := filepath.Join(dir, "agnostic.config.yaml")
	before, err := os.ReadFile(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(legacy, 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runCLI(t, "migrate", "--dry-run")
	if err != nil || !strings.Contains(out, "would rename agnostic.config.yaml -> agnostic-ai.yaml") {
		t.Fatalf("dry run: %v\n%s", err, out)
	}
	if !exists(legacy) || exists(filepath.Join(dir, "agnostic-ai.yaml")) {
		t.Fatal("--dry-run must write nothing")
	}

	if out, err := runCLI(t, "migrate"); err != nil || !strings.Contains(out, "renamed agnostic.config.yaml -> agnostic-ai.yaml") {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
	after, err := os.ReadFile(filepath.Join(dir, "agnostic-ai.yaml"))
	if err != nil || string(after) != string(before) || exists(legacy) {
		t.Fatalf("the rename must keep the bytes, comments included, and drop the old file: %v\n%s", err, after)
	}
	if info, err := os.Stat(filepath.Join(dir, "agnostic-ai.yaml")); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		t.Errorf("the rename must keep the file mode: %v %v", err, info.Mode())
	}
}

func TestMigrate_ConfigFileNameSkipsWhenBothFilesExist(t *testing.T) {
	dir := migrationFixture(t, "config-file-name")
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")

	out, err := runCLI(t, "migrate")
	if err != nil || !strings.Contains(out, "skipped agnostic.config.yaml: agnostic-ai.yaml exists and wins") {
		t.Fatalf("both files: %v\n%s", err, out)
	}
	if !exists(filepath.Join(dir, "agnostic.config.yaml")) {
		t.Error("a skip must leave the file")
	}
}

func TestMigrate_ListAndOnly(t *testing.T) {
	newProject(t)
	out, err := runCLI(t, "migrate", "--list")
	if err != nil || !strings.Contains(out, "config-file-name (0.79.0): rename agnostic.config.yaml to agnostic-ai.yaml: does not apply") {
		t.Errorf("--list: %v\n%s", err, out)
	}
	if out, err := runCLI(t, "migrate"); err != nil || !strings.Contains(out, "no migrations apply") {
		t.Errorf("nothing to do: %v\n%s", err, out)
	}
	if _, err := runCLI(t, "migrate", "--only", "nope"); err == nil || !strings.Contains(err.Error(), `no migration group "nope"; groups: config, hooks`) {
		t.Errorf("an unknown group must fail and list the groups: %v", err)
	}
	if _, err := runCLI(t, "migrate", "--only", ""); err == nil || !strings.Contains(err.Error(), `no migration group ""`) {
		t.Errorf("an empty --only must fail, not run everything: %v", err)
	}
}

func TestMigrate_QuietPrintsOnlySkips(t *testing.T) {
	dir := migrationFixture(t, "config-file-name")
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\n")
	out, err := runCLI(t, "-q", "migrate")
	if err != nil || strings.TrimSpace(out) != "config-file-name: skipped agnostic.config.yaml: agnostic-ai.yaml exists and wins; remove agnostic.config.yaml by hand once it holds nothing you need" {
		t.Errorf("-q output = %q, %v", out, err)
	}
}

func TestMigrate_OutsideAProjectFailsLikeOtherCommands(t *testing.T) {
	testutil.TempCwd(t)
	if _, err := runCLI(t, "migrate"); err == nil || !strings.Contains(err.Error(), "no agnostic-ai.yaml") {
		t.Errorf("migrate outside a project = %v", err)
	}
}

func TestRedactMigrationLines_HidesSecretsAndKeepsReferences(t *testing.T) {
	got := redactMigrationLines([]string{
		"env:",
		"  GITHUB_TOKEN: ghp_abcdefghijklmnop1234",
		"  API_KEY: ${API_KEY}",
		"  NODE_ENV: production",
		"  - --token=sk-live-abcdefghij123456",
		"url: https://h/mcp?token=abc123secret",
		"password: |",
		"  hunter2-plain",
		"next: kept",
		`args: ["--api-key", "abc123secretvalue"]`,
		"args:",
		"  - --api-key",
		"  - abc123secretvalue",
		"  - https://user:pa55word@host/mcp",
		"  - --port",
		"  - 8080",
	})
	want := []string{
		"env:",
		"  GITHUB_TOKEN: <redacted>",
		"  API_KEY: ${API_KEY}",
		"  NODE_ENV: production",
		"  - <redacted>",
		"url: <redacted>",
		"password: <redacted>",
		"  <redacted>",
		"next: kept",
		"args: <redacted>",
		"args:",
		"  - --api-key",
		"  - <redacted>",
		"  - <redacted>",
		"  - --port",
		"  - 8080",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestMigrate_ConfigFileNameFinishesAnInterruptedRename(t *testing.T) {
	dir := migrationFixture(t, "config-file-name")
	body, err := os.ReadFile(filepath.Join(dir, "agnostic.config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), string(body))

	out, err := runCLI(t, "migrate")
	if err != nil || !strings.Contains(out, "removed agnostic.config.yaml") {
		t.Fatalf("both files with the same bytes: %v\n%s", err, out)
	}
	if exists(filepath.Join(dir, "agnostic.config.yaml")) || !exists(filepath.Join(dir, "agnostic-ai.yaml")) {
		t.Error("the run must remove only the old file")
	}
}

func TestMigrate_ConfigFileNameSkipsWhenTheNewNameIsABrokenSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := migrationFixture(t, "config-file-name")
	if err := os.Symlink("missing.yaml", filepath.Join(dir, "agnostic-ai.yaml")); err != nil {
		t.Fatal(err)
	}
	out, err := runCLI(t, "migrate")
	if err != nil || !strings.Contains(out, "skipped agnostic.config.yaml: agnostic-ai.yaml is a broken symlink") {
		t.Fatalf("broken symlink: %v\n%s", err, out)
	}
}

func TestMigrate_ConfigFileNameSkipsWhenGitIgnoresTheNewName(t *testing.T) {
	dir := setupGitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, filepath.Join(dir, "agnostic.config.yaml"), "version: 1\n")
	mustWriteFile(t, filepath.Join(dir, ".gitignore"), "*.yaml\n")
	git(t, dir, "add", "-f", ".gitignore", "agnostic.config.yaml")

	out, err := runCLI(t, "migrate")
	if err != nil || !strings.Contains(out, "skipped agnostic.config.yaml: git ignores agnostic-ai.yaml") {
		t.Fatalf("ignored new name: %v\n%s", err, out)
	}
	if !exists(filepath.Join(dir, "agnostic.config.yaml")) {
		t.Error("a skip must leave the tracked file")
	}
}

func TestMigrate_ListAndDryRunAreExclusive(t *testing.T) {
	newProject(t)
	if _, err := runCLI(t, "migrate", "--list", "--dry-run"); err == nil {
		t.Error("--list with --dry-run must fail, not ignore --dry-run")
	}
}

// A content rewrite is the path the first config rename never takes: the
// dry run prints a redacted diff, and the run rewrites in place.
func TestMigrate_ContentRewriteShowsARedactedDiffAndWritesInPlace(t *testing.T) {
	dir := newProject(t)
	spec := filepath.Join(dir, "spec.yaml")
	before := "token: ghp_abcdefghijklmnop1234\nmode: old\n"
	after := "token: ghp_abcdefghijklmnop1234\nmode: new\n"
	mustWriteFile(t, spec, before)
	registry := specMigrations
	t.Cleanup(func() { specMigrations = registry })
	specMigrations = []specMigration{{
		ID: "test-rewrite", Group: "test", Release: "0.0.0", Summary: "rewrite mode",
		Plan: func(root string) ([]migrationChange, []migrationSkip, error) {
			body, err := os.ReadFile(filepath.Join(root, "spec.yaml"))
			if err != nil || string(body) != before {
				return nil, nil, err
			}
			return []migrationChange{{Path: filepath.Join(root, "spec.yaml"), Before: before, After: after}}, nil, nil
		},
	}}

	out, err := runCLI(t, "migrate", "--dry-run")
	if err != nil || !strings.Contains(out, "would rewrite spec.yaml") || !strings.Contains(out, "+mode: new") || strings.Contains(out, "ghp_") {
		t.Fatalf("dry run: %v\n%s", err, out)
	}
	if out, err := runCLI(t, "migrate"); err != nil || !strings.Contains(out, "rewrote spec.yaml") {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(spec); string(got) != after {
		t.Errorf("spec.yaml = %q, want %q", got, after)
	}
}

func TestMigrate_ConfigFileNameKeepsTheLegacyFileASymlinkPointsAt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := migrationFixture(t, "config-file-name")
	if err := os.Symlink("agnostic.config.yaml", filepath.Join(dir, "agnostic-ai.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "migrate"); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(dir, "agnostic.config.yaml")) {
		t.Error("the file a symlink points at must stay")
	}
}

func TestMigrate_ConfigFileNameKeepsATrackedDuplicateWhenGitIgnoresTheNewName(t *testing.T) {
	dir := setupGitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, filepath.Join(dir, "agnostic.config.yaml"), "version: 1\n")
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\n")
	mustWriteFile(t, filepath.Join(dir, ".gitignore"), "agnostic-ai.yaml\n")
	git(t, dir, "add", ".gitignore", "agnostic.config.yaml")

	out, err := runCLI(t, "migrate")
	if err != nil || !strings.Contains(out, "skipped agnostic.config.yaml: git ignores agnostic-ai.yaml") || !exists(filepath.Join(dir, "agnostic.config.yaml")) {
		t.Fatalf("ignored duplicate: %v\n%s", err, out)
	}
}

func TestWriteMigrationChange_RemoveNeedsTheKeptFile(t *testing.T) {
	dir := t.TempDir()
	legacy, current := filepath.Join(dir, "agnostic.config.yaml"), filepath.Join(dir, "agnostic-ai.yaml")
	mustWriteFile(t, legacy, "version: 1\n")
	err := writeMigrationChange(migrationChange{Path: legacy, NewPath: current, Before: "version: 1\n", Remove: true})
	if err == nil || !exists(legacy) {
		t.Fatalf("a removal whose kept file vanished must fail and keep the old file: %v", err)
	}
}

func TestDoctor_NamesPendingMigrations(t *testing.T) {
	migrationFixture(t, "config-file-name")
	out, _ := runCLI(t, "doctor")
	if !strings.Contains(out, "Spec migrations:\n  ! 1 spec migration applies (config-file-name). Preview: agnostic-ai migrate --dry-run") {
		t.Errorf("doctor must name the pending migration:\n%s", out)
	}
	if _, err := runCLI(t, "migrate"); err != nil {
		t.Fatal(err)
	}
	if out, _ := runCLI(t, "doctor"); strings.Contains(out, "Spec migrations:") {
		t.Errorf("doctor must say nothing once no migration applies:\n%s", out)
	}
}

func TestPendingMigrationHint_SaysManualWhenOnlySkips(t *testing.T) {
	dir := migrationFixture(t, "config-file-name")
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	if got := pendingMigrationHint("."); got != "1 spec migration needs a manual step (config-file-name). Preview: agnostic-ai migrate --dry-run" {
		t.Errorf("hint = %q", got)
	}
}

func TestMigrate_HooksPortableEventsRewritesInPlaceAndSkipsWhatDoesNotMap(t *testing.T) {
	dir := migrationFixture(t, "hooks-portable-events")
	silence(t)
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "local", "hooks", "session-status.yaml"), "timeout: 5\n")

	out, err := runCLI(t, "migrate", "--only", "hooks")
	if err != nil {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
	for _, want := range []string{
		"rewrote .agnostic-ai/hooks/no-force-push.yaml",
		"rewrote .agnostic-ai/hooks/stop-check.yaml",
		`skipped .agnostic-ai/hooks/gofmt-on-edit.yaml: no portable form gives PostToolUse with matcher "Edit|Write" on claude; match: edit there also covers MultiEdit and NotebookEdit`,
		`skipped .agnostic-ai/hooks/read-guard.yaml: no portable form gives PreToolUse with matcher "Read" on codex`,
		"skipped .agnostic-ai/local/hooks/session-status.yaml: a local/ spec extends this hook; rewrite both files by hand",
		"note: portable hooks reach augment, claude, cline, codex, copilot, crush, factory, gemini, goose, openhands, qoder, and windsurf today; other targets skip them until their mapping lands",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
	if hint := pendingMigrationHint("."); !strings.HasPrefix(hint, "1 spec migration needs a manual step (hooks-portable-events)") {
		t.Errorf("a local extension needs the user, so doctor names it: %q", hint)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".agnostic-ai", "hooks", "no-force-push.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want := "# Keep this comment: migrations must not reformat hooks.\n" +
		"name: no-force-push\n" +
		"description: Block git push --force.\n" +
		"targets: [claude, codex]\n" +
		"\n" +
		"on: \"before-tool\"   # the tool call, before it runs\n" +
		"match: shell\n" +
		"command: 'echo \"blocked\" >&2; exit 2'\n" +
		"timeout: 10\n"
	if string(got) != want {
		t.Errorf("no-force-push.yaml =\n%s\nwant:\n%s", got, want)
	}
}

func TestMigrate_HooksPortableEventsSkipsATargetWithoutPortableEvents(t *testing.T) {
	dir := migrationFixture(t, "hooks-portable-events")
	silence(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex, cursor]\n")

	out, err := runCLI(t, "migrate", "--only", "hooks", "--dry-run")
	if err != nil || !strings.Contains(out, "skipped .agnostic-ai/hooks/stop-check.yaml: no portable form gives Stop on cursor") {
		t.Errorf("a hook that reaches cursor must stay native: %v\n%s", err, out)
	}
	if !strings.Contains(out, "note: portable hooks reach augment, claude, cline, codex, copilot, crush, factory, gemini, goose, openhands, qoder, and windsurf today; cursor skips them until its mapping lands") {
		t.Errorf("want the note to name cursor:\n%s", out)
	}
	if !strings.Contains(out, "would rewrite .agnostic-ai/hooks/no-force-push.yaml") {
		t.Errorf("a hook scoped to claude and codex still migrates:\n%s", out)
	}
}

func TestPendingMigrationHint_CountsOnlyRewritesAndActionableSkips(t *testing.T) {
	dir := migrationFixture(t, "hooks-portable-events")
	silence(t)
	if got := pendingMigrationHint("."); !strings.HasPrefix(got, "1 spec migration applies (hooks-portable-events)") {
		t.Errorf("before the run: %q", got)
	}
	if _, err := runCLI(t, "migrate"); err != nil {
		t.Fatal(err)
	}
	if got := pendingMigrationHint("."); got != "" {
		t.Errorf("only skips that need nothing remain, so doctor must stay quiet: %q", got)
	}
	if out, _ := runCLI(t, "migrate", "--list"); !strings.Contains(out, "hooks-portable-events (0.79.0): rewrite a hook's event: and matcher: as the portable on: and match:: 0 to rewrite, 2 skipped") {
		t.Errorf("--list still counts every skip:\n%s", out)
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "both.yaml"), "on: stop\nevent: Stop\ncommand: 'true'\n")
	if got := pendingMigrationHint("."); !strings.HasPrefix(got, "1 spec migration needs a manual step (hooks-portable-events)") {
		t.Errorf("a spec with both forms needs the user: %q", got)
	}
}

func TestMigrate_APlanThatFailsDoesNotStopTheOthers(t *testing.T) {
	dir := migrationFixture(t, "config-file-name")
	silence(t)
	broken := specMigration{ID: "hooks-broken", Group: "hooks", Release: "0.79.0", Summary: "fails to plan",
		Plan: func(string) ([]migrationChange, []migrationSkip, error) {
			return nil, nil, errors.New("parse hooks/x.yaml:\n  token: ghp_abcdefghijklmnop1234")
		}}
	registry := specMigrations
	specMigrations = append([]specMigration{broken}, registry...)
	t.Cleanup(func() { specMigrations = registry })

	if out, _ := runCLI(t, "migrate", "--list"); !strings.Contains(out, "hooks-broken (0.79.0): fails to plan: cannot plan: parse hooks/x.yaml: token: <redacted>") {
		t.Errorf("--list must show the failed plan:\n%s", out)
	}
	if got := pendingMigrationHint("."); !strings.HasPrefix(got, "1 spec migration applies (config-file-name)") {
		t.Errorf("the hint ignores a failed plan: %q", got)
	}
	out, err := runCLI(t, "migrate")
	if err == nil || !strings.Contains(err.Error(), "1 migration could not plan: hooks-broken") {
		t.Errorf("migrate must exit non-zero after the rest ran: %v", err)
	}
	if !strings.Contains(out, "hooks-broken: cannot plan: parse hooks/x.yaml: token: <redacted>") || strings.Contains(out, "ghp_") {
		t.Errorf("the failure must print without its secret:\n%s", out)
	}
	if !exists(filepath.Join(dir, "agnostic-ai.yaml")) || exists(filepath.Join(dir, "agnostic.config.yaml")) {
		t.Errorf("the other migrations must still run:\n%s", out)
	}
}

func TestPrintMigrationPlan_PutsEachSkipOnOneLine(t *testing.T) {
	var out bytes.Buffer
	pending := []pendingMigration{{specMigration: specMigration{ID: "hooks-x", Summary: "s"}, skips: []migrationSkip{
		{Path: "hooks/a.yaml", Reason: "cannot rewrite in place: yaml: line 3:\n  mapping key \"on\" already defined at line 2"},
	}}}
	printMigrationPlan(&out, pending, true, false)
	if !strings.Contains(out.String(), "  skipped hooks/a.yaml: cannot rewrite in place: yaml: line 3: mapping key \"on\" already defined at line 2\n") {
		t.Errorf("a skip reason must print on one line:\n%s", out.String())
	}
}
