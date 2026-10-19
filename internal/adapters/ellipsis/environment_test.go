package ellipsis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const environmentFile = ".ellipsis/environments/api-environment.yaml"

func environmentSpec(meta map[string]any) spec.Entry {
	return spec.Entry{Kind: spec.KindEnvironment, Name: "dev", Path: "environments/dev.yaml", Meta: meta}
}

func mcpSpec(name string, meta map[string]any) spec.Entry {
	return spec.Entry{Kind: spec.KindMCP, Name: name, Path: "mcps/" + name + ".yaml", Meta: meta}
}

// selected is a valid x-ellipsis.environment block: one environment name
// and one repository.
func selected(extra map[string]any) map[string]any {
	env := map[string]any{"name": "api-environment", "repository": map[string]any{"name": "api-repo"}}
	for k, v := range extra {
		env[k] = v
	}
	return env
}

func environmentSettings(extra map[string]any) spec.Entry {
	return settings(map[string]any{"environment": selected(extra)})
}

func httpServer(name string) spec.Entry {
	return mcpSpec(name, map[string]any{
		"type": "http", "url": "https://tools.example.com/mcp",
		"headers": map[string]any{"Authorization": "Bearer ${TOOLS_TOKEN}"},
	})
}

func readEnvFile(t *testing.T) string {
	t.Helper()
	return readReview(t, environmentFile)
}

