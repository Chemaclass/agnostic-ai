package cli

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func runHookGuard(t *testing.T, payload string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(payload))
	root.SetArgs(append([]string{"hook", "guard"}, args...))
	err := root.Execute()
	return out.String(), err
}

// guardProject syncs a claude project with one clean skill.
func guardProject(t *testing.T) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	t.Setenv("AGNOSTIC_AI_TARGET", "claude")
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeAgnosticFile(t, "# Shared\n")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "ok", "SKILL.md"), "---\nname: ok\ndescription: Fine.\n---\n\nBody.\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	return dir
}

func editPayload(dir, rel string) string {
	return `{"tool_name":"Write","tool_input":{"file_path":` + jsonString(filepath.Join(dir, rel)) + `,"content":"x"}}`
}

func wantGuardReport(t *testing.T, err error, want string) {
	t.Helper()
	var report *guardReport
	if !errors.As(err, &report) || report.ExitCode() != 2 || !strings.Contains(report.Error(), want) {
		t.Fatalf("want an exit 2 report with %q, got %v", want, err)
	}
}

func TestHookGuard_AfterEditReportsLintErrorsInTheEditedSpec(t *testing.T) {
	dir := guardProject(t)
	rel := filepath.Join(".agnostic-ai", "skills", "x", "SKILL.md")
	writeFile(t, rel, "---\nname: x\ndescription: X.\n\nBody\n")

	_, err := runHookGuard(t, editPayload(dir, rel), "after-edit")

	wantGuardReport(t, err, "LINT006 [error] "+rel)
}

func TestHookGuard_AfterEditIsSilentOnACleanSpecAndOnOtherFiles(t *testing.T) {
	dir := guardProject(t)
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "x", "SKILL.md"), "---\nname: x\ndescription: X.\n\nBody\n")
	writeFile(t, "README.md", "# Readme\n")

	for _, rel := range []string{filepath.Join(".agnostic-ai", "skills", "ok", "SKILL.md"), "README.md"} {
		out, err := runHookGuard(t, editPayload(dir, rel), "after-edit")
		if err != nil || out != "" {
			t.Errorf("%s: want no output and exit 0, got %v %q", rel, err, out)
		}
	}
}

func TestHookGuard_StopReportsDriftOnce(t *testing.T) {
	guardProject(t)
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "ok", "SKILL.md"), "---\nname: ok\ndescription: Changed.\n---\n\nBody.\n")

	_, err := runHookGuard(t, `{"hook_event_name":"Stop"}`, "stop")
	wantGuardReport(t, err, "run `agnostic-ai sync`")

	out, err := runHookGuard(t, `{"hook_event_name":"Stop","stop_hook_active":true}`, "stop")
	if err != nil || out != "" {
		t.Errorf("a stop already continued by the hook must pass, got %v %q", err, out)
	}
}

func TestHookGuard_StopIsSilentWithoutDrift(t *testing.T) {
	guardProject(t)

	out, err := runHookGuard(t, `{}`, "stop")
	if err != nil || out != "" {
		t.Errorf("want no output and exit 0, got %v %q", err, out)
	}
}

func TestLint_FilesReportsOnlyTheNamedSpecs(t *testing.T) {
	guardProject(t)
	broken := filepath.Join(".agnostic-ai", "skills", "x", "SKILL.md")
	writeFile(t, broken, "---\nname: x\ndescription: X.\n\nBody\n")

	out, err := runCLI(t, "lint", "--files", filepath.Join(".agnostic-ai", "skills", "ok", "SKILL.md"))
	if err != nil || !strings.Contains(out, "ok — 1 file(s) clean") {
		t.Errorf("a clean file must pass alone, got %v\n%s", err, out)
	}

	var stdout bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&stdout)
	root.SetErr(&stdout)
	root.SetIn(strings.NewReader(broken + "\nREADME.md\n"))
	root.SetArgs([]string{"lint", "--files", "-"})
	if err := root.Execute(); err == nil || !strings.Contains(stdout.String(), "LINT006") {
		t.Errorf("want LINT006 for the piped path, got %v\n%s", err, stdout.String())
	}
}

func TestLint_PathsNeedFiles(t *testing.T) {
	guardProject(t)

	if _, err := runCLI(t, "lint", "README.md"); err == nil {
		t.Error("lint with a path and no --files must fail")
	}
}
