package integration

import (
	"os"
	"path/filepath"
	"regexp"
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
