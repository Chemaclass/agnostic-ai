package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func setupDoctorLintProject(t *testing.T, specs map[string]string) {
	t.Helper()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r1.md"), "---\nname: r1\ndescription: Style.\n---\nrule body\n")
	for path, content := range specs {
		mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", path), content)
	}
	testutil.Chdir(t, dir)
	silence(t)
	syncProject(t)
}

func TestDoctor_ListsLintWarningsWithoutFailing(t *testing.T) {
	setupDoctorLintProject(t, map[string]string{
		"agents/reviewer.md": "---\nname: reviewer\ndescription: Reviews diffs.\nallowed_tools: [Read]\n---\n\nReview.\n",
	})

	out, err := runDoctor(t)

	if err != nil {
		t.Fatalf("a lint warning must not fail doctor: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Spec health:\n") || !strings.Contains(out, "LINT007 [warn] "+filepath.FromSlash(".agnostic-ai/agents/reviewer.md")) {
		t.Errorf("doctor should list the lint warning under Spec health:\n%s", out)
	}
	if strings.Contains(out, "All checks passed") {
		t.Errorf("doctor claimed all checks passed with a lint finding:\n%s", out)
	}
	if !strings.Contains(out, "Next step:\n  Review spec findings: agnostic-ai lint\n") {
		t.Errorf("the next step should point at lint:\n%s", out)
	}
}

func TestDoctor_FailsOnLintError(t *testing.T) {
	setupDoctorLintProject(t, map[string]string{
		"rules/broken.md": "---\nname: broken\n\nbody\n",
	})

	out, err := runDoctor(t)

	if err == nil {
		t.Fatalf("a lint error must fail doctor:\n%s", out)
	}
	if !strings.Contains(err.Error(), "agnostic-ai lint") {
		t.Errorf("the error should point at lint, got %q", err.Error())
	}
	if !strings.Contains(out, "LINT006 [error] "+filepath.FromSlash(".agnostic-ai/rules/broken.md")) {
		t.Errorf("doctor should list the lint error:\n%s", out)
	}
}

func TestDoctor_SaysSpecsHealthyAndAllPassedWhenLintIsClean(t *testing.T) {
	setupDoctorLintProject(t, nil)

	out, err := runDoctor(t)

	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Spec health:\n  ✓ no lint findings\n") {
		t.Errorf("the spec health section should say it is clean:\n%s", out)
	}
	if !strings.Contains(out, "All checks passed. Nothing to do.") {
		t.Errorf("a clean run should say all checks passed:\n%s", out)
	}
}

func TestDoctorJSON_ListsLintFindingsAndFailsOnError(t *testing.T) {
	setupDoctorLintProject(t, map[string]string{
		"rules/broken.md": "---\nname: broken\n\nbody\n",
	})

	out, err := runDoctor(t, "--json")

	if err == nil {
		t.Fatalf("a lint error must fail doctor --json:\n%s", out)
	}
	var got struct {
		Writes []json.RawMessage `json:"writes"`
		Lint   []struct {
			Code     string `json:"code"`
			Severity string `json:"severity"`
			Path     string `json:"path"`
			Message  string `json:"message"`
		} `json:"lint"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, out)
	}
	if len(got.Writes) != 0 || len(got.Lint) != 1 {
		t.Fatalf("want no drift and one lint finding, got %+v\n%s", got, out)
	}
	if f := got.Lint[0]; f.Code != "LINT006" || f.Severity != "error" || f.Path != filepath.FromSlash(".agnostic-ai/rules/broken.md") || f.Message == "" {
		t.Errorf("unexpected lint finding: %+v", f)
	}
}

func TestDoctorJSON_EmitsEmptyLintListWhenClean(t *testing.T) {
	setupDoctorLintProject(t, nil)

	out, err := runDoctor(t, "--json")

	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"lint": []`) {
		t.Errorf("a clean run should emit an empty lint list:\n%s", out)
	}
}