func TestEnvironment_AcceptanceScenarioWritesSetupAndMCP(t *testing.T) {
	testutil.TempCwd(t)
	err := emitBundle(t, &config.Config{},
		review("general", "", "Check the changed code."),
		settings(map[string]any{
			"environment": selected(map[string]any{
				"compute": map[string]any{"cpu": 2, "memory": "8GB", "timeout": "30m"},
			}),
			"reviewer": reviewer(nil),
		}),
		environmentSpec(map[string]any{"install": "npm ci", "setup": "npm run build --if-present"}),
		httpServer("internal-tools"))
	if err != nil {
		t.Fatal(err)
	}
	want := header.Line(header.FormatYAML) + `ellipsis:
  kind: environment
  name: api-environment
repositories:
  - name: api-repo
hooks:
  build_base:
    run: |
      cd /sandbox/api-repo
      npm ci
  after_checkout: |
    cd /sandbox/api-repo
    npm run build --if-present
compute:
  cpu: 2
  memory: 8GB
  timeout: 30m
mcp_servers:
  - name: internal-tools
    url: https://tools.example.com/mcp
    headers:
      Authorization: Bearer ${TOOLS_TOKEN}
`
	if got := readEnvFile(t); got != want {
		t.Errorf("environment file mismatch\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

func TestEnvironment_ReviewReferencesTheSelectedEnvironment(t *testing.T) {
	testutil.TempCwd(t)
	if err := emitBundle(t, &config.Config{}, review("r", "", "Check."),
		settings(map[string]any{"environment": selected(nil), "reviewer": reviewer(nil)})); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Reviewers []map[string]any `yaml:"reviewers"`
	}
	if err := yaml.Unmarshal([]byte(readReview(t, reviewFile)), &doc); err != nil || len(doc.Reviewers) != 1 {
		t.Fatalf("parse review: %v", err)
	}
	if got := doc.Reviewers[0]["environment"]; got != "api-environment" {
		t.Errorf("reviewer.environment = %v, want api-environment", got)
	}
}

func TestEnvironment_ReviewerNamingTheSameEnvironmentIsFine(t *testing.T) {
	testutil.TempCwd(t)
	r := reviewer(map[string]any{"environment": "api-environment"})
	if err := emitBundle(t, &config.Config{}, review("r", "", "Check."),
		settings(map[string]any{"environment": selected(nil), "reviewer": r})); err != nil {
		t.Fatal(err)
	}
	if got := readReview(t, reviewFile); strings.Count(got, "environment: api-environment") != 1 {
		t.Errorf("want one environment reference:\n%s", got)
	}
}

func TestEnvironment_ReviewerNamingAnotherEnvironmentIsAConflict(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := captureNotes(t)
	r := reviewer(map[string]any{"environment": "other-environment"})
	if err := emitBundle(t, &config.Config{}, review("r", "", "Check."),
		settings(map[string]any{"environment": selected(nil), "reviewer": r})); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if _, err := os.Stat(filepath.Join(dir, reviewFile)); !os.IsNotExist(err) {
		t.Errorf("a review that contradicts the selection must not be written (%v)", err)
	}
	for _, want := range []string{"x-ellipsis.reviewer.environment", "other-environment", "api-environment"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("note should name %q:\n%s", want, buf)
		}
	}
}

func TestEnvironment_ReviewerWithANullEnvironmentStaysInTheBasicSandbox(t *testing.T) {
	for name, set := range map[string]map[string]any{
		"with a selection":    {"environment": selected(nil)},
		"without a selection": {},
	} {
		t.Run(name, func(t *testing.T) {
			testutil.TempCwd(t)
			set["reviewer"] = reviewer(map[string]any{"environment": nil})
			if err := emitBundle(t, &config.Config{}, review("r", "", "Check."), settings(set)); err != nil {
				t.Fatal(err)
			}
			if got := readReview(t, reviewFile); strings.Contains(got, "environment") {
				t.Errorf("a null reviewer environment must leave the key out:\n%s", got)
			}
		})
	}
}

func TestEnvironment_ReviewerKeepsAHandManagedEnvironmentWithoutSelection(t *testing.T) {
	testutil.TempCwd(t)
	r := reviewer(map[string]any{"environment": "managed-by-hand"})
	if err := emitBundle(t, &config.Config{}, review("r", "", "Check."), settings(map[string]any{"reviewer": r})); err != nil {
		t.Fatal(err)
	}
	if got := readReview(t, reviewFile); !strings.Contains(got, "environment: managed-by-hand") {
		t.Errorf("the hand-managed environment must pass through:\n%s", got)
	}
	if _, err := os.Stat(".ellipsis/environments"); !os.IsNotExist(err) {
		t.Errorf("no selection must create no environment folder (%v)", err)
	}
}

func TestEnvironment_SetupRunsInTheRepositoryCheckoutAtTheRightStage(t *testing.T) {
	testutil.TempCwd(t)
	err := emitBundle(t, &config.Config{}, environmentSettings(nil),
		environmentSpec(map[string]any{
			"install": []any{"sudo apt-get update", "sudo apt-get install -y jq"},
			"setup":   "bash scripts/setup.bash",
		}))
	if err != nil {
		t.Fatal(err)
	}
	got := readEnvFile(t)
	for _, want := range []string{
		"  build_base:\n    run: |\n      cd /sandbox/api-repo\n      sudo apt-get update\n      sudo apt-get install -y jq\n",
		"  after_checkout: |\n    cd /sandbox/api-repo\n    bash scripts/setup.bash\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "before_start") {
		t.Errorf("no portable field runs on every start, so before_start must be absent:\n%s", got)
	}
}

func TestEnvironment_NativeLifecycleOptionsAreWrittenAsGiven(t *testing.T) {
	testutil.TempCwd(t)
	hooks := map[string]any{
		"build_base":     map[string]any{"inputs": []any{"api-repo/package.json", "api-repo/package-lock.json"}},
		"before_start":   "cd /sandbox/api-repo\ntest -d node_modules\n",
		"validate":       map[string]any{"run": "command -v jq\n"},
		"after_checkout": nil,
	}
	delete(hooks, "after_checkout")
	err := emitBundle(t, &config.Config{}, environmentSettings(map[string]any{"hooks": hooks}),
		environmentSpec(map[string]any{"install": "npm ci"}))
	if err != nil {
		t.Fatal(err)
	}
	want := `hooks:
  build_base:
    inputs:
      - api-repo/package.json
      - api-repo/package-lock.json
    run: |
      cd /sandbox/api-repo
      npm ci
  before_start: |
    cd /sandbox/api-repo
    test -d node_modules
  validate: |
    command -v jq
`
	if got := readEnvFile(t); !strings.Contains(got, want) {
		t.Errorf("native hooks not written as given:\n%s", got)
	}
}

func TestEnvironment_NativeScriptsReplaceThePortableFieldTheyCover(t *testing.T) {
	testutil.TempCwd(t)
	hooks := map[string]any{
		"build_base":     map[string]any{"run": "sudo apt-get install -y jq\ncd /sandbox/api-repo\nnpm ci\n"},
		"after_checkout": "cd /sandbox/api-repo\nnpm run build\n",
	}
	if err := emitBundle(t, &config.Config{}, environmentSettings(map[string]any{"hooks": hooks})); err != nil {
		t.Fatal(err)
	}
	want := `hooks:
  build_base:
    run: |
      sudo apt-get install -y jq
      cd /sandbox/api-repo
      npm ci
  after_checkout: |
    cd /sandbox/api-repo
    npm run build
`
	if got := readEnvFile(t); !strings.Contains(got, want) {
		t.Errorf("native scripts must be written without a second cd:\n%s", got)
	}
}

func TestEnvironment_RepositoryOwnerRefVariablesAndMetadata(t *testing.T) {
	testutil.TempCwd(t)
	extra := map[string]any{
		"repository":  map[string]any{"name": "api-repo", "owner": "acme", "ref": "main"},
		"description": "API checkout",
		"metadata":    map[string]any{"labels": []any{"api"}},
		"variables": []any{
			map[string]any{"name": "NPM_TOKEN"},
			map[string]any{"name": "NODE_ENV", "value": "test"},
		},
		"mcp_servers": []any{"slack"},
	}
	if err := emitBundle(t, &config.Config{}, environmentSettings(extra), httpServer("internal-tools")); err != nil {
		t.Fatal(err)
	}
	want := header.Line(header.FormatYAML) + `ellipsis:
  kind: environment
  name: api-environment
  description: API checkout
  metadata:
    labels:
      - api
repositories:
  - name: api-repo
    owner: acme
    ref: main
variables:
  - name: NPM_TOKEN
  - name: NODE_ENV
    value: test
mcp_servers:
  - slack
  - name: internal-tools
    url: https://tools.example.com/mcp
    headers:
      Authorization: Bearer ${TOOLS_TOKEN}
`
	if got := readEnvFile(t); got != want {
		t.Errorf("mismatch\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

func TestEnvironment_StdioServerRunsInTheSandboxWithSecretReferences(t *testing.T) {
	testutil.TempCwd(t)
	server := mcpSpec("repo-tools", map[string]any{
		"command": "node",
		"args":    []any{"/sandbox/api-repo/scripts/mcp-server.js", "--verbose"},
		"env":     map[string]any{"API_TOKEN": "${REPO_TOOLS_TOKEN}", "LOG_LEVEL": "debug"},
	})
	if err := emitBundle(t, &config.Config{}, environmentSettings(nil), server); err != nil {
		t.Fatal(err)
	}
	want := `mcp_servers:
  - name: repo-tools
    command: node
    args:
      - /sandbox/api-repo/scripts/mcp-server.js
      - --verbose
    env:
      API_TOKEN: ${REPO_TOOLS_TOKEN}
      LOG_LEVEL: debug
`
	if got := readEnvFile(t); !strings.Contains(got, want) {
		t.Errorf("stdio server mismatch:\n%s", got)
	}
}

func TestEnvironment_ServersAreSortedAndIntegrationsStayFirst(t *testing.T) {
	testutil.TempCwd(t)
	if err := emitBundle(t, &config.Config{},
		environmentSettings(map[string]any{"mcp_servers": []any{"linear", "slack"}}),
		httpServer("zeta"), httpServer("alpha")); err != nil {
		t.Fatal(err)
	}
	got := readEnvFile(t)
	order := []string{"  - linear\n", "  - slack\n", "  - name: alpha\n", "  - name: zeta\n"}
	last := -1
	for _, want := range order {
		i := strings.Index(got, want)
		if i < 0 || i < last {
			t.Fatalf("%q out of order or missing in:\n%s", want, got)
		}
		last = i
	}
}

func TestEnvironment_NoSelectionWritesNoFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	captureNotes(t)
	if err := emitBundle(t, &config.Config{}, environmentSpec(map[string]any{"install": "npm ci"}), httpServer("internal-tools")); err != nil {
		t.Fatal(err)
	}
	if paths := testutil.WalkRel(t, dir); len(paths) != 0 {
		t.Errorf("no selected environment must write no file, got %v", paths)
	}
}

func TestEnvironment_NoSelectionNamesTheMissingKeyOnlyWhenSpecsReachTheTarget(t *testing.T) {
	buf := captureNotes(t)
	testutil.TempCwd(t)
	if err := emitBundle(t, &config.Config{}, environmentSpec(map[string]any{"install": "npm ci"})); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	for _, want := range []string{"ellipsis", "no environment file written", "x-ellipsis.environment"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("note should contain %q:\n%s", want, buf)
		}
	}

	quiet := captureNotes(t)
	if err := emitBundle(t, &config.Config{}, review("r", "", "Check."), settings(map[string]any{"reviewer": reviewer(nil)})); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if strings.Contains(quiet.String(), "environment") {
		t.Errorf("a review-only project must hear nothing about environments:\n%s", quiet)
	}
}

func TestEnvironment_NoSelectionFollowsOnUnsupported(t *testing.T) {
	testutil.TempCwd(t)
	captureNotes(t)
	err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{httpServer("internal-tools")}), &config.Config{OnUnsupported: "error"}, false)
	if err == nil || !strings.Contains(err.Error(), "x-ellipsis.environment") {
		t.Fatalf("want an error naming x-ellipsis.environment, got %v", err)
	}
	silent := captureNotes(t)
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{httpServer("internal-tools")}), &config.Config{OnUnsupported: "silent"}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if silent.Len() != 0 {
		t.Errorf("silent mode must print nothing, got %q", silent)
	}
}

