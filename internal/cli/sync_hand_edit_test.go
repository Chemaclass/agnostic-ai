package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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

// A second edit must not replace the backup of the first, and a link
// planted at the backup path must not be followed.
func TestSync_LeavesAHandEditInPlaceWhenItsBackupExists(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside.txt")
	for name, plant := range map[string]func(){
		"earlier backup": func() { mustWriteFile(t, handEditSkill+".bak", "first edit\n") },
		"planted link": func() {
			if err := os.Symlink(outside, handEditSkill+".bak"); err != nil {
				t.Skip(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			handEditProject(t, "claude")
			plant()
			before, _ := os.Readlink(handEditSkill + ".bak")
			edited := readFile(t, handEditSkill) + "second edit\n"
			mustWriteFile(t, handEditSkill, edited)
			log := captureLog(t)

			runSyncOK(t)

			if got := readFile(t, handEditSkill); got != edited {
				t.Errorf("sync wrote over the edit: %q", got)
			}
			if after, _ := os.Readlink(handEditSkill + ".bak"); after != before {
				t.Errorf("backup link changed: %q to %q", before, after)
			}
			if _, err := os.Stat(outside); err == nil {
				t.Error("sync wrote through the planted link")
			}
			if !strings.Contains(log.String(), "already holds an earlier one") {
				t.Errorf("sync output does not explain the kept edit:\n%s", log.String())
			}
		})
	}
}

// A checkout or pull, not the user, changed an output that now matches
// what Git holds: there is no hand edit to keep.
func TestSync_OutputMatchingTheCommittedVersionIsNotAHandEdit(t *testing.T) {
	handEditProject(t, "claude")
	isolateGit(t)
	gitInit(t, ".")
	mustWriteFile(t, handEditSkill, readFile(t, handEditSkill)+"from another branch\n")
	git(t, ".", "add", "-A", "-f")
	git(t, ".", "commit", "-q", "-m", "outputs")

	runSyncOK(t)

	if _, err := os.Stat(handEditSkill + ".bak"); err == nil {
		t.Error("a file matching its committed version was backed up")
	}
}

func TestSync_BackupKeepsAnExecutableMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no executable bit")
	}
	handEditProject(t, "claude")
	if err := os.Chmod(handEditSkill, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, handEditSkill, readFile(t, handEditSkill)+"my local tweak\n")

	runSyncOK(t)

	info, err := os.Stat(handEditSkill + ".bak")
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Errorf("backup mode = %v, %v; want 0755", info, err)
	}
}

// A backup sync left in a skill folder is not part of the skill.
func TestImport_SkipsABackupInASkillFolder(t *testing.T) {
	handEditProject(t, "claude")
	mustWriteFile(t, handEditSkill, readFile(t, handEditSkill)+"my local tweak\n")
	runSyncOK(t)

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}

	if _, err := os.Stat(".agnostic-ai/skills/review/SKILL.md.bak"); err == nil {
		t.Error("import copied the backup into the skill")
	}
}

func TestImport_KeepsASkillAssetNamedBak(t *testing.T) {
	handEditProject(t, "claude")
	mustWriteFile(t, ".claude/skills/review/examples/patch.bak", "a user asset\n")

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}

	if _, err := os.Stat(".agnostic-ai/skills/review/examples/patch.bak"); err != nil {
		t.Errorf("import dropped a user asset: %v", err)
	}
}

func TestImport_KeepsAUserPairOfFileAndBak(t *testing.T) {
	handEditProject(t, "claude")
	mustWriteFile(t, ".claude/skills/review/examples/patch", "new\n")
	mustWriteFile(t, ".claude/skills/review/examples/patch.bak", "old\n")

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}

	if _, err := os.Stat(".agnostic-ai/skills/review/examples/patch.bak"); err != nil {
		t.Errorf("import dropped a user asset: %v", err)
	}
}

// Only a backup sync recorded is skipped, not a user's own file that
// happens to sit beside a generated one.
func TestImport_KeepsAUserBakBesideAGeneratedFile(t *testing.T) {
	handEditProject(t, "claude")
	mustWriteFile(t, handEditSkill+".bak", "a user asset\n")

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}

	if _, err := os.Stat(".agnostic-ai/skills/review/SKILL.md.bak"); err != nil {
		t.Errorf("import dropped a user asset: %v", err)
	}
}

func TestImport_SkipsASyncBackupCopy(t *testing.T) {
	handEditProject(t, "claude")
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", "---\nname: review\ndescription: Review code.\n---\nReview twice.\n")
	runSyncOK(t, "--backup")

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}

	if _, err := os.Stat(".agnostic-ai/skills/review/SKILL.md.bak"); err == nil {
		t.Error("import copied a sync --backup copy into the skill")
	}
}

// A backup the user has since changed is theirs, so import keeps it.
func TestImport_KeepsABackupTheUserChanged(t *testing.T) {
	handEditProject(t, "claude")
	mustWriteFile(t, handEditSkill, readFile(t, handEditSkill)+"my local tweak\n")
	runSyncOK(t)
	mustWriteFile(t, handEditSkill+".bak", "now a user asset\n")

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}

	if _, err := os.Stat(".agnostic-ai/skills/review/SKILL.md.bak"); err != nil {
		t.Errorf("import dropped a changed backup: %v", err)
	}
}

// With shared skills, a target's skill folder is a link; import walks
// it and must still skip the backup sync made there.
func TestImport_SkipsASyncBackupBehindASharedSkillLink(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\nsync:\n  shared-skills: true\n")
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", "---\nname: review\ndescription: Review code.\n---\nReview.\n")
	runSyncOK(t)
	if fi, err := os.Lstat(".claude/skills/review"); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Skip("no shared-skill link on this platform")
	}
	mustWriteFile(t, ".agents/skills/review/SKILL.md", readFile(t, ".agents/skills/review/SKILL.md")+"my local tweak\n")
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", "---\nname: review\ndescription: Review code.\n---\nReview twice.\n")
	runSyncOK(t)

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}

	if _, err := os.Stat(".agnostic-ai/skills/review/SKILL.md.bak"); err == nil {
		t.Error("import copied the backup behind the link into the skill")
	}
}
