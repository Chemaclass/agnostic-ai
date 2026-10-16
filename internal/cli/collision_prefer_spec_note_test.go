package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func seedPreferSpecNoteProject(t *testing.T, targets string) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: "+targets+`
gitignore:
  enabled: false
outputs:
  gemini:
    skills-dir: .agents/skills
  opencode:
    skills-dir: .agents/skills
sync:
  collision-policy: prefer-spec
`)
	for _, name := range []string{"alpha", "beta", "gamma", "delta"} {
		mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "skills", name, "SKILL.md"),
			"---\nname: "+name+"\ndescription: Skill "+name+" does things.\nlicense: MIT\nmetadata:\n  owner: team\n---\n\nBody "+name+".\n")
	}
	return dir
}

func TestRunSyncOnce_PreferSpecNamesWinnerOnce(t *testing.T) {
	seedPreferSpecNoteProject(t, "[codex, amp, gemini, opencode]")
	silence(t)
	notes := captureNotes(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	want := "note: prefer-spec: 4 shared files use opencode's version over codex, amp, gemini (.agents/skills/alpha/SKILL.md, .agents/skills/beta/SKILL.md, .agents/skills/delta/SKILL.md, ...); reorder targets to change it"
	if got := strings.Count(notes.String(), "prefer-spec:"); got != 1 {
		t.Fatalf("want one prefer-spec note, got %d:\n%s", got, notes.String())
	}
	if !strings.Contains(notes.String(), want) {
		t.Errorf("note differs, want %q in:\n%s", want, notes.String())
	}
}

func TestRunSyncOnce_PreferSpecNoteRepeatsOnlyWhenWinnerChanges(t *testing.T) {
	dir := seedPreferSpecNoteProject(t, "[codex, amp, gemini, opencode]")
	silence(t)
	log := captureLog(t)
	notes := captureNotes(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	notes.Reset()
	log.Reset()
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(notes.String()+log.String(), "prefer-spec:") {
		t.Errorf("unchanged note re-printed:\n%s%s", log.String(), notes.String())
	}

	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic-ai.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	reordered := strings.Replace(string(cfg), "[codex, amp, gemini, opencode]", "[codex, amp, opencode, gemini]", 1)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), reordered)
	notes.Reset()
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(notes.String(), "use gemini's version over opencode (") {
		t.Errorf("reorder did not change the note:\n%s", notes.String())
	}
}