// brokenEnvironments are x-ellipsis.environment values Ellipsis could not
// run or that the specs contradict, with words the diagnostic must use.
var brokenEnvironments = []struct {
	name   string
	env    any
	specs  []spec.Entry
	want   string
	extras string
}{
	{"not a mapping", "api-environment", nil, "x-ellipsis.environment must be a mapping", ""},
	{"no name", map[string]any{"repository": map[string]any{"name": "api-repo"}}, nil, "x-ellipsis.environment.name is not set", ""},
	{"name with a slash", selected(map[string]any{"name": "../escape"}), nil, "x-ellipsis.environment.name", ""},
	{"name with spaces", selected(map[string]any{"name": "api environment"}), nil, "x-ellipsis.environment.name", ""},
	{"no repository", map[string]any{"name": "api-environment"}, nil, "x-ellipsis.environment.repository is not set", ""},
	{"repository not a mapping", selected(map[string]any{"repository": "api-repo"}), nil, "x-ellipsis.environment.repository must be a mapping", ""},
	{"repository without a name", selected(map[string]any{"repository": map[string]any{"owner": "acme"}}), nil, "x-ellipsis.environment.repository.name is not set", ""},
	{"repository with an owner in its name", selected(map[string]any{"repository": map[string]any{"name": "acme/api-repo"}}), nil, "without the owner", ""},
	{"repository path escape", selected(map[string]any{"repository": map[string]any{"name": ".."}}), nil, "x-ellipsis.environment.repository.name", ""},
	{"repository with a space", selected(map[string]any{"repository": map[string]any{"name": "api repo"}}), nil, "x-ellipsis.environment.repository.name", ""},
	{"unknown repository key", selected(map[string]any{"repository": map[string]any{"name": "a", "branch": "x"}}), nil, "x-ellipsis.environment.repository.branch", ""},
	{"several repositories", selected(map[string]any{"repositories": []any{map[string]any{"name": "a"}, map[string]any{"name": "b"}}}), nil, "one repository", ""},
	{"unknown key", selected(map[string]any{"triggers": map[string]any{}}), nil, "x-ellipsis.environment.triggers", ""},
	{"unknown compute key", selected(map[string]any{"compute": map[string]any{"gpu": 1}}), nil, "x-ellipsis.environment.compute.gpu", ""},
	{"compute cpu is text", selected(map[string]any{"compute": map[string]any{"cpu": "two"}}), nil, "x-ellipsis.environment.compute.cpu", ""},
	{"variables not a list", selected(map[string]any{"variables": "NODE_ENV"}), nil, "x-ellipsis.environment.variables must be a list", ""},
	{"variable name starts with a digit", selected(map[string]any{"variables": []any{map[string]any{"name": "1TOKEN"}}}), nil, "x-ellipsis.environment.variables", ""},
	{"variable name reserved", selected(map[string]any{"variables": []any{map[string]any{"name": "ANTHROPIC_API_KEY"}}}), nil, "ANTHROPIC_API_KEY", ""},
	{"credential variable holds a value", selected(map[string]any{"variables": []any{map[string]any{"name": "NPM_TOKEN", "value": "npm_abc"}}}), nil, "omit value", ""},
	{"hooks not a mapping", selected(map[string]any{"hooks": "npm ci"}), nil, "x-ellipsis.environment.hooks must be a mapping", ""},
	{"unknown hook", selected(map[string]any{"hooks": map[string]any{"after_start": "x"}}), nil, "x-ellipsis.environment.hooks.after_start", ""},
	{"input with a glob", selected(map[string]any{"hooks": map[string]any{"build_base": map[string]any{"run": "x", "inputs": []any{"api-repo/*.json"}}}}), nil, "x-ellipsis.environment.hooks.build_base.inputs", ""},
	{"input outside the repository", selected(map[string]any{"hooks": map[string]any{"build_base": map[string]any{"run": "x", "inputs": []any{"other-repo/package.json"}}}}), nil, "api-repo/", ""},
	{"input that climbs out", selected(map[string]any{"hooks": map[string]any{"build_base": map[string]any{"run": "x", "inputs": []any{"api-repo/../secrets"}}}}), nil, "..", ""},
	{"absolute input", selected(map[string]any{"hooks": map[string]any{"build_base": map[string]any{"run": "x", "inputs": []any{"/etc/passwd"}}}}), nil, "api-repo/", ""},
	{"inputs without a script", selected(map[string]any{"hooks": map[string]any{"build_base": map[string]any{"inputs": []any{"api-repo/package.json"}}}}), nil, "hooks.build_base.run", ""},
	{"install and native build_base", selected(map[string]any{"hooks": map[string]any{"build_base": map[string]any{"run": "make"}}}),
		[]spec.Entry{environmentSpec(map[string]any{"install": "npm ci"})}, "install", ""},
	{"setup and native after_checkout", selected(map[string]any{"hooks": map[string]any{"after_checkout": "make"}}),
		[]spec.Entry{environmentSpec(map[string]any{"setup": "npm run build"})}, "setup", ""},
	{"mcp_servers names a custom server", selected(map[string]any{"mcp_servers": []any{map[string]any{"name": "x", "url": "https://x"}}}), nil, "slack or linear", ""},
	{"mcp_servers lists another integration", selected(map[string]any{"mcp_servers": []any{"jira"}}), nil, "slack or linear", ""},
}

