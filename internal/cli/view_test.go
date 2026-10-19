package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// demoProject copies the Authority demo project into a fresh directory
// and returns it, so a test never touches the checked-in copy.
func demoProject(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "demo-project")
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "authority", "testdata", "demo-project"))); err != nil {
		t.Fatalf("copy demo project: %v", err)
	}
	return dir
}

func runView(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"view", "authority"}, args...))
	err := root.Execute()
	return out.String(), err
}

func viewJSON(t *testing.T, args ...string) authority.View {
	t.Helper()
	out, err := runView(t, append(args, "--json")...)
	if err != nil {
		t.Fatalf("view authority: %v\n%s", err, out)
	}
	var v authority.View
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("parse view: %v\n%s", err, out)
	}
	return v
}

func viewOperation(t *testing.T, v authority.View, id string) authority.Operation {
	t.Helper()
	for _, r := range v.Resources {
		for _, op := range r.Operations {
			if op.ID == id {
				return op
			}
		}
	}
	t.Fatalf("no operation %s", id)
	return authority.Operation{}
}

func TestViewAuthority_JSONOnTheDemoProjectIsAView(t *testing.T) {
	silence(t)
	cwd, _ := os.Getwd()
	v := viewJSON(t, "--dir", demoProject(t), "--agent", "coding-agent", "--target", "claude")

	if v.SchemaVersion != authority.SchemaVersion || v.Agent.ID != "coding-agent" || v.Snapshot.Project.Name != "demo-project" {
		t.Fatalf("view = %s, %s, %s", v.SchemaVersion, v.Agent.ID, v.Snapshot.Project.Name)
	}
	roots := 0
	for _, r := range v.Resources {
		if r.Kind == authority.ResourceDeclaredRoot {
			roots++
		}
	}
	if roots == 0 {
		t.Error("no declared-root resource")
	}
	if v.Harness.Installed.State != authority.InstalledNotGenerated || len(v.Harness.Installed.Files) == 0 {
		t.Errorf("installed = %+v, want not-generated on a fresh copy", v.Harness.Installed)
	}
	if after, _ := os.Getwd(); after != cwd {
		t.Errorf("working directory moved to %s", after)
	}
}

func TestViewAuthority_TextSummaryNamesEachOperation(t *testing.T) {
	silence(t)
	out, err := runView(t, "--dir", demoProject(t), "--agent", "coding-agent", "--target", "claude")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Authority: coding-agent on Claude Code",
		"Deploy: would be permitted (allow mcp:railway/deploy · settings/permissions.yaml) · Claude Code: native rule mcp__railway__deploy · installed: not-generated · runtime: not observed",
		"Uncertainties",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("summary lacks %q:\n%s", want, out)
		}
	}
}

func TestViewAuthority_StaleGeneratedFileMakesTheEffectHypothetical(t *testing.T) {
	silence(t)
	dir := demoProject(t)
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte("{\"hand\": \"edited\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := viewJSON(t, "--dir", dir, "--agent", "coding-agent", "--target", "claude")

	if v.Harness.Installed.State != authority.InstalledStale {
		t.Fatalf("installed = %s, want stale", v.Harness.Installed.State)
	}
	deploy := viewOperation(t, v, "mcp-railway-scope:deploy")
	if deploy.Installed != authority.InstalledStale || !deploy.Hypothetical {
		t.Errorf("deploy = installed %s, hypothetical %v", deploy.Installed, deploy.Hypothetical)
	}
	if !strings.HasPrefix(deploy.Explanation, "Would be permitted once .claude/settings.json is synced") {
		t.Errorf("explanation = %q", deploy.Explanation)
	}
}

func TestViewAuthority_GlobalReadsTheUserLevelConfiguration(t *testing.T) {
	silence(t)
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "permissions.yaml"), "permissions:\n  allow: [shell(go test:*)]\n")
	v := viewJSON(t, "--global")

	if !strings.HasPrefix(v.Snapshot.Project.Name, "global") || v.Snapshot.Evidence.Installed != authority.InstalledNotChecked {
		t.Fatalf("snapshot = %+v", v.Snapshot)
	}
	if op := viewOperation(t, v, "shell-go-test:run"); op.Declared != authority.EffectAllowed {
		t.Errorf("go test = %s, want allowed from the home settings spec", op.Declared)
	}
}

func TestViewAuthority_GlobalAndDirCannotCombine(t *testing.T) {
	silence(t)
	testutil.Chdir(t, demoProject(t))
	_, err := runView(t, "--global", "--dir", ".")
	if err == nil || !strings.Contains(err.Error(), "--global and --dir cannot be combined") {
		t.Errorf("error = %v", err)
	}
}

func TestInstalledState_FreshCopyIsNotGenerated(t *testing.T) {
	silence(t)
	testutil.Chdir(t, demoProject(t))
	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	state, err := installedState("claude", cfg, b, ".")
	if err != nil {
		t.Fatal(err)
	}
	if state.State != authority.InstalledNotGenerated || len(state.Files) == 0 || state.CheckedAt == "" {
		t.Fatalf("state = %+v", state)
	}
	for _, f := range state.Files {
		if f.State != authority.InstalledNotGenerated {
			t.Errorf("%s = %s on a fresh copy", f.Path, f.State)
		}
	}
	if _, err := os.Stat(".claude"); !os.IsNotExist(err) {
		t.Errorf("comparing wrote generated files: %v", err)
	}
}
