package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func runAsVersion(version string, args ...string) (string, string, error) {
	var out, errOut bytes.Buffer
	cmd := NewRootCmd(version)
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// requiresProject writes a claude-only project with one rule whose
// config sets requires.
func requiresProject(t *testing.T, requires string) {
	t.Helper()
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\nrequires: \""+requires+"\"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "safe.md"), "---\nname: safe\n---\nBe safe.\n")
}

func assertUnmetRequires(t *testing.T, err error, config string) {
	t.Helper()
	if errs.CodeOf(err) != errs.CodeRequiresUnmet {
		t.Fatalf("want %s, got %v", errs.CodeRequiresUnmet, err)
	}
	for _, want := range []string{config, ">=0.70.0", "0.69.0", "`agnostic-ai upgrade`"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message misses %q: %v", want, err)
		}
	}
}

func TestProjectCommands_StopWhenInstalledVersionIsBelowRequires(t *testing.T) {
	requiresProject(t, ">=0.70.0")

	for _, args := range [][]string{{"sync"}, {"sync", "--check"}, {"lint"}, {"validate"}} {
		_, _, err := runAsVersion("0.69.0", args...)
		assertUnmetRequires(t, err, "agnostic-ai.yaml")
	}
	for _, rel := range []string{"CLAUDE.md", ".claude", ".agnostic-ai/.sync-state"} {
		if _, err := os.Stat(rel); !os.IsNotExist(err) {
			t.Errorf("an unmet requires wrote %s: %v", rel, err)
		}
	}
}

func TestSync_RunsWhenInstalledVersionMeetsRequires(t *testing.T) {
	requiresProject(t, ">=0.70.0")
	silence(t)

	if _, _, err := runAsVersion("0.70.0", "sync"); err != nil {
		t.Fatalf("sync on 0.70.0: %v", err)
	}
	if _, err := os.Stat(filepath.Join(".claude", "rules", "safe.md")); err != nil {
		t.Errorf("expected the rule written: %v", err)
	}
	if _, _, err := runAsVersion("0.70.1", "sync", "--check"); err != nil {
		t.Errorf("sync --check on 0.70.1: %v", err)
	}
}

func TestSync_DevBuildWarnsAndSkipsRequires(t *testing.T) {
	requiresProject(t, ">=0.70.0")
	silence(t)

	_, errOut, err := runAsVersion("dev", "sync")
	if err != nil {
		t.Fatalf("a dev build must not be blocked: %v", err)
	}
	want := "warning: agnostic-ai.yaml: requires >=0.70.0, but dev is not a release build; not checked"
	if !strings.Contains(errOut, want) {
		t.Errorf("want %q, got:\n%s", want, errOut)
	}
}

func TestSync_InvalidRequiresNamesFileAndKey(t *testing.T) {
	requiresProject(t, "0.70")

	_, _, err := runAsVersion("0.69.0", "sync")
	if errs.CodeOf(err) != errs.CodeConfigDecode || !strings.Contains(err.Error(), "agnostic-ai.yaml: requires:") {
		t.Errorf("want AAI-004 naming agnostic-ai.yaml and requires, got %v", err)
	}
}

func TestGlobalCommands_StopWhenInstalledVersionIsBelowRequires(t *testing.T) {
	home, source := globalConfigTestHome(t)
	config := filepath.Join(source, "agnostic-ai.yaml")
	mustWriteGlobalTest(t, config, "requires: \">=0.70.0\"\ntargets: [claude]\n")

	for _, args := range [][]string{{"sync"}, {"sync", "--check"}, {"sync", "-t", "claude"}, {"lint"}, {"validate"}} {
		_, _, err := runAsVersion("0.69.0", append(args, "--global")...)
		assertUnmetRequires(t, err, config)
	}
	assertGlobalInstructions(t, home, map[string]bool{".claude/CLAUDE.md": false})

	_, errOut, err := runAsVersion("0.70.0", "sync", "--global")
	if err != nil {
		t.Fatalf("sync --global on 0.70.0: %v", err)
	}
	if strings.Contains(errOut, "ignoring") {
		t.Errorf("the home config must accept requires, got:\n%s", errOut)
	}
	assertGlobalInstructions(t, home, map[string]bool{".claude/CLAUDE.md": true})
}

func TestGlobalSync_LocalHomeConfigRequiresReplacesShared(t *testing.T) {
	_, source := globalConfigTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "requires: \">=0.60.0\"\n")
	local := filepath.Join(source, "local", "agnostic-ai.yaml")
	mustWriteGlobalTest(t, local, "requires: \">=0.70.0\"\n")

	_, _, err := runAsVersion("0.69.0", "sync", "--global")
	assertUnmetRequires(t, err, local)
}

func TestGlobalSync_InvalidRequiresNamesFileAndKey(t *testing.T) {
	_, source := globalConfigTestHome(t)
	config := filepath.Join(source, "agnostic-ai.yaml")
	mustWriteGlobalTest(t, config, "requires: [\">=0.70.0\"]\n")

	_, _, err := runAsVersion("0.69.0", "sync", "--global")
	if errs.CodeOf(err) != errs.CodeConfigDecode || !strings.Contains(err.Error(), config+": requires:") {
		t.Errorf("want AAI-004 naming %s and requires, got %v", config, err)
	}
}