func TestEnvironment_BrokenSelectionNotesAndWritesNoFile(t *testing.T) {
	for _, tc := range brokenEnvironments {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			buf := captureNotes(t)
			entries := append([]spec.Entry{settings(map[string]any{"environment": tc.env})}, tc.specs...)
			if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
				t.Fatalf("warn mode must not fail: %v", err)
			}
			emit.FlushCoverageNotes()
			if paths := testutil.WalkRel(t, dir); len(paths) != 0 {
				t.Errorf("no runnable file may be written, got %v", paths)
			}
			if !strings.Contains(buf.String(), tc.want) || !strings.Contains(buf.String(), "no environment file written") {
				t.Errorf("note should name %q and say no file was written, got:\n%s", tc.want, buf)
			}
		})
	}
}

func TestEnvironment_BrokenSelectionFailsUnderOnUnsupportedError(t *testing.T) {
	for _, tc := range brokenEnvironments {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			captureNotes(t)
			entries := append([]spec.Entry{settings(map[string]any{"environment": tc.env})}, tc.specs...)
			err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{OnUnsupported: "error"}, false)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error naming %q, got %v", tc.want, err)
			}
			if paths := testutil.WalkRel(t, dir); len(paths) != 0 {
				t.Errorf("no file may be written on error, got %v", paths)
			}
		})
	}
}

func TestEnvironment_BrokenSelectionLeavesTheReviewUnlinked(t *testing.T) {
	testutil.TempCwd(t)
	captureNotes(t)
	bad := selected(map[string]any{"repository": nil})
	delete(bad, "repository")
	if err := emitBundle(t, &config.Config{}, review("r", "", "Check."),
		settings(map[string]any{"environment": bad, "reviewer": reviewer(nil)})); err != nil {
		t.Fatal(err)
	}
	if got := readReview(t, reviewFile); strings.Contains(got, "environment:") {
		t.Errorf("a review must not point at an environment that was not written:\n%s", got)
	}
}

func TestEnvironment_ReportsEveryProblemAtOnce(t *testing.T) {
	testutil.TempCwd(t)
	buf := captureNotes(t)
	bad := map[string]any{"name": "bad name", "triggers": 1}
	if err := emitBundle(t, &config.Config{}, settings(map[string]any{"environment": bad})); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	for _, want := range []string{"x-ellipsis.environment.name", "x-ellipsis.environment.repository", "x-ellipsis.environment.triggers"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %q in:\n%s", want, buf)
		}
	}
}

