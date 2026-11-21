package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func setupDoctorProjectScope(t *testing.T) (string, string) {
	t.Helper()
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex, cursor]\n")
	writeFile(t, ".agnostic-ai/hooks/guard.yaml", "name: guard\ndescription: Check the project command.\non: before-tool\nmatch: shell\ncommand: echo project-guard\n")
	syncProject(t)
	return dir, home
}

func TestDoctor_ProjectScopePassesWithoutLocalTrustAndReportsSkipped(t *testing.T) {
	dir, home := setupDoctorProjectScope(t)
	before := snapshotTree(t, dir)
	text, err := runDoctor(t, "--checks", "project")
	if err != nil {
		t.Fatalf("clean project doctor must pass without local hook trust: %v\n%s", err, text)
	}
	if !strings.Contains(text, "SKIPPED") || !strings.Contains(text, "Codex hook trust") || !strings.Contains(text, "project scope") {
		t.Errorf("project doctor must identify skipped runtime trust: %s", text)
	}
	raw, err := runDoctor(t, "--checks", "project", "--json")
	if err != nil {
		t.Fatalf("clean JSON project doctor must pass: %v\n%s", err, raw)
	}
	var report struct {
		HookTrustCheck struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"hook_trust_check"`
		HookTrust []json.RawMessage `json:"hook_trust"`
		Writes    []json.RawMessage `json:"writes"`
	}
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		t.Fatal(err)
	}
	if report.HookTrustCheck.Status != "skipped" || !strings.Contains(report.HookTrustCheck.Reason, "project scope") || len(report.HookTrust) != 0 || len(report.Writes) != 0 {
		t.Errorf("project JSON must distinguish skipped trust from no findings: %s", raw)
	}
	if after := snapshotTree(t, dir); !reflect.DeepEqual(before, after) {
		t.Error("project doctor changed project files")
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("project doctor populated CODEX_HOME: %v", entries)
	}
	for _, path := range []string{filepath.Join(dir, ".claude/settings.json"), filepath.Join(dir, ".codex/hooks.json"), filepath.Join(dir, ".cursor/hooks.json")} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("acceptance fixture missing selected target output %s: %v", path, err)
		}
	}
}

func TestDoctor_ProjectScopeStillFailsOnDriftAndRequestedReferences(t *testing.T) {
	dir, _ := setupDoctorProjectScope(t)
	writeFile(t, ".agnostic-ai/skills/demo/SKILL.md", "---\nname: demo\ndescription: Demo skill.\n---\nRead [missing](missing.md).\n")
	syncProject(t)
	for _, format := range [][]string{nil, {"--json"}} {
		args := append([]string{"--checks", "project", "--check-references"}, format...)
		out, err := runDoctor(t, args...)
		if err == nil || !strings.Contains(err.Error(), "broken skill reference") || !strings.Contains(out, "missing.md") {
			t.Errorf("project scope must keep requested reference errors: %v\n%s", err, out)
		}
	}
	writeFile(t, filepath.Join(dir, ".cursor/hooks.json"), "{\"edited\":true}\n")
	for _, format := range [][]string{nil, {"--json"}} {
		args := append([]string{"--checks", "project"}, format...)
		out, err := runDoctor(t, args...)
		if err == nil || !strings.Contains(err.Error(), "drift") {
			t.Errorf("project scope must keep selected Cursor drift failure: %v\n%s", err, out)
		}
	}
}

func TestDoctor_ProjectScopeStillFailsOnLintErrors(t *testing.T) {
	setupDoctorLintProject(t, map[string]string{"rules/broken.md": "---\nname: broken\n\nbody\n"})
	for _, format := range [][]string{nil, {"--json"}} {
		out, err := runDoctor(t, append([]string{"--checks", "project"}, format...)...)
		if err == nil || !strings.Contains(out, "LINT006") {
			t.Errorf("project scope must retain lint errors: %v\n%s", err, out)
		}
	}
}

func TestDoctor_ProjectScopeFixRepairsDriftWithoutApprovingTrust(t *testing.T) {
	dir, home := setupDoctorProjectScope(t)
	path := filepath.Join(dir, ".claude/rules/r1.md")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, "hand-edited rule\n")
	out, err := runDoctor(t, "--checks", "project", "--fix")
	if err != nil || !strings.Contains(out, "SKIPPED") {
		t.Errorf("project fix must repair drift and skip trust: %v\n%s", err, out)
	}
	repaired, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(original) != string(repaired) {
		t.Error("project fix did not repair selected Claude rule")
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Error("project fix populated local trust")
	}
	out, err = runDoctor(t)
	if err == nil || !strings.Contains(out, "untrusted") {
		t.Errorf("default doctor must still require local hook approval: %v\n%s", err, out)
	}
}

func TestDoctor_ProjectScopeKeepsPackagingAdvisories(t *testing.T) {
	setupDoctorProjectScope(t)
	writeFile(t, ".vscodeignore", ".claude/**\n.codex/**\n")
	for _, format := range [][]string{nil, {"--json"}} {
		out, err := runDoctor(t, append([]string{"--checks", "project"}, format...)...)
		if err != nil || !strings.Contains(out, ".vscodeignore") {
			t.Errorf("project scope must preserve advisory packaging results: %v\n%s", err, out)
		}
	}
}

func TestDoctor_AllScopeKeepsStrictTrustClassification(t *testing.T) {
	for _, test := range []struct {
		name, status string
		failing      bool
	}{
		{"untrusted", "untrusted", true},
		{"modified", "modified", true},
		{"unreadable", "unknown", true},
		{"disabled", "disabled", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, home := setupDoctorProjectScope(t)
			dir, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			key := filepath.Join(dir, ".codex/hooks.json") + ":pre_tool_use:0:0"
			path := filepath.Join(home, "config.toml")
			switch test.name {
			case "modified":
				writeFile(t, path, fmt.Sprintf("[hooks.state.%q]\ntrusted_hash = \"sha256:changed\"\n", key))
			case "unreadable":
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			case "disabled":
				writeFile(t, path, fmt.Sprintf("[hooks.state.%q]\nenabled = false\n", key))
			}
			for _, scope := range [][]string{nil, {"--checks", "all"}} {
				for _, format := range [][]string{nil, {"--json"}} {
					out, err := runDoctor(t, append(append([]string{}, scope...), format...)...)
					expected := test.status
					if test.name == "unreadable" && len(format) == 0 {
						expected = "cannot check hook trust"
					}
					if (err != nil) != test.failing || !strings.Contains(out, expected) || strings.Contains(out, "SKIPPED") {
						t.Errorf("default/all trust classification changed: %v\n%s", err, out)
					}
				}
			}
		})
	}
}

func TestDoctor_ProjectScopeRejectsUnknownScopeBeforeReadingProject(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	out, err := runDoctor(t, "--checks", "runtime")
	if err == nil || !strings.Contains(err.Error(), "--checks") || !strings.Contains(err.Error(), "runtime") || strings.Contains(out, "Config:") {
		t.Errorf("invalid checks must fail before diagnostics: %v\n%s", err, out)
	}
}

// doctor --scope chose the checks before --scope meant where specs live.
// It still does, with a notice on stderr and the same stdout.
func TestDoctor_ScopeStillChoosesTheChecksAndWarns(t *testing.T) {
	setupDoctorProjectScope(t)
	for _, value := range []string{"project", "all"} {
		newOut, newErr, _ := runSplit(t, "doctor", "--checks", value, "--json")
		oldOut, oldErr, _ := runSplit(t, "doctor", "--scope", value, "--json")
		if oldOut != newOut {
			t.Errorf("--scope %s stdout differs from --checks %s\n%s\n%s", value, value, oldOut, newOut)
		}
		if strings.Contains(newErr, "deprecated") {
			t.Errorf("--checks %s warned: %q", value, newErr)
		}
		if strings.Count(oldErr, "deprecated") != 1 || !strings.Contains(oldErr, "--checks "+value) {
			t.Errorf("--scope %s stderr=%q, want one notice naming --checks %s", value, oldErr, value)
		}
	}
}

func TestDoctor_ScopeAndChecksMustAgree(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	if _, err := runDoctor(t, "--scope", "project", "--checks", "all"); err == nil || !strings.Contains(err.Error(), "--checks") {
		t.Errorf("conflicting --scope and --checks must fail, got %v", err)
	}
}

func TestDoctor_HelpNamesChecksNotScope(t *testing.T) {
	out, err := runDoctor(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "--checks") || strings.Contains(out, "--scope") {
		t.Errorf("help must list --checks and hide --scope:\n%s", out)
	}
}
