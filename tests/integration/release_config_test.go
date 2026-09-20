package integration

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Release configs these tests guard. None of them runs on a pull
// request, so a regression in any of them first shows up at tag time.
var (
	goreleaserConfigPath = filepath.Join("..", "..", ".goreleaser.yml")
	releaseWorkflowPath  = filepath.Join("..", "..", ".github", "workflows", "release.yml")
	installWorkflowPath  = filepath.Join("..", "..", ".github", "workflows", "install.yml")
)

// workflowJob is the slice of a workflow job these tests read.
type workflowJob struct {
	Permissions map[string]string `yaml:"permissions"`
	Env         map[string]string `yaml:"env"`
	Strategy    struct {
		Matrix struct {
			OS []string `yaml:"os"`
		} `yaml:"matrix"`
	} `yaml:"strategy"`
	Steps []struct {
		Name string `yaml:"name"`
		Run  string `yaml:"run"`
	} `yaml:"steps"`
}

// workflowJobs parses a GitHub Actions workflow and returns its jobs.
func workflowJobs(t *testing.T, path string) map[string]workflowJob {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc struct {
		Jobs map[string]workflowJob `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return doc.Jobs
}

// workflowStep returns the run script of the named step in the named job.
func workflowStep(t *testing.T, path, job, step string) string {
	t.Helper()
	j, ok := workflowJobs(t, path)[job]
	if !ok {
		t.Fatalf("%s has no %q job", path, job)
	}
	for _, s := range j.Steps {
		if s.Name == step {
			return s.Run
		}
	}
	t.Fatalf("%s job %q has no %q step", path, job, step)
	return ""
}

// caskConfig returns the first homebrew_casks entry from .goreleaser.yml.
func caskConfig(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(goreleaserConfigPath)
	if err != nil {
		t.Fatalf("read %s: %v", goreleaserConfigPath, err)
	}
	var doc struct {
		HomebrewCasks []map[string]any `yaml:"homebrew_casks"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", goreleaserConfigPath, err)
	}
	if len(doc.HomebrewCasks) == 0 {
		t.Fatal("no homebrew_casks entry in .goreleaser.yml")
	}
	return doc.HomebrewCasks[0]
}

// TestGoreleaserCask_AvoidsTheDeprecatedPostflightStanza keeps the cask
// off Homebrew's deprecated `postflight` block.
//
// goreleaser's `hooks` key renders raw Ruby as `postflight do`, which
// Homebrew deprecated in favor of the declarative `postflight_steps`
// DSL. Every `brew` command touching the tap then prints a warning
// naming our tap and asking the user to report it to us (#933).
//
// Nothing else catches a regression here. The cask is only generated at
// tag time, so switching back to `hooks` would look fine in CI and
// surface as a warning on every user's machine after the release.
func TestGoreleaserCask_AvoidsTheDeprecatedPostflightStanza(t *testing.T) {
	cask := caskConfig(t)
	if _, ok := cask["hooks"]; ok {
		t.Error("homebrew_casks has a `hooks` key; goreleaser renders it as Homebrew's deprecated `postflight do`, so use `custom_block` with `postflight_steps` instead")
	}
	block, _ := cask["custom_block"].(string)
	if !strings.Contains(block, "postflight_steps do") {
		t.Errorf("custom_block does not declare postflight_steps:\n%s", block)
	}
}

// TestGoreleaserCask_StripsQuarantine pins the reason the block exists
// at all.
//
// Homebrew propagates com.apple.quarantine from the downloaded archive
// to the extracted files, and the `binary` artifact only chmods +x, so
// it never strips it. Without this call macOS Gatekeeper blocks the
// unsigned binary on first run. Deleting the block is the obvious way
// to silence the deprecation warning and it breaks every macOS install.
func TestGoreleaserCask_StripsQuarantine(t *testing.T) {
	block, _ := caskConfig(t)["custom_block"].(string)
	for _, want := range []string{"xattr", "com.apple.quarantine"} {
		if !strings.Contains(block, want) {
			t.Errorf("custom_block no longer strips quarantine, missing %q:\n%s", want, block)
		}
	}
}

