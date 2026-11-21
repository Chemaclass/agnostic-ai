package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	RunsOn         string            `yaml:"runs-on"`
	TimeoutMinutes int               `yaml:"timeout-minutes"`
	Needs          any               `yaml:"needs"`
	If             string            `yaml:"if"`
	Permissions    map[string]string `yaml:"permissions"`
	Env            map[string]string `yaml:"env"`
	Strategy       struct {
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
	channelJob      = "channel-manifests"
	channelStep     = "Publish the channel manifest"
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

// caskPostInstallSteps returns the cask's hooks.post.install_steps body.
func caskPostInstallSteps(t *testing.T) string {
	t.Helper()
	hooks, _ := caskConfig(t)["hooks"].(map[string]any)
	post, _ := hooks["post"].(map[string]any)
	steps, _ := post["install_steps"].(string)
	return steps
}

// TestGoreleaserCask_UsesTheNativePostflightSteps keeps the cask off
// Homebrew's deprecated `postflight` block.
//
// goreleaser renders `hooks.post.install` as raw Ruby in `postflight do`,
// which Homebrew deprecated in favor of the declarative
// `postflight_steps` DSL. Every `brew` command touching the tap then
// prints a warning naming our tap and asking the user to report it to us
// (#933). `hooks.post.install_steps` renders `postflight_steps` instead,
// so a hand-written `custom_block` is no longer needed.
//
// Nothing else catches a regression here. The cask is only generated at
// tag time, so switching back would look fine in CI and surface as a
// warning on every user's machine after the release.
func TestGoreleaserCask_UsesTheNativePostflightSteps(t *testing.T) {
	cask := caskConfig(t)
	if _, ok := cask["custom_block"]; ok {
		t.Error("homebrew_casks has a `custom_block`; strip quarantine through `hooks.post.install_steps` instead")
	}
	hooks, _ := cask["hooks"].(map[string]any)
	for _, phase := range []string{"pre", "post"} {
		hook, _ := hooks[phase].(map[string]any)
		for _, key := range []string{"install", "uninstall"} {
			if _, ok := hook[key]; ok {
				t.Errorf("homebrew_casks has `hooks.%s.%s`; goreleaser renders it as Homebrew's deprecated raw flight block, so use `%s_steps`", phase, key, key)
			}
		}
	}
	if strings.TrimSpace(caskPostInstallSteps(t)) == "" {
		t.Error("homebrew_casks has no `hooks.post.install_steps`, so goreleaser renders no `postflight_steps` block")
	}
}

// TestGoreleaserCask_StripsQuarantineOnMacOS pins the reason the
// postflight steps exist at all.
//
// Homebrew propagates com.apple.quarantine from the downloaded archive
// to the extracted files, and the `binary` artifact only chmods +x, so
// it never strips it. Without this call macOS Gatekeeper blocks the
// unsigned binary on first run. Deleting the steps is the obvious way to
// silence a cask warning and it breaks every macOS install.
func TestGoreleaserCask_StripsQuarantineOnMacOS(t *testing.T) {
	steps := caskPostInstallSteps(t)
	if problem := quarantineStripProblem(steps); problem != "" {
		t.Errorf("hooks.post.install_steps: %s:\n%s", problem, steps)
	}
}

// TestGoreleaserCask_QuarantineStripCheckRejectsBrokenSteps proves the
// check reads the steps as code, so a strip that never runs fails it.
func TestGoreleaserCask_QuarantineStripCheckRejectsBrokenSteps(t *testing.T) {
	cases := map[string]string{
		"commented out":    "on_macos do\n  # " + quarantineStrip + "\nend\n",
		"outside on_macos": "on_macos do\nend\n" + quarantineStrip + "\n",
		"another path":     "on_macos do\n  " + strings.Replace(quarantineStrip, "/agnostic-ai", "/other", 1) + "\nend\n",
		"missing":          "",
	}
	for name, steps := range cases {
		if quarantineStripProblem(steps) == "" {
			t.Errorf("%s: the check accepted steps that strip nothing:\n%s", name, steps)
		}
	}
	if problem := quarantineStripProblem("\non_macos do\n  # strip quarantine\n  " + quarantineStrip + "\nend\n"); problem != "" {
		t.Errorf("the check rejected valid steps with a comment and blank lines: %s", problem)
	}
}

const quarantineStrip = `run "/usr/bin/xattr", args: ["-dr", "com.apple.quarantine", "{{ .StagedPath }}/agnostic-ai"]`

// quarantineStripProblem says why the install steps do not strip
// quarantine from the staged binary on macOS, or returns "" when they do.
// Comments and blank lines are dropped first, so text that never runs
// does not count.
func quarantineStripProblem(steps string) string {
	var code []string
	for _, line := range strings.Split(steps, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		code = append(code, line)
	}
	want := []string{"on_macos do", quarantineStrip, "end"}
	if !slices.Equal(code, want) {
		return fmt.Sprintf("want exactly %q, got %q", want, code)
	}
	return ""
}

// TestGoreleaserCask_LetsGoreleaserEmitTheStagedPathToken guards a
// failure that can only happen at tag time.
//
// goreleaser renders the cask and then runs the whole file through its
// own Go template engine a second time. Homebrew's `{{staged_path}}` is
// not a goreleaser function, so a bare token aborts the release with
// `function "staged_path" not defined`. `{{ .StagedPath }}` is the field
// goreleaser exposes in that pass to emit the token, so the steps need
// neither the bare token nor the old hand-escaped form.
func TestGoreleaserCask_LetsGoreleaserEmitTheStagedPathToken(t *testing.T) {
	steps := caskPostInstallSteps(t)
	if strings.Contains(steps, "staged_path") {
		t.Errorf("hooks.post.install_steps names Homebrew's staged_path token directly; use {{ .StagedPath }}:\n%s", steps)
	}
	if !strings.Contains(steps, "{{ .StagedPath }}") {
		t.Errorf("hooks.post.install_steps does not reference {{ .StagedPath }}:\n%s", steps)
	}
}

// TestGoreleaserRuns_PinAVersionThatKnowsInstallSteps keeps every
// goreleaser run on a version that can parse the cask config.
//
// `hooks.post.install_steps` arrived in goreleaser 2.19.0. Older
// versions reject the whole config with `field install_steps not found`,
// so pinning one fails the release at tag time. A floating pin such as
// `latest` or `~> v2` can move without a diff here, so it must be exact.
func TestGoreleaserRuns_PinAVersionThatKnowsInstallSteps(t *testing.T) {
	problems, pins := goreleaserPinProblemsUnder(t, filepath.Join("..", ".."))
	for _, problem := range problems {
		t.Error(problem)
	}
	if pins == 0 {
		t.Errorf("no workflow or composite action step uses %s; update this test if goreleaser now runs another way", goreleaserAction)
	}
}

// TestGoreleaserRuns_PinCheckScansEveryWorkflowAndAction proves an old
// or floating pin is caught in each place GitHub Actions reads steps
// from, not only in `.yml` workflows.
func TestGoreleaserRuns_PinCheckScansEveryWorkflowAndAction(t *testing.T) {
	workflow := func(version string) string {
		return "jobs:\n  release:\n    steps:\n      - uses: " + goreleaserAction + "abc\n        with:\n          version: " + version + "\n"
	}
	action := func(version string) string {
		return "runs:\n  using: composite\n  steps:\n    - uses: " + goreleaserAction + "abc\n      with:\n        version: " + version + "\n"
	}
	root := t.TempDir()
	files := map[string]string{
		".github/workflows/release.yml":        workflow("v2.20.1"),
		".github/workflows/old.yaml":           workflow("v2.18.0"),
		".github/workflows/numeric.yml":        workflow("v2.9.0"),
		".github/actions/floating/action.yml":  action("latest"),
		".github/actions/range/action.yaml":    action("'~> v2'"),
		".github/actions/unpinned/action.yaml": "runs:\n  using: composite\n  steps:\n    - uses: " + goreleaserAction + "abc\n",
	}
	for path, content := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	problems, pins := goreleaserPinProblemsUnder(t, root)
	if pins != len(files) {
		t.Errorf("found %d goreleaser steps, want %d", pins, len(files))
	}
	for _, bad := range []string{"old.yaml", "numeric.yml", "floating", "range", "unpinned"} {
		if !slices.ContainsFunc(problems, func(p string) bool { return strings.Contains(p, bad) }) {
			t.Errorf("no problem reported for %s, got %q", bad, problems)
		}
	}
	if slices.ContainsFunc(problems, func(p string) bool { return strings.Contains(p, "release.yml") }) {
		t.Errorf("v2.20.1 was rejected: %q", problems)
	}
}

const goreleaserAction = "goreleaser/goreleaser-action@"

// goreleaserPinProblemsUnder checks every goreleaser step in the
// workflows and composite actions under root, and returns the problems
// and the number of steps it found.
func goreleaserPinProblemsUnder(t *testing.T, root string) ([]string, int) {
	t.Helper()
	var files []string
	for _, pattern := range []string{
		".github/workflows/*.yml",
		".github/workflows/*.yaml",
		".github/actions/*/action.yml",
		".github/actions/*/action.yaml",
	} {
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, matches...)
	}
	var problems []string
	pins := 0
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		found, n, err := goreleaserPinProblems(path, data)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		problems = append(problems, found...)
		pins += n
	}
	return problems, pins
}

