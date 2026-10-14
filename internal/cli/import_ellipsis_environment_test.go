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

const handWrittenEllipsisEnvironment = `# The API sandbox.
ellipsis:
  kind: environment
  name: api-environment
  description: API sandbox
  metadata:
    labels: [api]

repositories:
  - name: api-repo
    ref: main

variables:
  - name: NPM_TOKEN
  - name: NODE_ENV
    value: test

hooks:
  build_base:
    inputs:
      - api-repo/package.json
      - api-repo/package-lock.json
    run: |
      cd /sandbox/api-repo
      npm ci
  after_checkout: |
    cd /sandbox/api-repo
    npm run build --if-present
  before_start: |
    cd /sandbox/api-repo
    test -d node_modules

compute:
  cpu: 2
  memory: 8GB
  timeout: 30m

mcp_servers:
  - slack
  - name: internal-tools
    url: https://tools.example.com/mcp
    headers:
      Authorization: Bearer ${TOOLS_TOKEN}
      X-Team: platform
  - name: repo-tools
    command: node
    args: [/sandbox/api-repo/scripts/mcp-server.js]
    env:
      API_TOKEN: ${REPO_TOOLS_TOKEN}
      LOG_LEVEL: debug
`

func ellipsisEnvironmentImportProject(t *testing.T, native string) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), ellipsisOnly)
	writeFile(t, filepath.Join(dir, ellipsisEnvironmentFile), native)
	return dir
}

// importLog captures what import prints, notes included.
func importLog(t *testing.T) *strings.Builder {
	t.Helper()
	var logged strings.Builder
	prev := logOut
	logOut = &logged
	t.Cleanup(func() { logOut = prev })
	return &logged
}

func parsedYAML(t *testing.T, path string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(readFile(t, path)), &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return doc
}

func TestImportEllipsisEnvironment_NativeFileBecomesSpecs(t *testing.T) {
	dir := ellipsisEnvironmentImportProject(t, handWrittenEllipsisEnvironment)
	execCLI(t, "import", "ellipsis")
	source := filepath.Join(dir, ".agnostic-ai")

	env := parsedYAML(t, filepath.Join(source, "environments", "ellipsis.yaml"))
	if env["install"] != "npm ci" || env["setup"] != "npm run build --if-present" {
		t.Errorf("the checkout scripts must become install and setup without their cd line: %v", env)
	}
	if targets, _ := env["targets"].([]any); len(targets) != 1 || targets[0] != "ellipsis" {
		t.Errorf("an Ellipsis setup must stay with Ellipsis: %v", env["targets"])
	}

	settings := readFile(t, filepath.Join(source, "settings", "ellipsis-environment.yaml"))
	for _, want := range []string{"x-ellipsis:", "name: api-environment", "name: api-repo", "ref: main", "API sandbox", "labels:", "NPM_TOKEN", "value: test",
		"api-repo/package-lock.json", "before_start:", "test -d node_modules", "memory: 8GB", "- slack"} {
		if !strings.Contains(settings, want) {
			t.Errorf("settings spec lost %q:\n%s", want, settings)
		}
	}
	for _, absent := range []string{"npm ci", "after_checkout", "internal-tools", "kind:"} {
		if strings.Contains(settings, absent) {
			t.Errorf("settings spec must not hold %q:\n%s", absent, settings)
		}
	}

	http := parsedYAML(t, filepath.Join(source, "mcps", "internal-tools.yaml"))
	if http["type"] != "http" || http["url"] != "https://tools.example.com/mcp" {
		t.Errorf("http server: %v", http)
	}
	if headers, _ := http["headers"].(map[string]any); headers["Authorization"] != "Bearer ${TOOLS_TOKEN}" {
		t.Errorf("the secret reference must survive: %v", http["headers"])
	}
	stdio := parsedYAML(t, filepath.Join(source, "mcps", "repo-tools.yaml"))
	if stdio["command"] != "node" {
		t.Errorf("stdio server: %v", stdio)
	}
	if targets, _ := stdio["targets"].([]any); len(targets) != 1 || targets[0] != "ellipsis" {
		t.Errorf("a sandbox server must stay with Ellipsis: %v", stdio["targets"])
	}
}