func TestEnvironment_UnsupportedEnvironmentFieldsAreNoted(t *testing.T) {
	testutil.TempCwd(t)
	buf := captureNotes(t)
	env := environmentSpec(map[string]any{
		"install":       "npm ci",
		"setup-windows": "npm ci",
		"cleanup":       "docker compose down",
		"dev-commands":  []any{map[string]any{"name": "web", "command": "npm run dev"}},
		"terminals":     []any{map[string]any{"name": "dev", "command": "npm run dev"}},
	})
	if err := emitBundle(t, &config.Config{}, environmentSettings(nil), env); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	for _, field := range []string{"setup-windows", "cleanup", "dev-commands", "terminals"} {
		if !strings.Contains(buf.String(), field) {
			t.Errorf("a note must say %s has no effect:\n%s", field, buf)
		}
	}
	got := readEnvFile(t)
	for _, absent := range []string{"docker compose", "npm run dev", "web"} {
		if strings.Contains(got, absent) {
			t.Errorf("%q must not reach the file:\n%s", absent, got)
		}
	}
}

func TestEnvironment_LastEnvironmentSpecWinsPerField(t *testing.T) {
	testutil.TempCwd(t)
	first := environmentSpec(map[string]any{"install": "npm ci", "setup": "make a"})
	second := environmentSpec(map[string]any{"setup": "make b"})
	second.Name = "later"
	if err := emitBundle(t, &config.Config{}, environmentSettings(nil), first, second); err != nil {
		t.Fatal(err)
	}
	got := readEnvFile(t)
	if !strings.Contains(got, "npm ci") || !strings.Contains(got, "make b") || strings.Contains(got, "make a") {
		t.Errorf("later spec must win per field and keep the others:\n%s", got)
	}
}

func TestEnvironment_XEllipsisOverridesAnEnvironmentField(t *testing.T) {
	testutil.TempCwd(t)
	env := environmentSpec(map[string]any{
		"install":    "go mod download",
		"x-ellipsis": map[string]any{"install": "go mod download && go build ./..."},
	})
	if err := emitBundle(t, &config.Config{}, environmentSettings(nil), env); err != nil {
		t.Fatal(err)
	}
	if got := readEnvFile(t); !strings.Contains(got, "go mod download && go build ./...") {
		t.Errorf("x-ellipsis override not applied:\n%s", got)
	}
}

func TestEnvironment_SelectionAloneWritesACheckoutOnlyEnvironment(t *testing.T) {
	testutil.TempCwd(t)
	if err := emitBundle(t, &config.Config{}, environmentSettings(nil)); err != nil {
		t.Fatal(err)
	}
	want := header.Line(header.FormatYAML) + "ellipsis:\n  kind: environment\n  name: api-environment\nrepositories:\n  - name: api-repo\n"
	if got := readEnvFile(t); got != want {
		t.Errorf("mismatch\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

func TestEnvironment_OutputOverrideMovesTheFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	cfg := &config.Config{Outputs: map[string]config.Output{"ellipsis": {EnvironmentFile: ".ellipsis/environments/alt.yaml"}}}
	if err := emitBundle(t, cfg, environmentSettings(nil), environmentSpec(map[string]any{"install": "npm ci"})); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readReview(t, ".ellipsis/environments/alt.yaml"), "npm ci") {
		t.Error("override file lacks the install")
	}
	if _, err := os.Stat(filepath.Join(dir, environmentFile)); !os.IsNotExist(err) {
		t.Errorf("default path must stay empty when the override is set (%v)", err)
	}
}

func TestEnvironment_OutputOverrideOutsideEllipsisIsRefused(t *testing.T) {
	for _, path := range []string{"ci/environment.yaml", "../.ellipsis/x.yaml", "/abs/.ellipsis/x.yaml", ".ellipsis/environments/x.json", ".ellipsis/../ci/x.yaml"} {
		t.Run(path, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			captureNotes(t)
			cfg := &config.Config{OnUnsupported: "error", Outputs: map[string]config.Output{"ellipsis": {EnvironmentFile: path}}}
			err := emitBundle(t, cfg, environmentSettings(nil))
			if err == nil || !strings.Contains(err.Error(), "outputs.ellipsis.environment-file") {
				t.Fatalf("want an error naming outputs.ellipsis.environment-file, got %v", err)
			}
			if paths := testutil.WalkRel(t, dir); len(paths) != 0 {
				t.Errorf("no file may be written, got %v", paths)
			}
		})
	}
}

func TestEnvironment_TargetFiltersKeepSpecsOutOfTheFile(t *testing.T) {
	testutil.TempCwd(t)
	only := environmentSpec(map[string]any{"install": "make cursor-only", "targets": []any{"cursor"}})
	shared := environmentSpec(map[string]any{"setup": "make shared"})
	shared.Name = "shared"
	server := httpServer("cursor-docs")
	server.Meta["targets"] = []any{"cursor"}
	b := spec.NewBundle([]spec.Entry{only, shared, server, environmentSettings(nil), httpServer("tools")}).For("ellipsis")
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readEnvFile(t)
	if strings.Contains(got, "cursor-only") || strings.Contains(got, "cursor-docs") || !strings.Contains(got, "make shared") || !strings.Contains(got, "name: tools") {
		t.Errorf("target filter not honored:\n%s", got)
	}
}

func TestEnvironment_DryRunWritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)
	b := spec.NewBundle([]spec.Entry{environmentSettings(nil), environmentSpec(map[string]any{"install": "npm ci"}), httpServer("tools")})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, true); err != nil {
		t.Fatal(err)
	}
	if paths := testutil.WalkRel(t, dir); len(paths) != 0 {
		t.Errorf("dry run wrote %v", paths)
	}
}

