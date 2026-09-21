package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The npm distribution is seven packages that have to agree with each
// other and with the release pipeline: a parent that pins six platform
// packages by exact version, a table in JavaScript the parent and the
// bin shim both read, a shell script that fetches the binaries, and a
// guard that checks the registry afterwards.
//
// None of it runs on a pull request. A mismatch between any two of these
// first shows up as `npm install agnostic-ai` putting no binary on a
// machine, because npm skips an optional dependency whose `os` or `cpu`
// does not match and says nothing about it.
const (
	npmManifestPath  = "npm/package.json"
	npmPlatformsPath = "npm/lib/platforms.js"
	npmBinariesJS    = "npm/scripts/build-platform-packages.js"
	npmBinariesShell = "scripts/npm-binaries.sh"
	npmPublishScript = "scripts/npm-publish.sh"
)

// npmPlatform is one row of the PLATFORMS table in npm/lib/platforms.js.
type npmPlatform struct {
	OS     string
	CPU    string
	GOOS   string
	GOARCH string
}

func (p npmPlatform) pkg() string { return "@agnostic-ai/" + p.OS + "-" + p.CPU }

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	path := filepath.Join("..", "..", filepath.FromSlash(rel))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

var platformRow = regexp.MustCompile(
	`\{\s*os:\s*'([^']+)',\s*cpu:\s*'([^']+)',\s*goos:\s*'([^']+)',\s*goarch:\s*'([^']+)'\s*\}`,
)

// npmPlatforms reads the table out of the JavaScript rather than
// restating it, so this file cannot be the second copy that drifts.
func npmPlatforms(t *testing.T) []npmPlatform {
	t.Helper()
	matches := platformRow.FindAllStringSubmatch(readRepoFile(t, npmPlatformsPath), -1)
	if len(matches) == 0 {
		t.Fatalf("no PLATFORMS rows found in %s; the table was reshaped and this guard went blind", npmPlatformsPath)
	}
	out := make([]npmPlatform, 0, len(matches))
	for _, m := range matches {
		out = append(out, npmPlatform{OS: m[1], CPU: m[2], GOOS: m[3], GOARCH: m[4]})
	}
	return out
}

