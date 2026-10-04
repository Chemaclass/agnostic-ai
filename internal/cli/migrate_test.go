package cli

import (
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
// sync --check passes. A second run finds nothing to do.
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
			if out, err := runCLI(t, "migrate", "--only", m.Group); err != nil || !strings.Contains(out, "no migrations apply") {
				t.Errorf("a second run must find nothing: %v\n%s", err, out)
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
	if _, err := runCLI(t, "migrate", "--only", "nope"); err == nil || !strings.Contains(err.Error(), `no migration group "nope"; groups: config`) {
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