func TestEnvironment_FileCarriesTheProvenanceHeader(t *testing.T) {
	testutil.TempCwd(t)
	if err := emitBundle(t, &config.Config{}, environmentSettings(nil)); err != nil {
		t.Fatal(err)
	}
	if got := readEnvFile(t); !header.Has(got) || !strings.HasPrefix(got, "# ") {
		t.Errorf("missing provenance header:\n%s", got)
	}
}

func TestEnvironment_SecretReferencesStayReferences(t *testing.T) {
	testutil.TempCwd(t)
	extra := map[string]any{"variables": []any{map[string]any{"name": "NPM_TOKEN"}}}
	server := mcpSpec("repo-tools", map[string]any{
		"command": "repo-tools",
		"env":     map[string]any{"API_TOKEN": "${REPO_TOOLS_TOKEN}"},
	})
	if err := emitBundle(t, &config.Config{}, environmentSettings(extra), httpServer("internal-tools"), server); err != nil {
		t.Fatal(err)
	}
	got := readEnvFile(t)
	for _, want := range []string{"Bearer ${TOOLS_TOKEN}", "API_TOKEN: ${REPO_TOOLS_TOKEN}", "- name: NPM_TOKEN\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("reference %q lost:\n%s", want, got)
		}
	}
	if strings.Contains(got, "value:") {
		t.Errorf("a secret variable must not carry a value:\n%s", got)
	}
}

// omittedServers are MCP specs Ellipsis cannot run as written, with the
// words the note must use. Each is left out rather than weakened.
var omittedServers = []struct {
	name string
	meta map[string]any
	want string
}{
	{"sse transport", map[string]any{"type": "sse", "url": "https://x.example/sse"}, "sse"},
	{"websocket transport", map[string]any{"type": "ws", "url": "wss://x.example"}, "ws"},
	{"stdio with cwd", map[string]any{"command": "node", "cwd": "tools"}, "cwd"},
	{"oauth", map[string]any{"type": "http", "url": "https://x.example/mcp", "oauth": map[string]any{"clientId": "abc"}}, "oauth"},
	{"no command", map[string]any{"type": "stdio"}, "command"},
	{"no url", map[string]any{"type": "http"}, "url"},
	{"relative script", map[string]any{"command": "node", "args": []any{"./scripts/server.js"}}, "/sandbox/api-repo/"},
	{"parent directory", map[string]any{"command": "node", "args": []any{"../tools/server.js"}}, "/sandbox/api-repo/"},
	{"home directory", map[string]any{"command": "node", "args": []any{"~/tools/server.js"}}, "/sandbox/api-repo/"},
	{"machine path", map[string]any{"command": "/Users/me/bin/server"}, "/sandbox/api-repo/"},
	{"windows path", map[string]any{"command": "C:\\tools\\server.exe"}, "/sandbox/api-repo/"},
	{"editor variable", map[string]any{"command": "node", "args": []any{"${workspaceFolder}/server.js"}}, "/sandbox/api-repo/"},
	{"another repository", map[string]any{"command": "node", "args": []any{"/sandbox/web-repo/server.js"}}, "web-repo"},
	{"climbing out of the checkout", map[string]any{"command": "node", "args": []any{"/sandbox/api-repo/../web-repo/server.js"}}, "/sandbox/api-repo/"},
	{"reserved integration name", map[string]any{"type": "http", "url": "https://x.example/mcp"}, "slack"},
	{"name outside letters digits dash underscore", map[string]any{"type": "http", "url": "https://x.example/mcp"}, "letters, digits"},
}

func omittedServerName(caseName string) string {
	switch caseName {
	case "reserved integration name":
		return "slack"
	case "name outside letters digits dash underscore":
		return "docs search"
	}
	return "broken"
}

func TestEnvironment_IncompatibleServersAreLeftOutWithANote(t *testing.T) {
	for _, tc := range omittedServers {
		t.Run(tc.name, func(t *testing.T) {
			testutil.TempCwd(t)
			buf := captureNotes(t)
			name := omittedServerName(tc.name)
			if err := emitBundle(t, &config.Config{}, environmentSettings(nil), httpServer("tools"), mcpSpec(name, tc.meta)); err != nil {
				t.Fatalf("warn mode must not fail: %v", err)
			}
			emit.FlushCoverageNotes()
			got := readEnvFile(t)
			if strings.Contains(got, "name: "+name+"\n") {
				t.Errorf("%s must be left out, got:\n%s", name, got)
			}
			if !strings.Contains(got, "name: tools\n") {
				t.Errorf("the other server must stay:\n%s", got)
			}
			if !strings.Contains(buf.String(), name) || !strings.Contains(buf.String(), tc.want) {
				t.Errorf("note should name %q and %q:\n%s", name, tc.want, buf)
			}
		})
	}
}

func TestEnvironment_IncompatibleServersFailUnderOnUnsupportedError(t *testing.T) {
	for _, tc := range omittedServers {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			captureNotes(t)
			name := omittedServerName(tc.name)
			err := emitBundle(t, &config.Config{OnUnsupported: "error"}, environmentSettings(nil), mcpSpec(name, tc.meta))
			if err == nil || !strings.Contains(err.Error(), name) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error naming %q and %q, got %v", name, tc.want, err)
			}
			if paths := testutil.WalkRel(t, dir); len(paths) != 0 {
				t.Errorf("no file may be written on error, got %v", paths)
			}
		})
	}
}

func TestEnvironment_IncompatibleServersAreQuietUnderSilent(t *testing.T) {
	testutil.TempCwd(t)
	buf := captureNotes(t)
	if err := emitBundle(t, &config.Config{OnUnsupported: "silent"}, environmentSettings(nil), mcpSpec("broken", map[string]any{"type": "sse", "url": "https://x.example/sse"})); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if buf.Len() != 0 {
		t.Errorf("silent mode must print nothing, got %q", buf)
	}
	if got := readEnvFile(t); strings.Contains(got, "broken") {
		t.Errorf("the server must still be left out:\n%s", got)
	}
}

