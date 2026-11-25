package integration

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const copilotSetupWorkflow = ".github/workflows/copilot-setup-steps.yml"
const copilotEnvironmentSpec = ".agnostic-ai/environments/dev.yaml"

func TestCopilotSetupSteps_GeneratesAWorkflowOnlyAfterOptIn(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	run := func(args ...string) string {
		t.Helper()
		out, err := ellipsisCommand(t, binary, dir, args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return out
	}
	const config = "version: 1\ntargets: [copilot]\ngitignore:\n  enabled: true\n"
	const spec = "install: npm ci\n"
	writeProjectFile(t, dir, "agnostic-ai.yaml", config)
	writeProjectFile(t, dir, copilotEnvironmentSpec, spec)
	out := run("sync")
	if strings.Count(out, "x-copilot.setup-steps") != 1 {
		t.Errorf("sync should name the opt-in once:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, copilotSetupWorkflow)); !os.IsNotExist(err) {
		t.Errorf("a workflow exists without opt-in: %v", err)
	}

	writeProjectFile(t, dir, copilotEnvironmentSpec, spec+"x-copilot:\n  setup-steps: true\n")
	run("sync")
	body := readProjectFile(t, dir, copilotSetupWorkflow)
	type step struct {
		Uses string
		Run  string
	}
	var doc struct {
		On   map[string]any
		Jobs map[string]struct {
			RunsOn         string `yaml:"runs-on"`
			Permissions    map[string]string
			TimeoutMinutes int `yaml:"timeout-minutes"`
			Steps          []step
		}
	}
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	job, ok := doc.Jobs["copilot-setup-steps"]
	if len(doc.Jobs) != 1 || !ok || job.RunsOn != "ubuntu-latest" || job.Permissions["contents"] != "read" {
		t.Errorf("unexpected jobs: %+v", doc.Jobs)
	}
	if len(doc.On) != 1 || doc.On["workflow_dispatch"] == nil {
		t.Errorf("unexpected triggers: %+v", doc.On)
	}
	pinned := regexp.MustCompile(`^actions/checkout@[0-9a-f]{40}$`)
	if len(job.Steps) != 2 || !pinned.MatchString(job.Steps[0].Uses) || job.Steps[1].Run != "npm ci" {
		t.Errorf("unexpected steps: %+v", job.Steps)
	}
	run("sync", "--check")
	run("lint")

	const override = "df4cb1c069e1874edd31b4311f1884172cec0e10"
	writeProjectFile(t, dir, copilotEnvironmentSpec, spec+`x-copilot:
  setup-steps:
    runs-on: ubuntu-24.04
    timeout-minutes: 30
    checkout: `+override+`
    steps:
      - run: node --version
`)
	run("sync")
	body = readProjectFile(t, dir, copilotSetupWorkflow)
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	job = doc.Jobs["copilot-setup-steps"]
	if job.RunsOn != "ubuntu-24.04" || job.TimeoutMinutes != 30 {
		t.Errorf("unexpected job settings: %+v", job)
	}
	if len(job.Steps) != 3 || job.Steps[0].Uses != "actions/checkout@"+override || job.Steps[1].Run != "node --version" || job.Steps[2].Run != "npm ci" {
		t.Errorf("unexpected steps: %+v", job.Steps)
	}
	for _, commit := range []string{"", "  commit: [copilot:environments]\n"} {
		writeProjectFile(t, dir, "agnostic-ai.yaml", config+commit)
		run("sync")
		ignore := readProjectFile(t, dir, ".gitignore")
		if strings.Contains(ignore, copilotSetupWorkflow) || strings.Contains(ignore, "/.github/workflows/\n") {
			t.Errorf("managed block hides workflow:\n%s", ignore)
		}
	}
	run("sync", "--check")

	writeProjectFile(t, dir, copilotEnvironmentSpec, spec+"x-copilot:\n  setup-steps: false\n")
	run("sync")
	if _, err := os.Stat(filepath.Join(dir, copilotSetupWorkflow)); !os.IsNotExist(err) {
		t.Errorf("generated workflow remains after disabling setup steps: %v", err)
	}
}

func TestCopilotSetupSteps_LeavesAHandWrittenWorkflowAlone(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [copilot]\noutputs:\n  copilot:\n    provenance-header: false\n")
	writeProjectFile(t, dir, copilotEnvironmentSpec, "install: npm ci\nx-copilot:\n  setup-steps: true\n")
	const native = "# Hand written\nname: Custom setup\non: workflow_dispatch\n"
	writeProjectFile(t, dir, copilotSetupWorkflow, native)
	for _, args := range [][]string{{"sync"}, {"doctor", "--fix", "--checks", "project"}} {
		out, err := ellipsisCommand(t, binary, dir, args...)
		if err == nil || !strings.Contains(filepath.ToSlash(out), copilotSetupWorkflow) || !strings.Contains(out, "hand-written file") {
			t.Errorf("%v did not refuse the hand-written workflow: %v\n%s", args, err, out)
		}
		if got := readProjectFile(t, dir, copilotSetupWorkflow); got != native {
			t.Errorf("%v changed the hand-written workflow: %q", args, got)
		}
	}
	if out, err := ellipsisCommand(t, binary, dir, "import", "copilot"); err != nil {
		t.Fatalf("import copilot: %v\n%s", err, out)
	}
	if got := readProjectFile(t, dir, copilotSetupWorkflow); got != native {
		t.Errorf("workflow changed: %q", got)
	}
	envs, err := os.ReadDir(filepath.Join(dir, ".agnostic-ai/environments"))
	if err != nil || len(envs) != 1 || envs[0].Name() != "dev.yaml" {
		t.Errorf("import should leave environment specs alone: %v, %v", envs, err)
	}
}

func TestCopilotSetupSteps_LintReportsInvalidJobOptions(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [copilot]\n")
	writeProjectFile(t, dir, copilotEnvironmentSpec, `install: npm ci
x-copilot:
  setup-steps:
    timeout-minutes: 60
    uses: some/ignored-workflow
    checkout: v7
    runs-on: [linux, true]
    permissions: {contents: 1}
    services: redis
    snapshot: {version: '2.*'}
`)
	out, err := ellipsisCommand(t, binary, dir, "lint")
	fields := []string{"timeout-minutes", "uses", "checkout", "runs-on", "permissions", "services", "snapshot"}
	if err == nil || strings.Count(out, "LINT065 [error]") != len(fields) {
		t.Errorf("lint should report %d invalid options: %v\n%s", len(fields), err, out)
	}
	for _, field := range fields {
		if !strings.Contains(out, field) {
			t.Errorf("lint omitted %q:\n%s", field, out)
		}
	}
	if _, err := ellipsisCommand(t, binary, dir, "sync"); err == nil {
		t.Error("sync should refuse invalid options")
	}
}

func TestCopilotSetupSteps_GitignoreKeepsTheWorkflowVisibleBesideOtherOutputs(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	cmd := exec.Command("git", "init", "-q", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [copilot]\noutputs:\n  copilot:\n    instructions-dir: .github/workflows\ngitignore:\n  enabled: true\n")
	writeProjectFile(t, dir, copilotEnvironmentSpec, "install: npm ci\nx-copilot:\n  setup-steps: true\n")
	writeProjectFile(t, dir, ".agnostic-ai/rules/go.md", "---\nname: go\nglobs: '**/*.go'\n---\nFormat Go files.\n")
	if out, err := ellipsisCommand(t, binary, dir, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "-C", dir, "check-ignore", "--", copilotSetupWorkflow)
	out, err := cmd.CombinedOutput()
	exit, ok := errors.AsType[*exec.ExitError](err)
	if !ok || exit.ExitCode() != 1 || len(out) != 0 {
		t.Errorf("workflow is ignored or Git failed: %v\n%s", err, out)
	}
}

func TestCopilotSetupSteps_UpdatesGeneratedWorkflowWithoutAHeader(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	const path = "preview/setup.yml"
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [copilot]\noutputs:\n  copilot:\n    setup-file: "+path+"\n    provenance-header: false\n")
	for _, install := range []string{"npm ci", "pnpm install --frozen-lockfile"} {
		writeProjectFile(t, dir, copilotEnvironmentSpec, "install: "+install+"\nx-copilot:\n  setup-steps: true\n")
		if out, err := ellipsisCommand(t, binary, dir, "sync"); err != nil {
			t.Fatalf("sync: %v\n%s", err, out)
		}
		body := readProjectFile(t, dir, path)
		if strings.Contains(body, "Generated by agnostic-ai") || !strings.Contains(body, install) {
			t.Errorf("unexpected workflow:\n%s", body)
		}
	}
	if out, err := ellipsisCommand(t, binary, dir, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}
	before := readProjectFile(t, dir, path)
	clone := t.TempDir()
	for _, rel := range []string{"agnostic-ai.yaml", copilotEnvironmentSpec, path} {
		writeProjectFile(t, clone, rel, readProjectFile(t, dir, rel))
	}
	if out, err := ellipsisCommand(t, binary, clone, "sync"); err != nil {
		t.Fatalf("sync in a checkout without a ledger: %v\n%s", err, out)
	}
	if got := readProjectFile(t, clone, path); got != before {
		t.Errorf("sync changed the workflow in the fresh checkout:\n%s", got)
	}
}

func TestCopilotSetupSteps_PreservesNativeOptionShapes(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [copilot]\n")
	writeProjectFile(t, dir, copilotEnvironmentSpec, `install: npm ci
x-copilot:
  setup-steps:
    runs-on:
      group: setup
      labels: [linux, '${{ vars.RUNNER_LABEL }}']
    permissions: read-all
    services: ${{ fromJSON(vars.SETUP_SERVICES) }}
    snapshot:
      image-name: project-setup
      version: 2.*
`)
	if out, err := ellipsisCommand(t, binary, dir, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	var doc struct {
		Jobs map[string]map[string]any
	}
	body := readProjectFile(t, dir, copilotSetupWorkflow)
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"runs-on": map[string]any{
			"group":  "setup",
			"labels": []any{"linux", "${{ vars.RUNNER_LABEL }}"},
		},
		"permissions": "read-all",
		"services":    "${{ fromJSON(vars.SETUP_SERVICES) }}",
		"snapshot":    map[string]any{"image-name": "project-setup", "version": "2.*"},
	}
	for key, value := range want {
		if got := doc.Jobs["copilot-setup-steps"][key]; !reflect.DeepEqual(got, value) {
			t.Errorf("%s = %#v, want %#v", key, got, value)
		}
	}
}

func TestCopilotSetupSteps_UsesSharedSpecsBeforeTargetRouting(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [copilot, openhands]\n")
	const shared = "name: dev\ninstall: echo shared-install\n"
	writeProjectFile(t, dir, copilotEnvironmentSpec, shared+`x-copilot:
  setup-steps:
    steps:
      - run: echo shared-step
`)
	writeProjectFile(t, dir, ".agnostic-ai/local/environments/dev.yaml", `name: dev
targets: [openhands]
install: echo private-install
x-copilot:
  setup-steps:
    timeout-minutes: 60
    steps:
      - run: echo private-step
`)
	writeProjectFile(t, dir, ".agnostic-ai/local/environments/extra.yaml", `name: extra
targets: [copilot]
install: echo private-extra
x-copilot:
  setup-steps: true
`)
	for _, args := range [][]string{{"sync"}, {"sync", "--check"}, {"lint"}} {
		if out, err := ellipsisCommand(t, binary, dir, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	body := readProjectFile(t, dir, copilotSetupWorkflow)
	if !strings.Contains(body, "shared-install") || !strings.Contains(body, "shared-step") || strings.Contains(body, "private-") {
		t.Errorf("workflow should use only shared specs:\n%s", body)
	}
	if openHands := readProjectFile(t, dir, ".openhands/setup.sh"); !strings.Contains(openHands, "private-install") {
		t.Errorf("OpenHands lost its local override:\n%s", openHands)
	}

	writeProjectFile(t, dir, copilotEnvironmentSpec, shared)
	if out, err := ellipsisCommand(t, binary, dir, "sync"); err != nil {
		t.Fatalf("sync without shared opt-in: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, copilotSetupWorkflow)); !os.IsNotExist(err) {
		t.Errorf("a local spec enabled the workflow: %v", err)
	}
}