// TestGoreleaserCask_EscapesHomebrewsStagedPathToken guards a failure
// that can only happen at tag time.
//
// goreleaser renders the cask and then runs the whole file through its
// own Go template engine a second time. Homebrew's `{{staged_path}}` is
// not a goreleaser variable, so a bare token aborts the release with
// `function "staged_path" not defined`. Wrapping it in a goreleaser
// template that yields the literal string survives that pass.
//
// Verified by reverting the wrapper locally: `goreleaser release
// --snapshot` fails with exactly that error, and no CI job would have
// caught it, because CI never renders the cask.
func TestGoreleaserCask_EscapesHomebrewsStagedPathToken(t *testing.T) {
	block, _ := caskConfig(t)["custom_block"].(string)
	const wrapped = `{{ "{{staged_path}}" }}`
	if !strings.Contains(block, wrapped) {
		t.Fatalf("custom_block must reference Homebrew's staged path as %s:\n%s", wrapped, block)
	}
	// A bare token anywhere outside the wrapper breaks the release the
	// same way, so check none survives once the wrapped form is removed.
	if rest := strings.ReplaceAll(block, wrapped, ""); strings.Contains(rest, "{{staged_path}}") {
		t.Errorf("custom_block has a bare {{staged_path}}; goreleaser templates the rendered cask, so it fails the release at tag time:\n%s", block)
	}
}

// TestReleaseWorkflow_PublishesWithProvenance keeps the npm publish
// attested.
//
// A provenance statement needs two things the workflow owns: the
// `--provenance` flag, and an OIDC token the job can only mint with
// `id-token: write`. Dropping either leaves the publish working and the
// package page without the attestation, which nothing else notices.
func TestReleaseWorkflow_PublishesWithProvenance(t *testing.T) {
	job, ok := workflowJobs(t, releaseWorkflowPath)["npm"]
	if !ok {
		t.Fatal("release.yml has no npm job")
	}
	if job.Permissions["id-token"] != "write" {
		t.Errorf("the npm job needs `id-token: write` to mint the OIDC token a provenance statement is signed against, got %q", job.Permissions["id-token"])
	}
	if publish := workflowStep(t, releaseWorkflowPath, "npm", "Publish"); !strings.Contains(publish, "npm publish --access public --provenance") {
		t.Errorf("the Publish step no longer passes --provenance:\n%s", publish)
	}
}

// TestReleaseWorkflow_PinsAnNpmCliThatSupportsProvenance guards the
// version the publish runs on.
//
// npm documents 9.5.0 as the floor for `--provenance`; older clients
// ignore the flag rather than failing, so a downgrade would silently
// ship an unattested package. setup-node's bundled npm is whatever the
// Node line ships that week, hence the explicit pin.
func TestReleaseWorkflow_PinsAnNpmCliThatSupportsProvenance(t *testing.T) {
	job := workflowJobs(t, releaseWorkflowPath)["npm"]
	pin := job.Env["NPM_CLI_VERSION"]
	if pin == "" {
		t.Fatal("the npm job no longer pins NPM_CLI_VERSION; setup-node would decide which npm signs the release")
	}
	if !strings.Contains(workflowStep(t, releaseWorkflowPath, "npm", "Pin the npm CLI that publishes"), "NPM_CLI_VERSION") {
		t.Error("the pin step does not install NPM_CLI_VERSION, so the pin is decorative")
	}
	const floor = "9.5.0"
	if compareVersions(t, pin, floor) < 0 {
		t.Errorf("NPM_CLI_VERSION is %s but npm needs %s or newer for --provenance", pin, floor)
	}
}

// compareVersions orders two dotted numeric versions.
func compareVersions(t *testing.T, a, b string) int {
	t.Helper()
	split := func(v string) []int {
		parts := strings.Split(v, ".")
		out := make([]int, len(parts))
		for i, p := range parts {
			n, err := strconv.Atoi(p)
			if err != nil {
				t.Fatalf("version %q is not dotted numbers: %v", v, err)
			}
			out[i] = n
		}
		return out
	}
	x, y := split(a), split(b)
	for i := 0; i < len(x) && i < len(y); i++ {
		if x[i] != y[i] {
			if x[i] < y[i] {
				return -1
			}
			return 1
		}
	}
	return len(x) - len(y)
}