func TestImportEllipsisEnvironment_ImportThenSyncKeepsTheFileMeaning(t *testing.T) {
	dir := ellipsisEnvironmentImportProject(t, handWrittenEllipsisEnvironment)
	want := parsedYAML(t, filepath.Join(dir, ellipsisEnvironmentFile))
	execCLI(t, "import", "ellipsis")
	execCLI(t, "sync")
	got := parsedYAML(t, filepath.Join(dir, ellipsisEnvironmentFile))
	wantYAML, _ := yaml.Marshal(want)
	gotYAML, _ := yaml.Marshal(got)
	if string(wantYAML) != string(gotYAML) {
		t.Errorf("import then sync changed the file's meaning\n--- want ---\n%s\n--- got ---\n%s", wantYAML, gotYAML)
	}
	if _, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after import and sync: %v", err)
	}
	if out, err := runCLI(t, "lint"); err != nil {
		t.Errorf("lint after import: %v\n%s", err, out)
	}
}

func TestImportEllipsisEnvironment_SecondImportAndSyncAreStable(t *testing.T) {
	dir := ellipsisEnvironmentImportProject(t, handWrittenEllipsisEnvironment)
	execCLI(t, "import", "ellipsis")
	execCLI(t, "sync")
	first := snapshotProject(t, dir)
	execCLI(t, "import", "ellipsis")
	execCLI(t, "sync")
	assertProjectUnchanged(t, withoutState(first), withoutState(snapshotProject(t, dir)))
}

func TestImportEllipsisEnvironment_NativeScriptsKeepTheirOwnForm(t *testing.T) {
	native := strings.Replace(handWrittenEllipsisEnvironment, "      cd /sandbox/api-repo\n      npm ci\n",
		"      sudo apt-get install -y jq\n      cd /sandbox/api-repo\n      npm ci\n", 1)
	native = strings.Replace(native, "    cd /sandbox/api-repo\n    npm run build --if-present\n", "    cd /sandbox/api-repo/web\n    npm run build\n", 1)
	dir := ellipsisEnvironmentImportProject(t, native)
	want := parsedYAML(t, filepath.Join(dir, ellipsisEnvironmentFile))
	execCLI(t, "import", "ellipsis")
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "environments", "ellipsis.yaml")); !os.IsNotExist(err) {
		t.Errorf("scripts that do not start in the checkout must not become install or setup (%v)", err)
	}
	settings := readFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "ellipsis-environment.yaml"))
	if !strings.Contains(settings, "sudo apt-get install -y jq") || !strings.Contains(settings, "cd /sandbox/api-repo/web") {
		t.Errorf("native scripts must be kept as written:\n%s", settings)
	}
	execCLI(t, "sync")
	got := parsedYAML(t, filepath.Join(dir, ellipsisEnvironmentFile))
	wantYAML, _ := yaml.Marshal(want)
	gotYAML, _ := yaml.Marshal(got)
	if string(wantYAML) != string(gotYAML) {
		t.Errorf("import then sync changed the file's meaning\n--- want ---\n%s\n--- got ---\n%s", wantYAML, gotYAML)
	}
}

func TestImportEllipsisEnvironment_UnrepresentableFilesAreLeftWithANote(t *testing.T) {
	for name, tc := range map[string]struct{ native, want string }{
		"two repositories": {strings.Replace(handWrittenEllipsisEnvironment, "repositories:\n  - name: api-repo\n    ref: main\n", "repositories:\n  - name: api-repo\n  - name: web-repo\n", 1), "repositories"},
		"unknown field":    {handWrittenEllipsisEnvironment + "triggers:\n  type: cron\n", "triggers"},
		"credential value": {strings.Replace(handWrittenEllipsisEnvironment, "  - name: NPM_TOKEN\n", "  - name: NPM_TOKEN\n    value: npm_abc123\n", 1), "NPM_TOKEN"},
		"credential shape": {strings.Replace(handWrittenEllipsisEnvironment, "  - name: NODE_ENV\n    value: test\n", "  - name: DATABASE_URL\n    value: postgres://admin:pw@db/app\n", 1), "DATABASE_URL"},
		"url and command":  {strings.Replace(handWrittenEllipsisEnvironment, "    command: node\n", "    command: node\n    url: https://x.example/mcp\n", 1), "repo-tools"},
		"server key":       {strings.Replace(handWrittenEllipsisEnvironment, "    command: node\n", "    command: node\n    cwd: tools\n", 1), "cwd"},
		"no repository":    {strings.Replace(handWrittenEllipsisEnvironment, "repositories:\n  - name: api-repo\n    ref: main\n", "", 1), "repositor"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := ellipsisEnvironmentImportProject(t, tc.native)
			logged := importLog(t)
			out, err := cliOutput(t, "import", "ellipsis")
			if err != nil {
				t.Fatalf("an unrepresentable file must not fail the import: %v", err)
			}
			out += logged.String()
			if !strings.Contains(out, tc.want) || !strings.Contains(out, "left") {
				t.Errorf("want a note that names %q and says the file was left:\n%s", tc.want, out)
			}
			if got := readFile(t, filepath.Join(dir, ellipsisEnvironmentFile)); got != tc.native {
				t.Errorf("the native file must stay as written:\n%s", got)
			}
			for _, sub := range []string{"environments", "mcps"} {
				if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", sub)); !os.IsNotExist(err) {
					t.Errorf("no %s spec may be written for a file left as written (%v)", sub, err)
				}
			}
			if strings.Contains(out, "npm_abc123") {
				t.Error("the note printed the credential")
			}
		})
	}
}

