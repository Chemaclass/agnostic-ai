package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// doctor is the CI gate on project health, so each case below printed a
// problem or skipped a check and still exited 0 (#2659).

func TestDoctor_UnknownTargetFailsLikeSync(t *testing.T) {
	setupDoctorLintProject(t, nil)

	for _, args := range [][]string{
		{"-t", "bogus", "--scope", "project"},
		{"-t", "bogus", "--scope", "project", "--json"},
	} {
		out, err := runDoctor(t, args...)

		if errs.CodeOf(err) != errs.CodeSyncTargetUnknown {
			t.Errorf("doctor %v: want AAI-301, got %v\n%s", args, err, out)
		}
		if strings.Contains(out, "All checks passed") {
			t.Errorf("doctor %v: an unknown target must not pass:\n%s", args, out)
		}
	}
}

func TestDoctorMCP_FailsOnBrokenConfig(t *testing.T) {
	dir := newProject(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.local.yaml"), "targets: claude\n")

	_, err := runCLI(t, "doctor", "mcp")

	if errs.CodeOf(err) != errs.CodeConfigDecode {
		t.Fatalf("doctor mcp on a config sync rejects: want AAI-004, got %v", err)
	}
}

func TestDoctorJSON_RejectsFix(t *testing.T) {
	setupDoctorLintProject(t, nil)

	_, err := runDoctor(t, "--json", "--fix")

	if errs.CodeOf(err) != errs.CodeFlagConflict {
		t.Fatalf("doctor --json --fix: want a flag conflict, got %v", err)
	}
}

type doctorGateJSON struct {
	Globs *[]struct {
		Rule    string `json:"rule"`
		Globs   string `json:"globs"`
		Invalid string `json:"invalid"`
	} `json:"globs"`
	NestedCopies []struct {
		Path string `json:"path"`
		Rule string `json:"rule"`
	} `json:"nested_copies"`
	HookScriptDrift []struct {
		Script string   `json:"script"`
		Tools  []string `json:"tools"`
	} `json:"hook_script_drift"`
}

func parseDoctorGateJSON(t *testing.T, out string) doctorGateJSON {
	t.Helper()
	var got doctorGateJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, out)
	}
	return got
}

func TestDoctorJSON_CheckGlobsListsUnmatchedAndFails(t *testing.T) {
	setupDoctorLintProject(t, map[string]string{
		"rules/dead.md": "---\nname: dead\ndescription: d\nglobs: does-not-exist/**\n---\nbody\n",
	})

	out, err := runDoctor(t, "--json", "--check-globs", "--scope", "project")

	if err == nil || !strings.Contains(err.Error(), "glob") {
		t.Fatalf("a rule that never loads must fail doctor --json as it fails text mode, got %v\n%s", err, out)
	}
	got := parseDoctorGateJSON(t, out)
	if got.Globs == nil || len(*got.Globs) != 1 {
		t.Fatalf("want one unmatched glob, got %+v\n%s", got.Globs, out)
	}
	if g := (*got.Globs)[0]; g.Rule != "dead" || g.Globs != "does-not-exist/**" {
		t.Errorf("unexpected glob finding: %+v", g)
	}
}

func TestDoctorJSON_OmitsGlobsWithoutFlag(t *testing.T) {
	setupDoctorLintProject(t, nil)

	out, err := runDoctor(t, "--json", "--scope", "project")

	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, out)
	}
	if got := parseDoctorGateJSON(t, out); got.Globs != nil {
		t.Errorf("globs must be absent without --check-globs:\n%s", out)
	}
	for _, want := range []string{`"nested_copies": []`, `"hook_script_drift": []`} {
		if !strings.Contains(out, want) {
			t.Errorf("a clean run should emit %s:\n%s", want, out)
		}
	}
}

func TestDoctorJSON_ListsNestedClaudeCopiesAndFails(t *testing.T) {
	setupDoctorLintProject(t, map[string]string{"rules/a.md": nestedClaudeRuleA})
	syncProject(t)
	mustWriteFile(t, filepath.Join("src", "a", "CLAUDE.md"), "# Module A\n\nA facts.\n")

	out, err := runDoctor(t, "--json", "--scope", "project")

	if err == nil {
		t.Fatalf("a nested copy fails text mode, so it must fail --json:\n%s", out)
	}
	got := parseDoctorGateJSON(t, out)
	if len(got.NestedCopies) != 1 || got.NestedCopies[0].Path != "src/a/CLAUDE.md" || got.NestedCopies[0].Rule != "a" {
		t.Errorf("want src/a/CLAUDE.md held by rule a, got %+v\n%s", got.NestedCopies, out)
	}
}

func TestDoctorJSON_ListsDivergentHookScriptsAndFails(t *testing.T) {
	setupDoctorLintProject(t, nil)
	mustWriteHookScript(t, ".", "claude", "guard.sh", "echo claude\n")
	mustWriteHookScript(t, ".", "codex", "guard.sh", "echo codex\n")

	out, err := runDoctor(t, "--json", "--scope", "project")

	if err == nil {
		t.Fatalf("divergent hook scripts fail text mode, so they must fail --json:\n%s", out)
	}
	got := parseDoctorGateJSON(t, out)
	if len(got.HookScriptDrift) != 1 || got.HookScriptDrift[0].Script != "guard.sh" ||
		strings.Join(got.HookScriptDrift[0].Tools, ",") != "claude,codex" {
		t.Errorf("want guard.sh diverging across claude and codex, got %+v\n%s", got.HookScriptDrift, out)
	}
}