func TestEnvironment_SandboxPathsAndPathNamesAreAccepted(t *testing.T) {
	testutil.TempCwd(t)
	servers := []spec.Entry{
		mcpSpec("tools", map[string]any{"command": "node", "args": []any{"/sandbox/api-repo/scripts/server.js"}}),
		mcpSpec("fs", map[string]any{"command": "npx", "args": []any{"-y", "@modelcontextprotocol/server-filesystem", "/sandbox/api-repo"}}),
		mcpSpec("py", map[string]any{"command": "/sandbox/api-repo/.venv/bin/python", "args": []any{"-m", "server"}}),
		mcpSpec("bin", map[string]any{"command": "/usr/local/bin/server"}),
	}
	if err := emitBundle(t, &config.Config{OnUnsupported: "error"}, append([]spec.Entry{environmentSettings(nil)}, servers...)...); err != nil {
		t.Fatal(err)
	}
	got := readEnvFile(t)
	for _, name := range []string{"tools", "fs", "py", "bin"} {
		if !strings.Contains(got, "name: "+name+"\n") {
			t.Errorf("server %s should be written:\n%s", name, got)
		}
	}
}

func TestEnvironment_ServerFieldsWithoutAnEllipsisKeyAreNoted(t *testing.T) {
	testutil.TempCwd(t)
	buf := captureNotes(t)
	server := mcpSpec("tools", map[string]any{
		"type": "http", "url": "https://x.example/mcp",
		"description": "Docs search", "timeout": 5000,
		"roots": []any{map[string]any{"uri": "file:///repo", "name": "repo"}},
	})
	if err := emitBundle(t, &config.Config{}, environmentSettings(nil), server); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	for _, field := range []string{"description", "timeout", "roots"} {
		if !strings.Contains(buf.String(), field) {
			t.Errorf("a note must say %s has no effect:\n%s", field, buf)
		}
	}
	got := readEnvFile(t)
	for _, absent := range []string{"Docs search", "5000", "roots"} {
		if strings.Contains(got, absent) {
			t.Errorf("%q must not reach the file:\n%s", absent, got)
		}
	}
	if !strings.Contains(got, "name: tools\n") {
		t.Errorf("the server itself must stay:\n%s", got)
	}
}

func TestEnvironment_XEllipsisOverridesAServerForTheSandbox(t *testing.T) {
	testutil.TempCwd(t)
	server := mcpSpec("repo-tools", map[string]any{
		"command":    "node",
		"args":       []any{"scripts/mcp-server.js"},
		"x-ellipsis": map[string]any{"args": []any{"/sandbox/api-repo/scripts/mcp-server.js"}},
	})
	if err := emitBundle(t, &config.Config{OnUnsupported: "error"}, environmentSettings(nil), server); err != nil {
		t.Fatal(err)
	}
	if got := readEnvFile(t); !strings.Contains(got, "- /sandbox/api-repo/scripts/mcp-server.js\n") {
		t.Errorf("the x-ellipsis args must replace the local ones:\n%s", got)
	}
}

func TestEnvironment_MCPServersAreWrittenWithoutAnEnvironmentSpec(t *testing.T) {
	testutil.TempCwd(t)
	if err := emitBundle(t, &config.Config{}, environmentSettings(nil), httpServer("tools")); err != nil {
		t.Fatal(err)
	}
	got := readEnvFile(t)
	if strings.Contains(got, "hooks:") || !strings.Contains(got, "mcp_servers:") {
		t.Errorf("want servers and no hooks:\n%s", got)
	}
}

func TestEnvironment_ProjectMCPFileIsNotUsed(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := emitBundle(t, &config.Config{}, environmentSettings(nil), httpServer("tools")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".mcp.json")); !os.IsNotExist(err) {
		t.Errorf("Ellipsis ignores .mcp.json, so none may be written (%v)", err)
	}
}

func TestEnvironment_DisabledServerIsLeftOutBecauseEllipsisHasNoSwitch(t *testing.T) {
	testutil.TempCwd(t)
	buf := captureNotes(t)
	off := mcpSpec("off", map[string]any{"type": "http", "url": "https://x.example/mcp", "disabled": true})
	if err := emitBundle(t, &config.Config{OnUnsupported: "error"}, environmentSettings(nil), httpServer("tools"), off); err != nil {
		t.Fatalf("a disabled server is unavailable either way, so error mode must not fail: %v", err)
	}
	emit.FlushCoverageNotes()
	got := readEnvFile(t)
	if strings.Contains(got, "name: off\n") || !strings.Contains(got, "name: tools\n") {
		t.Errorf("only the enabled server may be written:\n%s", got)
	}
	if !strings.Contains(buf.String(), "disabled") {
		t.Errorf("a note must say the disabled server was left out:\n%s", buf)
	}
}