func TestImportEllipsisEnvironment_SeveralEnvironmentFilesAreLeft(t *testing.T) {
	dir := ellipsisEnvironmentImportProject(t, handWrittenEllipsisEnvironment)
	other := strings.Replace(handWrittenEllipsisEnvironment, "name: api-environment", "name: other-environment", 1)
	writeFile(t, filepath.Join(dir, ".ellipsis", "environments", "other.yaml"), other)
	logged := importLog(t)
	out, err := cliOutput(t, "import", "ellipsis")
	if err != nil {
		t.Fatal(err)
	}
	if out += logged.String(); !strings.Contains(out, "2 environment files") {
		t.Errorf("want a note that says two files were found:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "environments")); !os.IsNotExist(err) {
		t.Errorf("nothing may be imported when the choice is ambiguous (%v)", err)
	}
}

func TestImportEllipsisEnvironment_LiteralServerValuesBecomeReferences(t *testing.T) {
	const secret = "ELLIPSIS_NATIVE_SECRET_2188"
	native := strings.Replace(handWrittenEllipsisEnvironment, "Bearer ${TOOLS_TOKEN}", "Bearer "+secret, 1)
	dir := ellipsisEnvironmentImportProject(t, native)
	out, err := cliOutput(t, "import", "ellipsis")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, secret) {
		t.Errorf("import printed the secret:\n%s", out)
	}
	if got := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "internal-tools.yaml")); strings.Contains(got, secret) || !strings.Contains(got, "${") {
		t.Errorf("the spec must hold a reference, not the value:\n%s", got)
	}
}

func TestImportEllipsisEnvironment_DryRunWritesNothingAndListsTheSpecs(t *testing.T) {
	dir := ellipsisEnvironmentImportProject(t, handWrittenEllipsisEnvironment)
	before := snapshotProject(t, dir)
	out, err := cliOutput(t, "import", "ellipsis", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	assertProjectUnchanged(t, withoutState(before), withoutState(snapshotProject(t, dir)))
	for _, want := range []string{"environments/ellipsis.yaml", "settings/ellipsis-environment.yaml", "mcps/internal-tools.yaml", "mcps/repo-tools.yaml"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry run does not list %s:\n%s", want, out)
		}
	}
}

func TestImportEllipsisEnvironment_GeneratedFileImportsNothing(t *testing.T) {
	dir := ellipsisEnvironmentProject(t, ellipsisOnly)
	execCLI(t, "sync")
	before := snapshotProject(t, dir)
	execCLI(t, "import", "ellipsis")
	assertProjectUnchanged(t, withoutState(before), withoutState(snapshotProject(t, dir)))
}

func TestImportEllipsisEnvironment_ReviewAndEnvironmentImportTogether(t *testing.T) {
	dir := ellipsisImportProject(t, handWrittenEllipsis)
	writeFile(t, filepath.Join(dir, ellipsisEnvironmentFile), handWrittenEllipsisEnvironment)
	wantReview := parsedReview(t, filepath.Join(dir, ellipsisFile))
	wantEnv := parsedYAML(t, filepath.Join(dir, ellipsisEnvironmentFile))
	execCLI(t, "import", "ellipsis")
	execCLI(t, "sync")
	for path, want := range map[string]map[string]any{ellipsisFile: wantReview, ellipsisEnvironmentFile: wantEnv} {
		var got map[string]any
		if path == ellipsisFile {
			got = parsedReview(t, filepath.Join(dir, path))
		} else {
			got = parsedYAML(t, filepath.Join(dir, path))
		}
		wantYAML, _ := yaml.Marshal(want)
		gotYAML, _ := yaml.Marshal(got)
		if string(wantYAML) != string(gotYAML) {
			t.Errorf("%s changed meaning\n--- want ---\n%s\n--- got ---\n%s", path, wantYAML, gotYAML)
		}
	}
	if _, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v", err)
	}
}

