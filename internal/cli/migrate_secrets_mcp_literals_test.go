package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMigrate_SecretsMarksPlainValuesInPlace(t *testing.T) {
	dir := migrationFixture(t, "secrets-mcp-literals")
	silence(t)
	captureLogOut(t)

	out, err := runCLI(t, "migrate", "--only", "secrets")
	if err != nil {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
	wants := map[string]string{
		filepath.Join(".agnostic-ai", "mcps", "app.yaml"): `# Plain settings in every quoting style, next to values that stay.
name: app
description: App server.
command: npx
args: ["-y", "@example/app-mcp"]
env:
  NODE_ENV: !literal production   # keep this comment
  LOG_LEVEL: !literal "debug"
  REGION: !literal 'eu-west-1'
  DATABASE_URL: !literal postgres://localhost:${PGPORT}/app
  GITHUB_TOKEN: ${GITHUB_TOKEN}
  TEMPLATE: $${USER}
  RETRIES: 3
  EMPTY: ""
  ALREADY: !literal yes
`,
		filepath.Join(".agnostic-ai", "mcps", "docs.yaml"):         "name: docs\ntype: http\nurl: https://docs.example.com/mcp\nheaders: {X-Mode: !literal fast, Authorization: \"Bearer ${DOCS_TOKEN}\"}\n",
		filepath.Join(".agnostic-ai", "local", "mcps", "app.yaml"): "name: app\nenv:\n  DEBUG: !literal \"1\"\n",
	}
	for path, want := range wants {
		got, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil || string(got) != want {
			t.Errorf("%s = %v\n%s\nwant\n%s", path, err, got, want)
		}
	}
	if strings.Contains(out, "note:") {
		t.Errorf("no value became a reference, so there is no variable to set:\n%s", out)
	}
	if out, _ := runCLI(t, "lint"); strings.Contains(out, "LINT035") {
		t.Errorf("lint after migrate still warns:\n%s", out)
	}
}

func TestMigrate_SecretsTurnsCredentialsIntoReferencesAndNamesTheVariables(t *testing.T) {
	dir := newProject(t)
	captureLogOut(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	gh := filepath.Join(dir, ".agnostic-ai", "mcps", "gh.yaml")
	docs := filepath.Join(dir, ".agnostic-ai", "mcps", "docs.yaml")
	mustWriteFile(t, gh, "name: gh\ncommand: gh-mcp\nenv:\n  GITHUB_TOKEN: ghp_AAAAbbbbCCCC1111\n  NODE_ENV: production\n")
	mustWriteFile(t, docs, "name: docs\ntype: http\nurl: https://docs.example.com/mcp\nheaders:\n  Authorization: Bearer abc123def456\n")
	secrets := []string{"ghp_AAAAbbbbCCCC1111", "abc123def456"}

	dry, err := runCLI(t, "migrate", "--dry-run")
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, dry)
	}
	for _, want := range []string{
		"would rewrite .agnostic-ai/mcps/gh.yaml",
		"-  GITHUB_TOKEN: <redacted>", "+  GITHUB_TOKEN: ${GITHUB_TOKEN}",
		"-  NODE_ENV: <redacted>", "+  NODE_ENV: !literal <redacted>",
		"+  Authorization: Bearer ${DOCS_AUTHORIZATION}",
		"note: the rewritten credentials read a variable now; set DOCS_AUTHORIZATION and GITHUB_TOKEN",
	} {
		if !strings.Contains(dry, want) {
			t.Errorf("dry run lacks %q:\n%s", want, dry)
		}
	}

	out, err := runCLI(t, "migrate")
	if err != nil || !strings.Contains(out, "set DOCS_AUTHORIZATION and GITHUB_TOKEN in the shell that starts your tools") {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
	for _, text := range []string{dry, out} {
		for _, secret := range secrets {
			if strings.Contains(text, secret) {
				t.Errorf("output quotes a secret:\n%s", text)
			}
		}
	}
	if got, _ := os.ReadFile(gh); string(got) != "name: gh\ncommand: gh-mcp\nenv:\n  GITHUB_TOKEN: ${GITHUB_TOKEN}\n  NODE_ENV: !literal production\n" {
		t.Errorf("gh.yaml =\n%s", got)
	}
	if got, _ := os.ReadFile(docs); !strings.HasSuffix(string(got), "  Authorization: Bearer ${DOCS_AUTHORIZATION}\n") {
		t.Errorf("docs.yaml =\n%s", got)
	}
	if p, err := planMCPLiterals(migrationScope{root: "."}); err != nil || len(p.changes)+len(p.skips) != 0 {
		t.Errorf("a second run must find nothing: %v %+v", err, p)
	}
}

func TestMigrate_SecretsSkipsWhatItCannotDecide(t *testing.T) {
	dir := newProject(t)
	captureLogOut(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	path := filepath.Join(dir, ".agnostic-ai", "mcps", "api.yaml")
	body := "name: api\ncommand: srv\nenv:\n  API_KEY: sk-live-abc\n  DB_URL: postgres://u:pw@${HOST}/db\n  MODE: fast\n"
	mustWriteFile(t, path, body)

	out, err := runCLI(t, "migrate", "--only", "secrets")
	if err != nil {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
	for _, want := range []string{
		"skipped .agnostic-ai/mcps/api.yaml: env API_KEY has a credential name; write a ${NAME} reference, or mark it !literal by hand",
		"skipped .agnostic-ai/mcps/api.yaml: env DB_URL holds a credential around a reference; move it into one ${NAME} by hand",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "sk-live-abc") || strings.Contains(out, "pw@") {
		t.Errorf("output quotes a value:\n%s", out)
	}
	if got, _ := os.ReadFile(path); string(got) != strings.Replace(body, "MODE: fast", "MODE: !literal fast", 1) {
		t.Errorf("api.yaml =\n%s\nwant only MODE marked", got)
	}
	if hint := pendingMigrationHint("."); !strings.Contains(hint, "needs a manual step (secrets-mcp-literals)") {
		t.Errorf("doctor hint = %q, want the skips named as a manual step", hint)
	}
}

func TestRedactMigrationLines_HidesMultilineFlowQuotedKeysAndDefaults(t *testing.T) {
	in := []string{
		"env: {",
		"  NODE_ENV: SENTINEL1,",
		"}",
		"'headers':",
		"  X-Mode: SENTINEL2",
		"env:",
		"  TOKEN: ${TOKEN:-SENTINEL3}",
	}
	for _, line := range redactMigrationLines(in) {
		if strings.Contains(line, "SENTINEL") {
			t.Errorf("value leaks: %q", line)
		}
	}
}

func TestRedactMigrationLines_HidesEnvAndHeadersValuesButKeepsTagsAndReferences(t *testing.T) {
	in := []string{
		"env:",
		"  NODE_ENV: production",
		"  MODE: !literal fast",
		"  TOKEN: ${TOKEN}",
		"  NOTE: |",
		"    line one",
		"headers: {X-Key: abc}",
		"description: kept",
	}
	want := []string{
		"env:",
		"  NODE_ENV: <redacted>",
		"  MODE: !literal <redacted>",
		"  TOKEN: ${TOKEN}",
		"  NOTE: <redacted>",
		"    <redacted>",
		"headers: <redacted>",
		"description: kept",
	}
	if got := redactMigrationLines(in); !slices.Equal(got, want) {
		t.Errorf("redacted =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
