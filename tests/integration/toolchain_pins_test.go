package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Local tool pins must match the workflow that uses each tool.
const (
	makefilePath      = "../../Makefile"
	ciWorkflowPath    = "../../.github/workflows/ci.yml"
	pagesWorkflowPath = "../../.github/workflows/playground.yml"
	goModPath         = "../../go.mod"
	misePath          = "../../mise.toml"
)

var (
	makefilePinRE     = regexp.MustCompile(`(?m)^GOLANGCI_LINT_VERSION := (\S+)$`)
	workflowPinRE     = regexp.MustCompile(`(?s)golangci-lint-action@\S+.*?version:\s*(\S+)`)
	zolaMakefilePinRE = regexp.MustCompile(`(?m)^ZOLA_VERSION := (\S+)$`)
	zolaWorkflowPinRE = regexp.MustCompile(`(?m)^\s+ZOLA_VERSION:\s+(\S+)$`)
)

func pinFrom(t *testing.T, path string, re *regexp.Regexp) string {
	t.Helper()
	data, err := os.ReadFile(filepath.FromSlash(path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	m := re.FindSubmatch(data)
	if m == nil {
		t.Fatalf("no matching version pin found in %s", path)
	}
	return string(m[1])
}

// TestGolangciLintPin_MatchesCI keeps the local linter and the PR gate on
// one version. A Makefile pin older than the contributor's Go toolchain
// cannot decode its export data, and golangci-lint reports that as a
// typecheck failure in files the branch never touched, so the drift reads
// as a real lint break. Failing here names it instead.
func TestGolangciLintPin_MatchesCI(t *testing.T) {
	local := pinFrom(t, makefilePath, makefilePinRE)
	ci := pinFrom(t, ciWorkflowPath, workflowPinRE)
	if local != ci {
		t.Errorf("Makefile pins golangci-lint %s but %s pins %s; bump both together so `make lint` matches the PR check",
			local, ciWorkflowPath, ci)
	}
}

// TestMisePins_MatchRepoPins keeps the optional mise.toml on the versions
// CI uses, so `mise install` sets up the same toolchain as the PR checks.
func TestMisePins_MatchRepoPins(t *testing.T) {
	data, err := os.ReadFile(filepath.FromSlash(ciWorkflowPath))
	if err != nil {
		t.Fatalf("read %s: %v", ciWorkflowPath, err)
	}
	ci := string(data)

	goMod, err := os.ReadFile(filepath.FromSlash(goModPath))
	if err != nil {
		t.Fatalf("read %s: %v", goModPath, err)
	}
	// Without a toolchain line, the go line names the toolchain, as in scripts/doctor-dev.sh.
	goPin := regexp.MustCompile(`(?m)^toolchain go(\S+)$`).FindSubmatch(goMod)
	if goPin == nil {
		goPin = regexp.MustCompile(`(?m)^go (\S+)$`).FindSubmatch(goMod)
	}
	if goPin == nil {
		t.Fatalf("no toolchain or go line found in %s", goModPath)
	}

	pins := []struct {
		tool, source, want string
	}{
		{"go", goModPath, string(goPin[1])},
		{"zola", makefilePath, pinFrom(t, makefilePath, zolaMakefilePinRE)},
	}
	nodes := regexp.MustCompile(`(?m)^\s+node-version:\s*'?([^'\s]+)'?$`).FindAllStringSubmatch(ci, -1)
	if len(nodes) == 0 {
		t.Fatalf("no node-version found in %s", ciWorkflowPath)
	}
	for _, m := range nodes {
		pins = append(pins, struct{ tool, source, want string }{"node", ciWorkflowPath, m[1]})
	}
	java := regexp.MustCompile(`(?s)setup-java@\S+.*?distribution:\s*'?([^'\s]+)'?\s+java-version:\s*'?([^'\s]+)'?`).FindStringSubmatch(ci)
	if java == nil {
		t.Fatalf("no setup-java distribution and java-version found in %s", ciWorkflowPath)
	}
	pins = append(pins, struct{ tool, source, want string }{"java", ciWorkflowPath, java[1] + "-" + java[2]})

	for _, p := range pins {
		got := pinFrom(t, misePath, regexp.MustCompile(`(?m)^`+p.tool+` = "([^"]+)"$`))
		if got != p.want {
			t.Errorf("mise.toml pins %s %s but %s pins %s; bump them together", p.tool, got, p.source, p.want)
		}
	}
}

func TestZolaPin_MatchesWorkflows(t *testing.T) {
	local := pinFrom(t, makefilePath, zolaMakefilePinRE)
	for _, path := range []string{pagesWorkflowPath, ciWorkflowPath} {
		workflow := pinFrom(t, path, zolaWorkflowPinRE)
		if local != workflow {
			t.Errorf("Makefile pins Zola %s but %s pins %s; bump them together so local, CI and Pages builds match",
				local, path, workflow)
		}
	}
}

// lookupPinnedZola returns the zola binary only when it matches the
// Makefile pin. A missing binary or a different version is a skip, not a
// fail, so a contributor with another zola on PATH still runs the rest of
// the suite. The CI shard that installs the pin sets
// AGNOSTIC_AI_REQUIRE_ZOLA=1 to turn the skip into a failure. Matching the
// Makefile gate here stops a newer zola from reporting a template parse
// error in a file that is valid under the pin.
func lookupPinnedZola(t *testing.T) string {
	t.Helper()
	skip := t.Skipf
	if os.Getenv("AGNOSTIC_AI_REQUIRE_ZOLA") == "1" {
		skip = t.Fatalf
	}
	zolaPath, err := exec.LookPath("zola")
	if err != nil {
		skip("zola is not installed")
	}
	pin := pinFrom(t, makefilePath, zolaMakefilePinRE)
	out, err := exec.Command(zolaPath, "--version").CombinedOutput()
	have := strings.TrimSpace(string(out))
	if err != nil {
		skip("zola --version failed: %v\n%s", err, have)
	}
	want := "zola " + pin
	if have != want {
		skip("this test needs zola %s (the Makefile pin), found %s", pin, zolaVersionFromLine(have))
	}
	return zolaPath
}

func zolaVersionFromLine(have string) string {
	have = strings.TrimSpace(have)
	if have == "" {
		return "nothing"
	}
	return strings.TrimPrefix(have, "zola ")
}

func TestZolaVersionFromLine(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"zola 0.22.0", "0.22.0"},
		{"zola 0.23.6\n", "0.23.6"},
		{"", "nothing"},
		{"  ", "nothing"},
	}
	for _, tc := range tests {
		if got := zolaVersionFromLine(tc.in); got != tc.want {
			t.Errorf("zolaVersionFromLine(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// usesLineRE matches a workflow step's action reference and the comment
// after it.
var usesLineRE = regexp.MustCompile(`(?m)^\s*(?:-\s+)?uses:\s+(\S+)(.*)$`)

// TestWorkflows_PinActionsToCommitSHAs keeps every third-party action on
// an immutable commit. A tag can be moved to other code after review, and
// a release job runs with write access to the repository and the npm
// token, so a moved tag would run unreviewed code with both. The trailing
// `# vX` comment lets Renovate and a reader see which release it is.
func TestWorkflows_PinActionsToCommitSHAs(t *testing.T) {
	paths, err := filepath.Glob(filepath.FromSlash("../../.github/workflows/*.yml"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no workflows found: %v", err)
	}
	pinned := regexp.MustCompile(`^[^@\s]+@[0-9a-f]{40}$`)
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range usesLineRE.FindAllStringSubmatch(string(data), -1) {
			ref, comment := m[1], strings.TrimSpace(m[2])
			if strings.HasPrefix(ref, "./") {
				continue
			}
			if !pinned.MatchString(ref) || !strings.HasPrefix(comment, "# v") {
				t.Errorf("%s: %s is not pinned to a commit SHA with a `# vX` comment", filepath.Base(p), ref)
			}
		}
	}
}