// goreleaserPinProblems checks the goreleaser steps of one workflow or
// composite action. Only steps are decoded: the shared workflowJob shape
// does not fit every workflow's matrix.
func goreleaserPinProblems(path string, data []byte) ([]string, int, error) {
	var doc struct {
		Jobs map[string]struct {
			Steps []workflowStep `yaml:"steps"`
		} `yaml:"jobs"`
		Runs struct {
			Steps []workflowStep `yaml:"steps"`
		} `yaml:"runs"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, 0, err
	}
	steps := map[string][]workflowStep{"composite action": doc.Runs.Steps}
	for name, job := range doc.Jobs {
		steps["job "+strconv.Quote(name)] = job.Steps
	}
	minimum := []int{2, 19, 0}
	exact := regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)
	var problems []string
	pins := 0
	for where, list := range steps {
		for _, step := range list {
			if !strings.HasPrefix(step.Uses, goreleaserAction) {
				continue
			}
			pins++
			version, _ := step.With["version"].(string)
			m := exact.FindStringSubmatch(version)
			if m == nil {
				problems = append(problems, fmt.Sprintf("%s %s runs goreleaser at %q; pin an exact version of at least v2.19.0, which the cask's install_steps needs", path, where, version))
				continue
			}
			got := make([]int, 3)
			for i := range got {
				got[i], _ = strconv.Atoi(m[i+1])
			}
			if slices.Compare(got, minimum) < 0 {
				problems = append(problems, fmt.Sprintf("%s %s runs goreleaser %s; the cask's hooks.post.install_steps needs v2.19.0 or later, or the release fails at tag time with `field install_steps not found`", path, where, version))
			}
		}
	}
	return problems, pins, nil
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

func TestReleaseWorkflow_PinsAnNpmCliThatSupportsProvenance(t *testing.T) {
	var pin struct {
		Version   string
		URL       string
		Integrity string
	}
	data := readRepoFile(t, "scripts/npm-toolchain-pin.json")
	if err := json.Unmarshal([]byte(data), &pin); err != nil {
		t.Fatalf("parse toolchain pin: %v", err)
	}
	if !strings.Contains(workflowRun(t, releaseWorkflowPath, "npm", "Verify and install the npm publishing toolchain"), "scripts/npm-toolchain.sh") {
		t.Error("npm does not use the verified toolchain")
	}
	if !strings.HasPrefix(pin.Integrity, "sha512-") || pin.URL != "https://registry.npmjs.org/npm/-/npm-"+pin.Version+".tgz" {
		t.Errorf("invalid pinned toolchain: %+v", pin)
	}
	const floorMajor, floorMinor = 9, 5
	var major, minor, patch int
	if _, err := fmt.Sscanf(pin.Version, "%d.%d.%d", &major, &minor, &patch); err != nil {
		t.Fatalf("npm toolchain version is not exact: %v", err)
	}
	if major < floorMajor || (major == floorMajor && minor < floorMinor) {
		t.Errorf("npm %s is below the provenance floor", pin.Version)
	}
	job := workflowJobs(t, releaseWorkflowPath)["npm"]
	if job.Env["NODE_AUTH_TOKEN"] != "" {
		t.Error("publication token is exposed outside the publish step")
	}
}

func TestReleaseWorkflow_HomebrewCheckRetriesAStaleReplica(t *testing.T) {
	script := workflowRun(t, releaseWorkflowPath, "distribution", "Homebrew cask serves this tag")
	for _, want := range []string{"attempts=6", "delay * 2", "sleep", "::error::", "homebrew: pending", "::notice::", "exit 0"} {
		if !strings.Contains(script, want) {
			t.Errorf("the Homebrew check lacks %q:\n%s", want, script)
		}
	}
}

func TestReleaseWorkflow_HomebrewCheckChecksOutRepository(t *testing.T) {
	checkout, checkoutAt := workflowStepAt(t, releaseWorkflowPath, "distribution", "Checkout")
	if !regexp.MustCompile(`^actions/checkout@[0-9a-f]{40}$`).MatchString(checkout.Uses) {
		t.Errorf("the distribution checkout uses %q, want actions/checkout pinned to a commit SHA", checkout.Uses)
	}
	_, checkAt := workflowStepAt(t, releaseWorkflowPath, "distribution", "Homebrew cask serves this tag")
	if checkoutAt > checkAt {
		t.Error("checkout runs after the Homebrew check")
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
	job := workflowJobs(t, releaseWorkflowPath)[channelJob]
	flag := job.Env["HOMEBREW_TAP_APP_CONFIGURED"]
	if !strings.Contains(flag, tapAppIDSecret) || !strings.Contains(flag, tapAppKeySecret) {
		t.Fatalf("the channel job does not record whether both App secrets are set, got %q", flag)
	}
	mint, mintAt := workflowStepAt(t, releaseWorkflowPath, channelJob, tapMintStep)
	if !strings.Contains(mint.If, "HOMEBREW_TAP_APP_CONFIGURED") {
		t.Errorf("the mint step is not gated on the App being configured, so a release without the App fails on an empty app-id, got %q", mint.If)
	}
	if strings.Contains(mint.If, "secrets.") {
		t.Errorf("the mint step reads `secrets` from a step `if:`, which GitHub rejects as an unrecognized named-value, got %q", mint.If)
	}
	if mint.ID == "" {
		t.Fatal("the mint step has no id, so nothing can read its token output")
	}
	_, releaseAt := workflowStepAt(t, releaseWorkflowPath, channelJob, channelStep)
	if mintAt > releaseAt {
		t.Error("the mint step runs after the cask push")
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
	mint, _ := workflowStepAt(t, releaseWorkflowPath, channelJob, tapMintStep)
	release, _ := workflowStepAt(t, releaseWorkflowPath, channelJob, channelStep)
	token := release.Env["GH_TOKEN"]
	if want := "steps." + mint.ID + ".outputs.token"; !strings.Contains(token, want) {
		t.Errorf("manifest publisher does not read the minted token (%s): %q", want, token)
	}
	if !strings.Contains(token, "secrets."+tapPATSecret) {
		t.Errorf("manifest publisher has no fallback to the stored %s: %q", tapPATSecret, token)
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
// The pin is a commit SHA to match every other action in this repo.
func TestReleaseWorkflow_ScopesTheTapTokenToTheTap(t *testing.T) {
	mint, _ := workflowStepAt(t, releaseWorkflowPath, channelJob, tapMintStep)
	if !regexp.MustCompile(`^actions/create-github-app-token@[0-9a-f]{40}$`).MatchString(mint.Uses) {
		t.Errorf("the mint step must use actions/create-github-app-token pinned to a commit SHA like the rest of this repo, got %q", mint.Uses)
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
	release, releaseIndex := workflowStepAt(t, installWorkflowPath, "published", "Wait for the registry to serve the latest release")
	install, installIndex := workflowStepAt(t, installWorkflowPath, "published", "Install the published package")
	binary, binaryIndex := workflowStepAt(t, installWorkflowPath, "published", "Binary reports the published version")
	if releaseIndex >= installIndex || installIndex >= binaryIndex {
		t.Error("release validation must precede registry installation and installed-version verification")
	}
	if release.ID != "release" {
		t.Errorf("release step must supply the version output, got id %q", release.ID)
	}
	for _, required := range []string{`want="${want#v}"`, `if [[ ! "$want" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ ]]; then`, "release tag is not an exact npm version", "exit 1", `npm view "agnostic-ai@${want}" version`, `[ "$got" = "$want" ]`, `echo "version=$want" >> "$GITHUB_OUTPUT"`} {
		if !strings.Contains(release.Run, required) {
			t.Errorf("registry wait does not validate and export the exact release version: missing %q", required)
		}
	}
	validation := strings.Index(release.Run, "release tag is not an exact npm version")
	registry := strings.Index(release.Run, `npm view "agnostic-ai@${want}" version`)
	output := strings.Index(release.Run, `echo "version=$want" >> "$GITHUB_OUTPUT"`)
	if validation < 0 || registry < validation || output < registry {
		t.Error("exact-version validation and registry confirmation must precede exporting the install version")
	}
	const versionOutput = "${{ steps.release.outputs.version }}"
	if install.Env["NPM_RELEASE_VERSION"] != versionOutput || binary.Env["NPM_RELEASE_VERSION"] != versionOutput {
		t.Error("installation and binary check must consume the confirmed release step output")
	}
	if strings.TrimSpace(install.Run) != `npm install -g "agnostic-ai@$NPM_RELEASE_VERSION"` {
		t.Errorf("registry install must quote the confirmed exact version, got %q", install.Run)
	}
	if strings.Contains(all, "agnostic-ai@latest") || strings.Contains(all, "agnostic-ai@next") {
		t.Error("published install must not select a moving tag")
	}
	for _, required := range []string{"agnostic-ai --version", `[ "$got" = "agnostic-ai version $NPM_RELEASE_VERSION" ]`} {
		if !strings.Contains(binary.Run, required) {
			t.Errorf("installed binary version must equal the selected release, missing %q", required)
		}
	}
	// Triggered by a release, this job races the same replica lag the
	// distribution guard hit, so it has to wait the same way.
	if !strings.Contains(release.Run, "for attempt in") || !strings.Contains(release.Run, "sleep") {
		t.Errorf("the published job does not wait for the registry, so a run right after a release fails on propagation delay:\n%s", all)
	}
}

// companionArchive is the macOS companion archive `agnostic-ai companion
// install` downloads for Apple silicon; the Intel one differs in the
// architecture only.
const companionArchive = "agnostic-ai-companion_darwin_arm64.tar.gz"

// TestReleaseWorkflow_BuildsTheCompanionOnAMacWithoutPublishRights guards
// the job that builds the companion archives: it runs the dependencies of
// a C build, so it gets no token beyond read access and no secret.
func TestReleaseWorkflow_BuildsTheCompanionOnAMacWithoutPublishRights(t *testing.T) {
	jobs := workflowJobs(t, releaseWorkflowPath)
	job, ok := jobs["companion"]
	if !ok {
		t.Fatal("release.yml has no companion job")
	}
	if !strings.HasPrefix(job.RunsOn, "macos-") {
		t.Errorf("the companion job runs on %q; the companion needs a Mac to build", job.RunsOn)
	}
	if len(job.Permissions) != 1 || job.Permissions["contents"] != "read" {
		t.Errorf("the companion job holds %v, want only contents: read", job.Permissions)
	}
	data := readRepoFile(t, ".github/workflows/release.yml")
	start := strings.Index(data, "\n  companion:\n")
	end := strings.Index(data, "\n  goreleaser:\n")
	if start < 0 || end < start {
		t.Fatal("release.yml lists the companion job after goreleaser")
	}
	if strings.Contains(data[start:end], "secrets.") {
		t.Error("the companion job reads a secret")
	}
	if build := workflowRun(t, releaseWorkflowPath, "companion", "Build the companion archives"); !strings.Contains(build, "scripts/companion-release.sh") {
		t.Errorf("the companion job does not build with scripts/companion-release.sh:\n%s", build)
	}

	goreleaser := jobs["goreleaser"]
	if !needsJob(goreleaser.Needs, "companion") {
		t.Errorf("goreleaser needs %v, want it to wait for companion", goreleaser.Needs)
	}
	if goreleaser.If != "" {
		t.Errorf("candidate build uses %q instead of the default success requirement", goreleaser.If)
	}
	if job.TimeoutMinutes == 0 {
		t.Error("the companion job has no timeout; goreleaser waits for it")
	}
	download, _ := workflowStepAt(t, releaseWorkflowPath, "goreleaser", "Download the companion archives")
	if download.If != "needs.companion.outputs.artifact != ''" {
		t.Errorf("the archives are downloaded if %q", download.If)
	}
	run, _ := workflowStepAt(t, releaseWorkflowPath, "goreleaser", "Run GoReleaser")
	if run.Env["COMPANION_DIST"] != download.With["path"] {
		t.Errorf("GoReleaser reads the archives from %q, the download writes them to %v", run.Env["COMPANION_DIST"], download.With["path"])
	}
}

func TestReleaseWorkflow_ChecksCandidateBeforeAnyPublisher(t *testing.T) {
	jobs := workflowJobs(t, releaseWorkflowPath)
	for name, deps := range map[string][]string{
		"goreleaser":        {"ui-bundle", "companion"},
		"ui-smoke":          {"goreleaser"},
		"attest":            {"goreleaser", "ui-smoke"},
		"publish":           {"attest"},
		"npm":               {"publish"},
		"channel-manifests": {"publish"},
	} {
		needs := jobNeeds(t, releaseWorkflowPath, name)
		for _, dep := range deps {
			if !slices.Contains(needs, dep) {
				t.Errorf("%s needs %v, want it to wait for %s", name, needs, dep)
			}
		}
		if condition := jobs[name].If; strings.Contains(condition, "always()") || strings.Contains(condition, "failure()") {
			t.Errorf("%s allows its prerequisites to fail: %q", name, condition)
		}
	}
	for _, name := range []string{"goreleaser", "ui-smoke"} {
		for scope, access := range jobs[name].Permissions {
			if access == "write" {
				t.Errorf("%s has %s: write before the candidate checks pass", name, scope)
			}
		}
	}
	build, _ := workflowStepAt(t, releaseWorkflowPath, "goreleaser", "Run GoReleaser")
	if args, _ := build.With["args"].(string); !strings.Contains(args, "--skip=publish") {
		t.Errorf("GoReleaser can publish while it builds: %q", args)
	}
}

func TestReleaseWorkflow_RecoveryUsesTheOriginalTagAndSelectedChannel(t *testing.T) {
	jobs := workflowJobs(t, releaseWorkflowPath)
	for _, name := range []string{"ui-bundle", "companion"} {
		if jobs[name].If != "github.event_name == 'push'" {
			t.Errorf("%s can run on a recovery dispatch: %q", name, jobs[name].If)
		}
	}
	script := workflowRun(t, releaseWorkflowPath, "recovery", "Find the original release for the tag")
	for _, want := range []string{`"$GITHUB_REF_TYPE" != tag`, "scripts/release-assets.py recover", `"$(git rev-parse HEAD)" "$CHANNEL"`} {
		if !strings.Contains(script, want) {
			t.Errorf("recovery does not require %q", want)
		}
	}
	for _, name := range []string{"channel-manifests", "npm"} {
		if !needsJob(jobs[name].Needs, "recovery") || !strings.Contains(jobs[name].If, "needs.recovery.result == 'success'") {
			t.Errorf("%s can recover before verifying the original tag", name)
		}
	}
	download, _ := workflowStepAt(t, releaseWorkflowPath, "channel-manifests", "Download the signed candidate")
	if !strings.Contains(fmt.Sprint(download.With["artifact-ids"]), "needs.recovery.outputs.artifact") ||
		!strings.Contains(fmt.Sprint(download.With["run-id"]), "needs.recovery.outputs.run-id") {
		t.Error("manifest recovery does not use the original signed artifact and its run ID")
	}
	binaries := workflowRun(t, releaseWorkflowPath, "npm", "Collect the release binaries")
	if !strings.Contains(binaries, `"$(git rev-parse HEAD)"`) {
		t.Error("npm does not verify the tag commit of the downloaded archives")
	}
}

// TestGoreleaser_PublishesAndChecksumsTheCompanionArchives keeps the
// companion archives on the release and in checksums.txt, which the CLI
// verifies them against and the provenance step signs.
func TestGoreleaser_PublishesAndChecksumsTheCompanionArchives(t *testing.T) {
	var doc struct {
		Checksum struct {
			ExtraFiles []struct {
				Glob string `yaml:"glob"`
			} `yaml:"extra_files"`
		} `yaml:"checksum"`
		Release struct {
			ExtraFiles []struct {
				Glob string `yaml:"glob"`
			} `yaml:"extra_files"`
		} `yaml:"release"`
	}
	if err := yaml.Unmarshal([]byte(readRepoFile(t, ".goreleaser.yml")), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Checksum.ExtraFiles) != 2 || len(doc.Release.ExtraFiles) != 2 {
		t.Fatalf("want archive and disk image globs in checksum and release, got %+v %+v", doc.Checksum.ExtraFiles, doc.Release.ExtraFiles)
	}
	glob := doc.Checksum.ExtraFiles[0].Glob
	if doc.Release.ExtraFiles[0].Glob != glob {
		t.Errorf("release attaches %q but checksums.txt lists %q", doc.Release.ExtraFiles[0].Glob, glob)
	}
	if matched, err := filepath.Match(filepath.Base(glob), companionArchive); err != nil || !matched {
		t.Errorf("the glob %q does not match %s", glob, companionArchive)
	}
	if !strings.Contains(glob, `envOrDefault "COMPANION_DIST"`) {
		t.Errorf("the glob %q does not read the folder the workflow downloads to", glob)
	}
	imageGlob := doc.Checksum.ExtraFiles[1].Glob
	if doc.Release.ExtraFiles[1].Glob != imageGlob {
		t.Errorf("release and checksums list different disk images: %q, %q", doc.Release.ExtraFiles[1].Glob, imageGlob)
	}
	for _, arch := range []string{"arm64", "amd64"} {
		image := "agnostic-ai-companion_darwin_" + arch + ".dmg"
		if matched, err := filepath.Match(filepath.Base(imageGlob), image); err != nil || !matched {
			t.Errorf("the glob %q does not match %s", imageGlob, image)
		}
	}
	upload, _ := workflowStepAt(t, releaseWorkflowPath, "companion", "Hand the archives to GoReleaser")
	paths, ok := upload.With["path"].(string)
	if !ok || !strings.Contains(paths, "agnostic-ai-companion_darwin_*.dmg") {
		t.Errorf("the companion job does not upload disk images: %v", upload.With["path"])
	}
	guard := workflowRun(t, releaseWorkflowPath, "goreleaser", "Stage and validate the release inventory")
	if !strings.Contains(guard, "scripts/release-assets.py stage") {
		t.Error("the candidate build does not validate the release inventory")
	}
	script := readRepoFile(t, "scripts/companion-release.sh")
	for _, want := range []string{"name=agnostic-ai-companion", `"$outdir/${name}_darwin_${arch}.tar.gz"`, "for arch in arm64 amd64"} {
		if !strings.Contains(script, want) {
			t.Errorf("scripts/companion-release.sh no longer has %q; the archives must keep the names the CLI downloads", want)
		}
	}
	for _, want := range []string{"hdiutil_retrying create", `hdiutil_retrying verify "$image"`} {
		if !strings.Contains(script, want) {
			t.Errorf("scripts/companion-release.sh no longer runs %q; a busy runner fails the disk image without the retry", want)
		}
	}
}

// needsJob reports whether a job's `needs`, one name or a list, names job.
func needsJob(needs any, job string) bool {
	switch needs := needs.(type) {
	case string:
		return needs == job
	case []any:
		return slices.Contains(needs, any(job))
	}
	return false
}