func TestImportEllipsisEnvironment_HonorsTheOutputOverride(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [ellipsis]\ngitignore:\n  enabled: false\noutputs:\n  ellipsis:\n    environment-file: .ellipsis/environments/ci.yaml\n")
	writeFile(t, filepath.Join(dir, ".ellipsis", "environments", "ci.yaml"), handWrittenEllipsisEnvironment)
	execCLI(t, "import", "ellipsis")
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "environments", "ellipsis.yaml")); err != nil {
		t.Errorf("override file not read: %v", err)
	}
}

func TestFindUnmanagedConfig_FlagsAHandWrittenEllipsisEnvironmentFile(t *testing.T) {
	dir := ellipsisEnvironmentImportProject(t, handWrittenEllipsisEnvironment)
	got, err := findUnmanagedConfig(dir, &config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, g := range got {
		if g.Path == ellipsisEnvironmentFile && g.Target == "ellipsis" {
			found = true
		}
	}
	if !found {
		t.Errorf("a hand-written environment file must be flagged: %+v", got)
	}
}

func TestImportEllipsisEnvironment_PlainServerValuesStayPlain(t *testing.T) {
	dir := ellipsisEnvironmentImportProject(t, handWrittenEllipsisEnvironment)
	execCLI(t, "import", "ellipsis")
	for path, want := range map[string]string{
		filepath.Join(".agnostic-ai", "mcps", "internal-tools.yaml"): "X-Team: !literal platform",
		filepath.Join(".agnostic-ai", "mcps", "repo-tools.yaml"):     "LOG_LEVEL: !literal debug",
	} {
		if got := readFile(t, filepath.Join(dir, path)); !strings.Contains(got, want) {
			t.Errorf("%s must keep the plain value as %q, not turn it into a secret reference:\n%s", path, want, got)
		}
	}
	execCLI(t, "sync")
	got := readFile(t, filepath.Join(dir, ellipsisEnvironmentFile))
	for _, want := range []string{"X-Team: platform", "LOG_LEVEL: debug"} {
		if !strings.Contains(got, want) {
			t.Errorf("sync lost %q:\n%s", want, got)
		}
	}
	for _, absent := range []string{"INTERNAL_TOOLS_X_TEAM", "${LOG_LEVEL}"} {
		if strings.Contains(got, absent) {
			t.Errorf("a plain value became a stored-secret reference %q, and a missing secret fails the session:\n%s", absent, got)
		}
	}
}

func TestImportEllipsisEnvironment_CredentialValuesPointToEllipsisSecrets(t *testing.T) {
	const secret = "ELLIPSIS_NATIVE_TOKEN_2188"
	native := strings.Replace(handWrittenEllipsisEnvironment, "API_TOKEN: ${REPO_TOOLS_TOKEN}", "API_TOKEN: "+secret, 1)
	dir := ellipsisEnvironmentImportProject(t, native)
	logged := importLog(t)
	out, err := cliOutput(t, "import", "ellipsis")
	if err != nil {
		t.Fatal(err)
	}
	out += logged.String()
	if strings.Contains(out, secret) {
		t.Errorf("import printed the secret:\n%s", out)
	}
	if !strings.Contains(out, "Ellipsis") || strings.Contains(out, "shell") {
		t.Errorf("the hint must say to store the secret in Ellipsis, not to export it in a shell:\n%s", out)
	}
	if got := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "repo-tools.yaml")); strings.Contains(got, secret) || !strings.Contains(got, "API_TOKEN: ${API_TOKEN}") {
		t.Errorf("the credential must become a reference:\n%s", got)
	}
}

func TestImportEllipsisEnvironment_CredentialInAURLIsLeftWithANote(t *testing.T) {
	const secret = "ELLIPSIS_URL_SECRET_2188"
	native := strings.Replace(handWrittenEllipsisEnvironment, "url: https://tools.example.com/mcp", "url: https://tools.example.com/mcp?api_key="+secret, 1)
	dir := ellipsisEnvironmentImportProject(t, native)
	logged := importLog(t)
	out, err := cliOutput(t, "import", "ellipsis")
	if err != nil {
		t.Fatal(err)
	}
	out += logged.String()
	if !strings.Contains(out, "internal-tools") || !strings.Contains(out, "left") || strings.Contains(out, secret) {
		t.Errorf("want a note that names the server, says the file was left, and hides the value:\n%s", out)
	}
	if got := readFile(t, filepath.Join(dir, ellipsisEnvironmentFile)); got != native {
		t.Errorf("the native file must stay as written:\n%s", got)
	}
}