// TestReleaseWorkflow_DistributionChecksRetryAStaleReplica keeps the
// guard honest about propagation delay.
//
// Both checks read a replica that lags the write they verify: on v0.62.0
// `npm view` reported the version absent nine seconds after a publish
// that had landed. A single read turns that lag into a red release, and
// a check that is sometimes wrong gets dismissed when it is right.
func TestReleaseWorkflow_DistributionChecksRetryAStaleReplica(t *testing.T) {
	for _, step := range []string{"Homebrew cask serves this tag", "npm serves this tag"} {
		script := workflowStep(t, releaseWorkflowPath, "distribution", step)
		if !strings.Contains(script, "for attempt in") || !strings.Contains(script, "sleep") {
			t.Errorf("%q reads its channel once; a replica that has not caught up then fails the release:\n%s", step, script)
		}
		if !strings.Contains(script, "delay * 2") {
			t.Errorf("%q retries without backoff; hammering the replica does not make it catch up faster:\n%s", step, script)
		}
		// The retry must not swallow the case the guard exists for.
		if !strings.Contains(script, "::error::") {
			t.Errorf("%q no longer errors on a channel that stays stale:\n%s", step, script)
		}
		if !strings.Contains(script, "::warning::") || !strings.Contains(script, "exit 0") {
			t.Errorf("%q no longer warns and passes when its token is absent, which is how forks release:\n%s", step, script)
		}
	}
}

// TestInstallWorkflow_NpmJobsCoverEveryPublishedPlatform keeps the npm
// coverage matching what the package declares it supports.
//
// The wrapper ships `os: [darwin, linux, win32]`, and macOS is the
// primary platform for this tool, so leaving it out of either npm job
// means the install path most users take is never run.
func TestInstallWorkflow_NpmJobsCoverEveryPublishedPlatform(t *testing.T) {
	jobs := workflowJobs(t, installWorkflowPath)
	for _, name := range []string{"npm", "published"} {
		job, ok := jobs[name]
		if !ok {
			t.Errorf("install.yml has no %q job", name)
			continue
		}
		for _, want := range []string{"ubuntu-latest", "macos-latest", "windows-latest"} {
			if !slices.Contains(job.Strategy.Matrix.OS, want) {
				t.Errorf("install.yml job %q does not run on %s, got %v", name, want, job.Strategy.Matrix.OS)
			}
		}
	}
}

// TestInstallWorkflow_PublishedJobInstallsFromTheRegistry pins what the
// job is for.
//
// Packing the working tree carries the 0.0.0-dev placeholder, which
// sends resolveVersion down its "fall back to the latest release"
// branch. Only a package installed from the registry carries a real
// version and exercises the package.json branch, which is the one every
// user hits. Rewriting this job to pack the tree again would look like a
// simplification and would delete the coverage.
func TestInstallWorkflow_PublishedJobInstallsFromTheRegistry(t *testing.T) {
	job, ok := workflowJobs(t, installWorkflowPath)["published"]
	if !ok {
		t.Fatal("install.yml has no published job")
	}
	var scripts strings.Builder
	for _, step := range job.Steps {
		scripts.WriteString(step.Run)
		scripts.WriteString("\n")
	}
	all := scripts.String()
	if strings.Contains(all, "npm pack") || strings.Contains(all, ".tgz") {
		t.Errorf("the published job installs a packed tree, which carries 0.0.0-dev and tests the wrong resolveVersion branch:\n%s", all)
	}
	if !strings.Contains(all, "npm install -g agnostic-ai@") {
		t.Errorf("the published job does not install a published version from the registry:\n%s", all)
	}
	if !strings.Contains(all, "--version") {
		t.Errorf("the published job never runs the installed binary:\n%s", all)
	}
	// Triggered by a release, this job races the same replica lag the
	// distribution guard hit, so it has to wait the same way.
	if !strings.Contains(all, "for attempt in") || !strings.Contains(all, "sleep") {
		t.Errorf("the published job does not wait for the registry, so a run right after a release fails on propagation delay:\n%s", all)
	}
}