type npmManifest struct {
	Version              string            `json:"version"`
	Files                []string          `json:"files"`
	Scripts              map[string]string `json:"scripts"`
	OS                   []string          `json:"os"`
	CPU                  []string          `json:"cpu"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
}

func npmParent(t *testing.T) npmManifest {
	t.Helper()
	var m npmManifest
	if err := json.Unmarshal([]byte(readRepoFile(t, npmManifestPath)), &m); err != nil {
		t.Fatalf("parse %s: %v", npmManifestPath, err)
	}
	return m
}

// TestNpmPlatformTable_CoversEverySupportedTarget pins the shape of the
// table everything else is derived from.
func TestNpmPlatformTable_CoversEverySupportedTarget(t *testing.T) {
	platforms := npmPlatforms(t)
	if len(platforms) != 6 {
		t.Fatalf("expected 6 platform packages, got %d: %v", len(platforms), platforms)
	}
	// npm and Go spell the same machine differently, and crossing the two
	// vocabularies is the defect this table exists to prevent: it ships a
	// linux binary under a darwin `os`, and npm installs it happily.
	wantOS := map[string]string{"darwin": "darwin", "linux": "linux", "win32": "windows"}
	wantCPU := map[string]string{"x64": "amd64", "arm64": "arm64"}
	for _, p := range platforms {
		if wantOS[p.OS] != p.GOOS {
			t.Errorf("%s maps os %q to goos %q", p.pkg(), p.OS, p.GOOS)
		}
		if wantCPU[p.CPU] != p.GOARCH {
			t.Errorf("%s maps cpu %q to goarch %q", p.pkg(), p.CPU, p.GOARCH)
		}
	}
}

// TestNpmParent_PinsEveryPlatformPackageExactly keeps the seven packages
// in lockstep.
//
// A caret or a missing name means npm resolves a platform package built
// from another commit, or none at all. Both install cleanly.
func TestNpmParent_PinsEveryPlatformPackageExactly(t *testing.T) {
	parent := npmParent(t)
	var want []string
	for _, p := range npmPlatforms(t) {
		want = append(want, p.pkg())
	}
	got := make([]string, 0, len(parent.OptionalDependencies))
	for name, version := range parent.OptionalDependencies {
		got = append(got, name)
		if version != parent.Version {
			t.Errorf("%s is pinned at %q but the parent is %q", name, version, parent.Version)
		}
	}
	sort.Strings(want)
	sort.Strings(got)
	if !slices.Equal(want, got) {
		t.Errorf("optionalDependencies %v does not match the platform table %v", got, want)
	}
}

// TestNpmParent_RunsNoInstallScript is the whole point of the change.
//
// An install script is the failure surface: it needs the network, it is
// blocked by `--ignore-scripts`, and npm 11 prompts before running it.
// Putting one back reintroduces every one of those.
func TestNpmParent_RunsNoInstallScript(t *testing.T) {
	parent := npmParent(t)
	for _, hook := range []string{"preinstall", "install", "postinstall", "prepare", "prepack"} {
		if cmd, ok := parent.Scripts[hook]; ok {
			t.Errorf("npm/package.json declares a %s script (%q); the binary ships in a platform package, so nothing has to run at install time", hook, cmd)
		}
	}
	// `os`/`cpu` on the parent would fail `npm install` for a whole
	// project because one machine runs something we do not build for.
	// biome leaves them off and lets the bin shim explain instead.
	if len(parent.OS) != 0 || len(parent.CPU) != 0 {
		t.Errorf("the parent declares os %v and cpu %v; that blocks the install rather than the command", parent.OS, parent.CPU)
	}
	// Named files, not directories: `lib/` would ship the test suite and
	// `scripts/` the generator to every user.
	want := []string{"bin/agnostic-ai.js", "lib/platforms.js", "README.md"}
	if !slices.Equal(parent.Files, want) {
		t.Errorf("the published tarball carries %v, want %v", parent.Files, want)
	}
}

// TestNpmBinariesScript_CoversEveryPlatformPackage holds the fetch
// script level with the table.
//
// The script unpacks one binary per target and the generator refuses to
// run without all six, so a target dropped here fails the release. A
// target added here and not to the table builds a binary nobody ships.
func TestNpmBinariesScript_CoversEveryPlatformPackage(t *testing.T) {
	script := readRepoFile(t, npmBinariesShell)
	for _, p := range npmPlatforms(t) {
		target := fmt.Sprintf("%s_%s", p.GOOS, p.GOARCH)
		if !regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(target) + `\s*$`).MatchString(script) {
			t.Errorf("%s has no %s in TARGETS, so %s would ship without a binary", npmBinariesShell, target, p.pkg())
		}
	}
}

// TestReleaseWorkflow_BuildsPlatformPackagesBeforeItPublishes pins the
// order the npm job runs in.
//
// Collect the binaries, generate the packages, publish. Reorder any two
// and the release publishes the previous version's packages, or nothing.
func TestReleaseWorkflow_BuildsPlatformPackagesBeforeItPublishes(t *testing.T) {
	job, ok := workflowJobs(t, releaseWorkflowPath)["npm"]
	if !ok {
		t.Fatal("release.yml has no npm job")
	}
	var order []string
	for _, step := range job.Steps {
		for _, want := range []string{npmBinariesShell, npmBinariesJS, npmPublishScript} {
			if strings.Contains(step.Run, want) {
				order = append(order, want)
			}
		}
	}
	if !slices.Equal(order, []string{npmBinariesShell, npmBinariesJS, npmPublishScript}) {
		t.Errorf("the npm job runs %v; it has to collect the binaries, build the packages, then publish", order)
	}
}

// TestReleaseWorkflow_PublishesPlatformPackagesBeforeTheParent guards
// the ordering that makes a partial publish survivable.
//
// The parent pins exact versions of all six. Publish it first and every
// install between the two publishes resolves a platform package that is
// not on the registry yet.
func TestReleaseWorkflow_PublishesPlatformPackagesBeforeTheParent(t *testing.T) {
	script := readRepoFile(t, npmPublishScript)
	platforms := strings.Index(script, `for dir in "${dirs[@]}"`)
	parent := strings.Index(script, `publish_package "$parent"`)
	if platforms < 0 || parent < 0 {
		t.Fatalf("%s no longer publishes the platform packages and the parent as two steps", npmPublishScript)
	}
	if platforms > parent {
		t.Errorf("%s publishes the parent before the platform packages", npmPublishScript)
	}
	if !strings.Contains(script, "wait_for") {
		t.Errorf("%s does not wait for the platform packages to be served before publishing the parent", npmPublishScript)
	}
}

// TestReleaseWorkflow_DistributionChecksEveryNpmPackage widens the guard
// that used to check one package.
//
// Six of the seven can be missing while `npm view agnostic-ai@x` answers
// correctly. That is the new way a release looks green and installs
// nothing on some platform.
func TestReleaseWorkflow_DistributionChecksEveryNpmPackage(t *testing.T) {
	step := workflowRun(t, releaseWorkflowPath, "distribution", "npm serves this tag")

	// Two ways to cover all six, and the derived one is stronger: a literal
	// list in the workflow drifts from the table the packages are generated
	// from, and then checks six names nobody publishes while the six that
	// exist go unverified. Accept either, insist on one.
	derived := strings.Contains(step, "lib/platforms.js") && strings.Contains(step, "packageName")
	if !derived {
		for _, p := range npmPlatforms(t) {
			if !strings.Contains(step, p.pkg()) {
				t.Errorf("the distribution guard neither reads lib/platforms.js nor names %s:\n%s", p.pkg(), step)
			}
		}
	}
	if !strings.Contains(step, "agnostic-ai ") && !strings.Contains(step, "agnostic-ai\\") {
		t.Errorf("the distribution guard no longer checks the parent package:\n%s", step)
	}
}

// TestNpmPublish_TagsEveryPublishAttempt closes the hole an untagged
// publish leaves.
//
// npm defaults a publish with no `--tag` to `latest`, and `latest` is what
// an unpinned `npm install agnostic-ai` resolves. The release workflow fires
// on every `v*` tag and scripts/release.sh accepts a prerelease, so one
// `v0.64.0-beta.1` would replace the stable release on all seven packages.
// The plain retry is a second publish and needs the flag just as much.
func TestNpmPublish_TagsEveryPublishAttempt(t *testing.T) {
	script := readRepoFile(t, npmPublishScript)
	for i, line := range strings.Split(script, "\n") {
		if !strings.Contains(line, "npm publish --access") {
			continue
		}
		if !strings.Contains(line, "--tag") {
			t.Errorf("%s:%d publishes without --tag, which writes the latest dist-tag: %s",
				npmPublishScript, i+1, strings.TrimSpace(line))
		}
	}
	if !strings.Contains(script, "npm_dist_tag()") {
		t.Errorf("%s no longer derives the dist-tag, so nothing tests it in isolation", npmPublishScript)
	}
}

// TestReleaseWorkflow_DistributionChecksTheDistTag guards the other half of
// the npm check.
//
// Asking whether the version exists is not enough: a prerelease published
// without `--tag` is on the registry at its own version and has also taken
// over `latest`, so a version-only guard reports that release as good. The
// guard derives the tag from the publish script rather than restating it,
// because a second copy is how the guard ends up asserting a tag nobody
// published.
func TestReleaseWorkflow_DistributionChecksTheDistTag(t *testing.T) {
	step := workflowRun(t, releaseWorkflowPath, "distribution", "npm serves this tag")
	if !strings.Contains(step, ". "+npmPublishScript) {
		t.Errorf("the guard does not source %s, so its dist-tag can drift from the published one:\n%s",
			npmPublishScript, step)
	}
	if !strings.Contains(step, "npm_dist_tag") {
		t.Errorf("the guard never derives a dist-tag:\n%s", step)
	}
	if !strings.Contains(step, `npm view "${pkg}@${dist_tag}" version`) {
		t.Errorf("the guard never asks the registry what the dist-tag resolves to:\n%s", step)
	}
	// One loop over all seven names does both reads, so the tag is checked
	// for every package rather than for the parent alone.
	if !strings.Contains(step, `[ "$got" = "$want" ] && [ "$tagged" = "$want" ]`) {
		t.Errorf("the guard accepts a package whose dist-tag points elsewhere:\n%s", step)
	}
}
