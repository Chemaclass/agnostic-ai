package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSyncVerbose_ShowsNewDiagnosticsAfterJSONSync(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [cline, codex]\n")
	skill := filepath.Join(dir, ".agnostic-ai", "skills", "deploy.md")
	mustWriteFile(t, skill, "---\nname: deploy\nargument-hint: '[version]'\n---\nDeploy.\n")
	silence(t)
	captureLog(t)
	notes := captureNotes(t)
	if _, err := runCLI(t, "sync", "-v"); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, skill, "---\nname: deploy\nargument-hint: '[version]'\ndisable-model-invocation: true\n---\nDeploy.\n")
	notes.Reset()
	out := runSyncJSONDrops(t)
	found := false
	for _, note := range out.Notes {
		found = found || strings.Contains(note.Message, "`disable-model-invocation`")
	}
	if !found {
		t.Fatalf("JSON sync did not report the new diagnostic: %+v", out.Notes)
	}
	notes.Reset()
	if _, err := runCLI(t, "sync", "-v"); err != nil {
		t.Fatal(err)
	}
	if got := notes.String(); !strings.Contains(got, "`disable-model-invocation`") || strings.Contains(got, "`argument-hint`") {
		t.Errorf("human sync should report only the unseen note after JSON sync:\n%s", got)
	}
}

func TestSyncVerbose_ReportsKeptHandEditWhenBackupExists(t *testing.T) {
	handEditProject(t, "claude")
	edited := readFile(t, handEditSkill) + "my local tweak\n"
	mustWriteFile(t, handEditSkill, edited)
	mustWriteFile(t, handEditSkill+".bak", "earlier edit\n")
	log := captureLog(t)
	if _, err := runCLI(t, "sync", "-v"); err != nil {
		t.Fatal(err)
	}
	if got := log.String(); !strings.Contains(got, "kept a hand edit to "+handEditSkill) || !strings.Contains(got, "already holds an earlier one") {
		t.Errorf("compact output omitted the backup warning:\n%s", got)
	}
	if got := readFile(t, handEditSkill); got != edited {
		t.Errorf("sync changed the kept edit: %q", got)
	}
}

func TestSyncVerboseFull_ListsEveryChangedPathAndAcceptedNote(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n"+acceptCodexTools)
	mustWriteFile(t, ".agnostic-ai/agents/rev.md", "---\nname: rev\ndescription: Reviews code.\ntools: [Read, Grep]\n---\nReview.\n")
	for i := 0; i < 5; i++ {
		mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", fmt.Sprintf("rule%d.md", i)), fmt.Sprintf("---\nname: rule%d\n---\nRule.\n", i))
	}
	silence(t)
	log := captureLog(t)
	notes := captureNotes(t)
	if _, err := runCLI(t, "sync", "-vv"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		path := fmt.Sprintf(".claude/rules/rule%d.md", i)
		if !strings.Contains(log.String(), path) {
			t.Errorf("full output omitted %s:\n%s", path, log.String())
		}
	}
	if got := notes.String(); !strings.Contains(got, "accepted: "+codexToolsNote) || !strings.Contains(got, "reason: Codex limits come from sandbox_mode.") {
		t.Errorf("full output omitted the accepted note and reason:\n%s", got)
	}
}
