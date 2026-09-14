package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// The Makefile pin is what `make tools` installs and what a contributor
// ends up running locally; the workflow pin is what gates the PR. When
// they drift, the local binary can be too old to decode the export data
// of a newer Go toolchain, and golangci-lint reports that as a typecheck
// failure in files the branch never touched. The pins are compared here
// so the drift fails a test instead of a contributor's afternoon.
const (
	makefilePath = "../../Makefile"
	ciWorkflow   = "../../.github/workflows/ci.yml"
)

var (
	makefilePinRE = regexp.MustCompile(`(?m)^GOLANGCI_LINT_VERSION := (\S+)$`)
	workflowPinRE = regexp.MustCompile(`(?s)golangci-lint-action@v\d+.*?version:\s*(\S+)`)
)

func pinFrom(t *testing.T, path string, re *regexp.Regexp) string {
	t.Helper()
	data, err := os.ReadFile(filepath.FromSlash(path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	m := re.FindSubmatch(data)
	if m == nil {
		t.Fatalf("no golangci-lint version pin found in %s", path)
	}
	return string(m[1])
}

func TestGolangciLintPin_MatchesCI(t *testing.T) {
	local := pinFrom(t, makefilePath, makefilePinRE)
	ci := pinFrom(t, ciWorkflow, workflowPinRE)
	if local != ci {
		t.Errorf("Makefile pins golangci-lint %s but %s pins %s; bump both together so `make lint` matches the PR check",
			local, ciWorkflow, ci)
	}
}
