package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncGlobal_CheckDiffPrintsTheManagedBlockOnly(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "# Agreements\n\n- Rule one.\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".claude", "CLAUDE.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, path, "My own notes.\n\n"+strings.Replace(string(data), "- Rule one.", "- Rule one, edited by hand.", 1))
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "tidy", "SKILL.md"), "---\nname: tidy\ndescription: Tidy\n---\nTidy.\n")

	out, _, err := runGlobalAgentTest("--only", "claude", "--check", "--diff")
	if err == nil {
		t.Fatal("drift must still exit non-zero")
	}
	for _, want := range []string{
		"--- " + filepath.ToSlash(path) + " (on disk)",
		"-- Rule one, edited by hand.",
		"+- Rule one.",
		"would create " + filepath.ToSlash(filepath.Join(home, ".claude", "skills", "tidy", "SKILL.md")),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("diff output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "My own notes.") {
		t.Errorf("text outside the managed block is not part of the diff:\n%s", out)
	}
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "SKILL.md") {
		t.Errorf("the error must name every drifted file, got %v", err)
	}
}

func TestSyncGlobal_CheckDiffIsSilentWhenInSync(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Be terse.\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	out, _, err := runGlobalAgentTest("--only", "claude", "--check", "--diff")
	if err != nil || out != "" {
		t.Errorf("an in-sync check prints nothing, got err = %v, out:\n%s", err, out)
	}
}

func TestSyncGlobal_DiffRequiresCheck(t *testing.T) {
	globalAgentTestHome(t)
	if _, _, err := runGlobalAgentTest("--diff"); err == nil || !strings.Contains(err.Error(), "--check") {
		t.Errorf("--diff without --check must be rejected, got %v", err)
	}
}
