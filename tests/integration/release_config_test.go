package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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
	Steps []workflowStep `yaml:"steps"`
}

// workflowStep is the slice of a workflow step these tests read. `with`
// holds ints as well as strings (`fetch-depth: 0`), hence `any`.
type workflowStep struct {
	Name string            `yaml:"name"`
	ID   string            `yaml:"id"`
	If   string            `yaml:"if"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	With map[string]any    `yaml:"with"`
	Env  map[string]string `yaml:"env"`
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

// workflowRun returns the run script of the named step in the named job.
func workflowRun(t *testing.T, path, job, step string) string {
	t.Helper()
	s, _ := workflowStepAt(t, path, job, step)
	return s.Run
}

// workflowStepAt returns the named step of the named job and its position
// in the job, so a test can assert one step runs before another.
func workflowStepAt(t *testing.T, path, job, step string) (workflowStep, int) {
	t.Helper()
	j, ok := workflowJobs(t, path)[job]
	if !ok {
		t.Fatalf("%s has no %q job", path, job)
	}
	for i, s := range j.Steps {
		if s.Name == step {
			return s, i
		}
	}
	t.Fatalf("%s job %q has no %q step", path, job, step)
	return workflowStep{}, -1
}

// Names the tap credential guards below share, so a rename touches one
// place instead of drifting between them.
const (
	tapMintStep     = "Mint the Homebrew tap token from the GitHub App"
	tapAppIDSecret  = "HOMEBREW_TAP_APP_ID"
	tapAppKeySecret = "HOMEBREW_TAP_APP_PRIVATE_KEY"
	tapPATSecret    = "HOMEBREW_TAP_TOKEN"
)

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
// A provenance statement needs two things the release owns: the
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
	if publish := workflowRun(t, releaseWorkflowPath, "npm", "Publish"); !strings.Contains(publish, npmPublishScript) {
		t.Errorf("the Publish step no longer runs %s:\n%s", npmPublishScript, publish)
	}
	script := readRepoFile(t, npmPublishScript)
	if !strings.Contains(script, "npm publish --access public --provenance") {
		t.Errorf("%s no longer passes --provenance", npmPublishScript)
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
	if !strings.Contains(workflowRun(t, releaseWorkflowPath, "npm", "Pin the npm CLI that publishes"), "NPM_CLI_VERSION") {
		t.Error("the pin step does not install NPM_CLI_VERSION, so the pin is decorative")
	}
	const floorMajor, floorMinor = 9, 5
	var major, minor, patch int
	if _, err := fmt.Sscanf(pin, "%d.%d.%d", &major, &minor, &patch); err != nil {
		t.Fatalf("NPM_CLI_VERSION is %q, which is not an exact version; a range would let the signing client drift: %v", pin, err)
	}
	if major < floorMajor || (major == floorMajor && minor < floorMinor) {
		t.Errorf("NPM_CLI_VERSION is %s but npm needs %d.%d or newer for --provenance", pin, floorMajor, floorMinor)
	}
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
		script := workflowRun(t, releaseWorkflowPath, "distribution", step)
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
		// An absent credential must say so and pass, never fail the
		// release; that is how a fork releases. The two steps differ on
		// how loud to be. No npm token means nothing was published, so
		// that is a warning. No tap credential is the steady state here:
		// Chemaclass/homebrew-tap updates its own cask on a schedule with
		// its own GITHUB_TOKEN, so a warning every release would train
		// everyone to ignore the one that matters.
		if !strings.Contains(script, "::warning::") && !strings.Contains(script, "::notice::") {
			t.Errorf("%q says nothing when its credential is absent:\n%s", step, script)
		}
		if !strings.Contains(script, "exit 0") {
			t.Errorf("%q no longer passes when its credential is absent, which is how forks release:\n%s", step, script)
		}
	}
}

// TestReleaseWorkflow_DistributionChecksOutRepository guards the source files
// both distribution checks read after the build jobs finish.
//
// Jobs do not share a workspace. Without a checkout, the Homebrew check can
// pass without source files, then the npm check fails when it sources
// scripts/npm-publish.sh or reads npm/lib/platforms.js.
func TestReleaseWorkflow_DistributionChecksOutRepository(t *testing.T) {
	checkout, checkoutAt := workflowStepAt(t, releaseWorkflowPath, "distribution", "Checkout")
	if checkout.Uses != "actions/checkout@v7" {
		t.Errorf("the distribution checkout uses %q, want actions/checkout@v7", checkout.Uses)
	}
	for _, name := range []string{"Homebrew cask serves this tag", "npm serves this tag"} {
		_, checkAt := workflowStepAt(t, releaseWorkflowPath, "distribution", name)
		if checkoutAt > checkAt {
			t.Errorf("Checkout runs after %q, so the check cannot read repository files", name)
		}
	}
}

// TestReleaseWorkflow_MintsTheTapTokenWithoutRequiringTheApp guards the
// one thing this credential change must never do: fail a release.
//
// `actions/create-github-app-token` calls `core.setFailed` when its
// identifier or its private key is empty, so an ungated mint step turns
// an absent App into a red release, on every fork too. The `secrets`
// context is not readable from a step `if:`, so the presence flag has to
// be captured in the job `env`, the same trap the npm job hit with
// setup-node writing a placeholder into `.npmrc`.
//
// Nothing else catches this. The job only runs on a tag.
func TestReleaseWorkflow_MintsTheTapTokenWithoutRequiringTheApp(t *testing.T) {
	job := workflowJobs(t, releaseWorkflowPath)["goreleaser"]
	flag := job.Env["HOMEBREW_TAP_APP_CONFIGURED"]
	if !strings.Contains(flag, tapAppIDSecret) || !strings.Contains(flag, tapAppKeySecret) {
		t.Fatalf("the goreleaser job does not record whether both App secrets are set, so a step `if:` cannot test them, got %q", flag)
	}
	mint, mintAt := workflowStepAt(t, releaseWorkflowPath, "goreleaser", tapMintStep)
	if !strings.Contains(mint.If, "HOMEBREW_TAP_APP_CONFIGURED") {
		t.Errorf("the mint step is not gated on the App being configured, so a release without the App fails on an empty app-id, got %q", mint.If)
	}
	if strings.Contains(mint.If, "secrets.") {
		t.Errorf("the mint step reads `secrets` from a step `if:`, which GitHub rejects as an unrecognized named-value, got %q", mint.If)
	}
	if mint.ID == "" {
		t.Fatal("the mint step has no id, so nothing can read its token output")
	}
	_, releaseAt := workflowStepAt(t, releaseWorkflowPath, "goreleaser", "Run GoReleaser")
	if mintAt > releaseAt {
		t.Errorf("the mint step runs after GoReleaser, so its token output is empty and the cask push silently falls back to the PAT")
	}
}

// TestReleaseWorkflow_FallsBackToTheStoredTapToken keeps the release
// shipping through whichever credential exists.
//
// The App is not created yet, so the PAT is still the live credential;
// once the App lands the PAT goes away. Reading only one of the two
// breaks the release in one of those two states, and a lost token here
// does not fail the run, it just stops pushing the cask, which is how a
// stale cask went unnoticed for ten releases (#920).
func TestReleaseWorkflow_FallsBackToTheStoredTapToken(t *testing.T) {
	mint, _ := workflowStepAt(t, releaseWorkflowPath, "goreleaser", tapMintStep)
	release, _ := workflowStepAt(t, releaseWorkflowPath, "goreleaser", "Run GoReleaser")
	token := release.Env[tapPATSecret]
	if want := "steps." + mint.ID + ".outputs.token"; !strings.Contains(token, want) {
		t.Errorf("GoReleaser does not read the minted token (%s), so the App would be set up and unused: %q", want, token)
	}
	if !strings.Contains(token, "secrets."+tapPATSecret) {
		t.Errorf("GoReleaser has no fallback to the stored %s, so the cask stops being pushed until the App exists: %q", tapPATSecret, token)
	}
}

// TestReleaseWorkflow_ScopesTheTapTokenToTheTap pins the reason a
// GitHub App beats the PAT it replaces.
//
// The action defaults the installation to the current repository, which
// is not the one the cask is pushed to, so dropping `owner` and
// `repositories` breaks the push. It is also the whole security
// argument: a classic PAT with `repo` writes everywhere the owner can,
// while this token reaches the tap and nothing else.
//
// The pin is a major tag to match every other action in this repo.
func TestReleaseWorkflow_ScopesTheTapTokenToTheTap(t *testing.T) {
	mint, _ := workflowStepAt(t, releaseWorkflowPath, "goreleaser", tapMintStep)
	if !regexp.MustCompile(`^actions/create-github-app-token@v\d+$`).MatchString(mint.Uses) {
		t.Errorf("the mint step must use actions/create-github-app-token pinned to a major tag like the rest of this repo, got %q", mint.Uses)
	}
	for key, want := range map[string]string{
		"owner":               "Chemaclass",
		"repositories":        "homebrew-tap",
		"permission-contents": "write",
	} {
		if got, _ := mint.With[key].(string); got != want {
			t.Errorf("the mint step sets %s to %q, want %q; the token must reach the tap and only the tap", key, got, want)
		}
	}
	id, _ := mint.With["app-id"].(string)
	key, _ := mint.With["private-key"].(string)
	if !strings.Contains(id, tapAppIDSecret) || !strings.Contains(key, tapAppKeySecret) {
		t.Errorf("the mint step does not read %s and %s, so it signs with something other than the App this repo documents: app-id=%q private-key=%q", tapAppIDSecret, tapAppKeySecret, id, key)
	}
}

// TestReleaseWorkflow_DistributionAcceptsEitherTapCredential keeps the
// only check that proves the push landed from going blind.
//
// The guard warns and exits 0 when it believes no credential was set,
// because that is how a fork releases. Left testing the PAT alone, it
// would take that branch on every App-only release and stop verifying
// the tap, which is exactly the silence #920 is about.
func TestReleaseWorkflow_DistributionAcceptsEitherTapCredential(t *testing.T) {
	step, _ := workflowStepAt(t, releaseWorkflowPath, "distribution", "Homebrew cask serves this tag")
	configured := step.Env["TAP_CONFIGURED"]
	for _, secret := range []string{tapAppIDSecret, tapAppKeySecret, tapPATSecret} {
		if !strings.Contains(configured, secret) {
			t.Errorf("TAP_CONFIGURED ignores %s, so a release pushed with it would skip the check that proves the cask landed: %q", secret, configured)
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