func TestEnvironment_BrokenSelectionKeepsTheDeployedFileAndTheReviewLink(t *testing.T) {
	testutil.TempCwd(t)
	captureNotes(t)
	good := settings(map[string]any{"environment": selected(nil), "reviewer": reviewer(nil)})
	if err := emitBundle(t, &config.Config{}, review("r", "", "Check."), good, environmentSpec(map[string]any{"install": "npm ci"})); err != nil {
		t.Fatal(err)
	}
	deployed := readEnvFile(t)

	bad := selected(nil)
	delete(bad, "repository")
	broken := settings(map[string]any{"environment": bad, "reviewer": reviewer(nil)})
	if err := emitBundle(t, &config.Config{}, review("r", "", "Check."), broken, environmentSpec(map[string]any{"install": "npm ci"})); err != nil {
		t.Fatal(err)
	}
	if got := readEnvFile(t); got != deployed {
		t.Errorf("a broken selection must keep the deployed environment file:\n%s", got)
	}
	if got := readReview(t, reviewFile); !strings.Contains(got, "environment: api-environment") {
		t.Errorf("the review must stay linked to the environment that is still deployed:\n%s", got)
	}
}

func TestEnvironment_LiteralInAnXEllipsisOverrideIsRefused(t *testing.T) {
	const secret = "ELLIPSIS_OVERRIDE_SECRET_2188"
	for field, override := range map[string]map[string]any{
		"env":     {"command": "repo-tools", "env": map[string]any{"API_TOKEN": secret}},
		"headers": {"type": "http", "url": "https://x.example/mcp", "headers": map[string]any{"Authorization": "Bearer " + secret}},
	} {
		t.Run(field, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			buf := captureNotes(t)
			server := mcpSpec("tools", map[string]any{"command": "tools", "x-ellipsis": override})
			if err := emitBundle(t, &config.Config{}, environmentSettings(nil), httpServer("other"), server); err != nil {
				t.Fatal(err)
			}
			emit.FlushCoverageNotes()
			got := readEnvFile(t)
			if strings.Contains(got, secret) || strings.Contains(got, "name: tools\n") {
				t.Errorf("a literal under x-ellipsis must keep the server out of the committed file:\n%s", got)
			}
			if !strings.Contains(buf.String(), "tools") || !strings.Contains(buf.String(), "x-ellipsis") || strings.Contains(buf.String(), secret) {
				t.Errorf("the note should name the server and the override, never the value:\n%s", buf)
			}

			err := emitBundle(t, &config.Config{OnUnsupported: "error"}, environmentSettings(nil), server)
			if err == nil || strings.Contains(err.Error(), secret) {
				t.Errorf("error mode must refuse without printing the value, got %v", err)
			}
			_ = dir
		})
	}
}

func TestEnvironment_ReferencesInAnXEllipsisOverrideAreKept(t *testing.T) {
	testutil.TempCwd(t)
	server := mcpSpec("tools", map[string]any{
		"command":    "tools",
		"x-ellipsis": map[string]any{"env": map[string]any{"API_TOKEN": "${SANDBOX_TOKEN}"}},
	})
	if err := emitBundle(t, &config.Config{OnUnsupported: "error"}, environmentSettings(nil), server); err != nil {
		t.Fatal(err)
	}
	if got := readEnvFile(t); !strings.Contains(got, "API_TOKEN: ${SANDBOX_TOKEN}") {
		t.Errorf("an override reference must reach the file:\n%s", got)
	}
}

func TestEnvironment_LocalHomePathsAreRefused(t *testing.T) {
	testutil.TempCwd(t)
	captureNotes(t)
	err := emitBundle(t, &config.Config{OnUnsupported: "error"}, environmentSettings(nil),
		mcpSpec("tools", map[string]any{"command": "node", "args": []any{"/home/chema/tools/server.js"}}))
	if err == nil || !strings.Contains(err.Error(), "/sandbox/api-repo/") {
		t.Errorf("a home directory path must be refused with the sandbox form, got %v", err)
	}
}

func TestEnvironment_BrokenRenameKeepsTheDeployedName(t *testing.T) {
	testutil.TempCwd(t)
	captureNotes(t)
	cfg := &config.Config{Outputs: map[string]config.Output{"ellipsis": {EnvironmentFile: ".ellipsis/environments/ci.yaml"}}}
	good := settings(map[string]any{"environment": selected(nil), "reviewer": reviewer(nil)})
	if err := emitBundle(t, cfg, review("r", "", "Check."), good); err != nil {
		t.Fatal(err)
	}
	bad := settings(map[string]any{"environment": map[string]any{"name": "renamed"}, "reviewer": reviewer(nil)})
	if err := emitBundle(t, cfg, review("r", "", "Check."), bad); err != nil {
		t.Fatal(err)
	}
	if got := readReview(t, reviewFile); !strings.Contains(got, "environment: api-environment") {
		t.Errorf("review must name deployed environment:\n%s", got)
	}
}

func TestEnvironment_BrokenSelectionWithSeveralDeployedFilesStopsSync(t *testing.T) {
	testutil.TempCwd(t)
	captureNotes(t)
	for _, name := range []string{"first", "second"} {
		if err := emitBundle(t, &config.Config{}, environmentSettings(map[string]any{"name": name})); err != nil {
			t.Fatal(err)
		}
	}
	err := emitBundle(t, &config.Config{}, settings(map[string]any{"environment": map[string]any{}}), review("r", "", "Check."))
	if err == nil || !strings.Contains(err.Error(), "2 deployed environment files") {
		t.Errorf("ambiguous deployment must stop sync, got %v", err)
	}
	for _, name := range []string{"first", "second"} {
		if got := readReview(t, ".ellipsis/environments/"+name+".yaml"); !strings.Contains(got, "name: "+name) {
			t.Errorf("deployed environment changed:\n%s", got)
		}
	}
	if _, err := os.Stat(reviewFile); !os.IsNotExist(err) {
		t.Errorf("no review may be written before the environment is resolved, got %v", err)
	}
}
