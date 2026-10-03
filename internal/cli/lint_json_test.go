package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

type lintJSONReport struct {
	Version  string `json:"version"`
	Command  string `json:"command"`
	Findings []struct {
		Code     string `json:"code"`
		Severity string `json:"severity"`
		Path     string `json:"path"`
		Message  string `json:"message"`
	} `json:"findings"`
}

// runLintJSON keeps stdout apart from stderr, since a script pipes only
// stdout into jq.
func runLintJSON(t *testing.T, args ...string) (lintJSONReport, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(append([]string{"lint", "--json"}, args...))
	err := root.Execute()
	var report lintJSONReport
	if jsonErr := json.Unmarshal(stdout.Bytes(), &report); jsonErr != nil {
		t.Fatalf("stdout is not JSON: %v\nstdout:\n%s\nstderr:\n%s", jsonErr, stdout.String(), stderr.String())
	}
	return report, stderr.String(), err
}

func TestLintJSON_ReportsFindingsWithLintExitStatus(t *testing.T) {
	dir := budgetProject(t, "targets: [claude]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai/rules/style.md"), "---\nallowed_tools: [Read]\n---\n\nPrefer short functions.\n")

	report, _, err := runLintJSON(t)
	if err != nil {
		t.Fatalf("warnings alone must exit 0 without --strict: %v", err)
	}
	if report.Version != "1" || report.Command != "lint" {
		t.Errorf("header = %q %q, want 1 lint", report.Version, report.Command)
	}
	if len(report.Findings) != 1 || report.Findings[0].Code != "LINT007" || report.Findings[0].Severity != "warn" {
		t.Fatalf("findings = %+v, want one LINT007 warn", report.Findings)
	}
	if report.Findings[0].Path == "" || report.Findings[0].Message == "" {
		t.Errorf("finding lacks path or message: %+v", report.Findings[0])
	}

	if _, _, err := runLintJSON(t, "--strict"); err == nil {
		t.Error("--strict must exit 1 on a warning, as the text output does")
	}
}

func TestLintJSON_CleanProjectPrintsEmptyFindings(t *testing.T) {
	dir := budgetProject(t, "targets: [claude]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai/rules/style.md"), "Prefer short functions.\n")

	report, _, err := runLintJSON(t, "--strict")
	if err != nil {
		t.Fatalf("clean project must pass: %v", err)
	}
	if report.Findings == nil || len(report.Findings) != 0 {
		t.Errorf("findings = %#v, want an empty list", report.Findings)
	}
}

func TestLintJSON_EmptyProjectKeepsHintOnStderr(t *testing.T) {
	budgetProject(t, "targets: [claude]\n")

	report, stderr, err := runLintJSON(t)
	if err != nil {
		t.Fatalf("empty project must pass: %v", err)
	}
	if len(report.Findings) != 0 || stderr == "" {
		t.Errorf("findings = %+v, stderr = %q; want none and a hint", report.Findings, stderr)
	}
}

func TestLintJSON_GlobalReportsErrorsAndFails(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "defaults.yaml"), "effort:\n  claude: max\n")

	report, _, err := runLintJSON(t, "--global")
	if err == nil {
		t.Fatal("an error finding must exit 1")
	}
	found := false
	for _, f := range report.Findings {
		if f.Code == "LINT014" && f.Severity == "error" {
			found = true
		}
	}
	if !found {
		t.Errorf("findings = %+v, want LINT014 error", report.Findings)
	}
}
