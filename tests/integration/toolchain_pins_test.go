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
)

var (
	makefilePinRE     = regexp.MustCompile(`(?m)^GOLANGCI_LINT_VERSION := (\S+)$`)
	workflowPinRE     = regexp.MustCompile(`(?s)golangci-lint-action@v\d+.*?version:\s*(\S+)`)
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

func TestZolaPin_MatchesPagesWorkflow(t *testing.T) {
	local := pinFrom(t, makefilePath, zolaMakefilePinRE)
	workflow := pinFrom(t, pagesWorkflowPath, zolaWorkflowPinRE)
	if local != workflow {
		t.Errorf("Makefile pins Zola %s but %s pins %s; bump both together so local and Pages builds match",
			local, pagesWorkflowPath, workflow)
	}
}

// lookupPinnedZola returns the zola binary only when it matches the
// Makefile pin. A missing binary or a different version is a skip, not a
// fail: CI installs 0.22.0, and a contributor with another zola on PATH
// should still run the rest of the suite. Matching the Makefile gate here
// stops a newer zola from reporting a template parse error in a file that
// is valid under the pin.
func lookupPinnedZola(t *testing.T) string {
	t.Helper()
	zolaPath, err := exec.LookPath("zola")
	if err != nil {
		t.Skip("zola is not installed")
	}
	pin := pinFrom(t, makefilePath, zolaMakefilePinRE)
	out, err := exec.Command(zolaPath, "--version").CombinedOutput()
	have := strings.TrimSpace(string(out))
	if err != nil {
		t.Skipf("zola --version failed: %v\n%s", err, have)
	}
	want := "zola " + pin
	if have != want {
		t.Skipf("skipping: this test needs zola %s (the Makefile pin), found %s", pin, zolaVersionFromLine(have))
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
