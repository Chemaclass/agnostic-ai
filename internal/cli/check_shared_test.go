package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// syncSharedSkillThenEdit syncs a skill that codex and amp both read from
// .agents/skills/ and gemini reads from its own tree, then edits the source
// so every copy drifts.
func syncSharedSkillThenEdit(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex, gemini, amp]\n")
	skill := filepath.Join(dir, ".agnostic-ai", "skills", "style", "SKILL.md")
	writeFile(t, skill, "---\nname: style\ndescription: Style guide.\n---\n\nBody one.\n")
	execCLI(t, "sync")
	writeFile(t, skill, "---\nname: style\ndescription: Style guide.\n---\n\nBody two.\n")
}

func runCheck(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"sync", "--check"}, args...))
	if err := root.Execute(); err == nil {
		t.Fatal("expected drift")
	}
	return out.String()
}

func TestSyncCheck_ListsASharedFileOnceWithItsTargets(t *testing.T) {
	syncSharedSkillThenEdit(t)
	logged := captureLogOut(t)
	runCheck(t)

	got := logged.String()
	if n := strings.Count(got, ".agents/skills/style/SKILL.md"); n != 1 {
		t.Errorf("shared file listed %d times, want once:\n%s", n, got)
	}
	if !strings.Contains(got, ".agents/skills/style/SKILL.md (shared with amp)") {
		t.Errorf("expected the shared file to name its other target:\n%s", got)
	}
	if !strings.Contains(got, ".gemini/skills/style/SKILL.md") {
		t.Errorf("expected gemini's own copy:\n%s", got)
	}
	if strings.Contains(got, "amp: drift") {
		t.Errorf("amp has no file of its own, so it needs no section:\n%s", got)
	}
}

func TestSyncCheck_DiffPrintsASharedHunkOnce(t *testing.T) {
	syncSharedSkillThenEdit(t)
	captureLogOut(t)
	got := runCheck(t, "--diff")
	if n := strings.Count(got, "--- .agents/skills/style/SKILL.md (on disk)"); n != 1 {
		t.Errorf("shared hunk printed %d times, want once:\n%s", n, got)
	}
}

func TestSyncCheck_GitHubAnnotatesASharedFileOnce(t *testing.T) {
	syncSharedSkillThenEdit(t)
	captureLogOut(t)
	got := runCheck(t, "--format", "github")
	if n := strings.Count(got, "::error file=.agents/skills/style/SKILL.md"); n != 1 {
		t.Errorf("shared file annotated %d times, want once:\n%s", n, got)
	}
}

func TestStatus_CountsASharedFileOnce(t *testing.T) {
	syncSharedSkillThenEdit(t)
	out, _ := runCLI(t, "status")
	if !strings.Contains(out, "Drift:   2 files out of date") {
		t.Errorf("expected 2 distinct drifted files:\n%s", out)
	}
}

func TestSyncCheck_JSONKeepsOneRecordPerTarget(t *testing.T) {
	syncSharedSkillThenEdit(t)
	captureLogOut(t)
	got := runCheck(t, "--json")
	if n := strings.Count(got, ".agents/skills/style/SKILL.md"); n < 2 {
		t.Errorf("--json shape changed: shared file appears %d times:\n%s", n, got)
	}
}
