package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const handEditSkill = ".claude/skills/review/SKILL.md"

func handEditProject(t *testing.T, targets string) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+targets+"]\n")
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", "---\nname: review\ndescription: Review code.\n---\nReview.\n")
	runSyncOK(t)
}

// A hand edit to a generated file was overwritten with no trace (#1590).
func TestSync_KeepsAHandEditAsABackupBeforeWritingOverIt(t *testing.T) {
	handEditProject(t, "claude")
	generated := readFile(t, handEditSkill)
	edited := generated + "my local tweak\n"
	mustWriteFile(t, handEditSkill, edited)
	log := captureLog(t)

	runSyncOK(t)

	if got := readFile(t, handEditSkill); got != generated {
		t.Errorf("sync left %q, want the spec's version", got)
	}
	if got := readFile(t, handEditSkill+".bak"); got != edited {
		t.Errorf(".bak = %q, want the hand edit", got)
	}
	if want := "overwrote a hand edit to " + handEditSkill + " (saved as " + handEditSkill + ".bak)"; !strings.Contains(log.String(), want) {
		t.Errorf("sync output does not say %q:\n%s", want, log.String())
	}
}

func TestSync_SpecChangeAloneWritesNoBackup(t *testing.T) {
	handEditProject(t, "claude")
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", "---\nname: review\ndescription: Review code.\n---\nReview twice.\n")

	runSyncOK(t)

	if _, err := os.Stat(handEditSkill + ".bak"); err == nil {
		t.Error("a spec change wrote a backup")
	}
}

// Targets sharing a file each write it; the second must not back up the
// first one's write over the user's edit.
func TestSync_SharedFileKeepsTheHandEditInItsBackup(t *testing.T) {
	handEditProject(t, "codex, opencode")
	edited := readFile(t, "AGENTS.md") + "my local tweak\n"
	mustWriteFile(t, "AGENTS.md", edited)

	runSyncOK(t)

	if got := readFile(t, "AGENTS.md.bak"); got != edited {
		t.Errorf("AGENTS.md.bak = %q, want the hand edit", got)
	}
}

func TestSyncJSON_NamesTheBackupOfAHandEdit(t *testing.T) {
	handEditProject(t, "claude")
	mustWriteFile(t, handEditSkill, readFile(t, handEditSkill)+"my local tweak\n")

	out, err := runCLI(t, "sync", "--json")
	if err != nil {
		t.Fatalf("sync --json: %v\n%s", err, out)
	}

	var got jsonOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	for _, w := range got.Writes {
		if w.Path == handEditSkill && w.Backup == handEditSkill+".bak" {
			return
		}
	}
	t.Errorf("no write names the backup of %s:\n%s", handEditSkill, out)
}

// --keep-edits still leaves the edit in place and writes no backup.
func TestSync_KeepEditsStillLeavesTheEditInPlace(t *testing.T) {
	handEditProject(t, "claude")
	edited := readFile(t, handEditSkill) + "my local tweak\n"
	mustWriteFile(t, handEditSkill, edited)

	runSyncOK(t, "--keep-edits")

	if got := readFile(t, handEditSkill); got != edited {
		t.Errorf("--keep-edits wrote over the edit: %q", got)
	}
	if _, err := os.Stat(handEditSkill + ".bak"); err == nil {
		t.Error("--keep-edits wrote a backup")
	}
}

// A merged settings file holds the user's keys by design, so a key they
// add there is not a hand edit to back up.
func TestSync_UserKeyInAMergedFileWritesNoBackup(t *testing.T) {
	handEditProject(t, "claude")
	mustWriteFile(t, ".agnostic-ai/hooks/fmt.yaml", "name: fmt\nevent: PostToolUse\ncommand: gofmt -l .\n")
	runSyncOK(t)
	settings := readFile(t, ".claude/settings.json")
	mustWriteFile(t, ".claude/settings.json", strings.Replace(settings, "{", "{\n  \"theme\": \"dark\",", 1))
	mustWriteFile(t, ".agnostic-ai/hooks/fmt.yaml", "name: fmt\nevent: PostToolUse\ncommand: gofmt -l ./...\n")

	runSyncOK(t)

	if !strings.Contains(readFile(t, ".claude/settings.json"), `"theme"`) {
		t.Fatal("sync dropped the user's key")
	}
	if _, err := os.Stat(".claude/settings.json.bak"); err == nil {
		t.Error("a user key in a merged file wrote a backup")
	}
}

func TestRevert_RestoresAHandEditSyncBackedUp(t *testing.T) {
	handEditProject(t, "claude")
	edited := readFile(t, handEditSkill) + "my local tweak\n"
	mustWriteFile(t, handEditSkill, edited)
	runSyncOK(t)

	if out, err := runCLI(t, "revert"); err != nil {
		t.Fatalf("revert: %v\n%s", err, out)
	}

	if got := readFile(t, handEditSkill); got != edited {
		t.Errorf("revert left %q, want the hand edit", got)
	}
}
