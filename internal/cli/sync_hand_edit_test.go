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
	if want := "overwrote a hand edit to " + handEditSkill + " (saved as " + handEditSkill + ".bak); move the edit into .agnostic-ai/\n"; !strings.Contains(log.String(), want) {
		t.Errorf("sync output does not say %q:\n%s", want, log.String())
	}
}

// Gemini CLI saves team facts to GEMINI.md, which sync writes over, so
// the warning for an entry point also names shared memory (#2103).
func TestSync_OverwriteWarningForAnEntryPointNamesSharedMemory(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [gemini]\nbuiltins: [memory]\n")
	runSyncOK(t)
	skill := ".gemini/skills/shared-memory/SKILL.md"
	for _, path := range []string{"GEMINI.md", skill} {
		mustWriteFile(t, path, readFile(t, path)+"The API listens on port 8080.\n")
	}
	log := captureLog(t)

	runSyncOK(t)

	for path, advice := range map[string]string{
		"GEMINI.md": "move the edit into .agnostic-ai/, or save a fact in .agnostic-ai/memory/\n",
		skill:       "move the edit into .agnostic-ai/\n",
	} {
		if want := "overwrote a hand edit to " + path + " (saved as " + path + ".bak); " + advice; !strings.Contains(log.String(), want) {
			t.Errorf("sync output does not say %q:\n%s", want, log.String())
		}
	}
}

// Without the memory built-in no tool reads .agnostic-ai/memory/.
func TestSync_OverwriteWarningForAnEntryPointWithoutSharedMemoryKeepsItsText(t *testing.T) {
	handEditProject(t, "gemini")
	mustWriteFile(t, "GEMINI.md", readFile(t, "GEMINI.md")+"my local tweak\n")
	log := captureLog(t)

	runSyncOK(t)

	if want := "overwrote a hand edit to GEMINI.md (saved as GEMINI.md.bak); move the edit into .agnostic-ai/\n"; !strings.Contains(log.String(), want) {
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
		if filepath.ToSlash(w.Path) == handEditSkill && w.Backup == handEditSkill+".bak" {
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

// Another skill's asset with the same name and bytes as a sync backup is
// the user's, so import keeps it.
func TestImport_KeepsAnUnrelatedCopyOfASyncBackup(t *testing.T) {
	handEditProject(t, "claude")
	mustWriteFile(t, handEditSkill, readFile(t, handEditSkill)+"my local tweak\n")
	runSyncOK(t)
	mustWriteFile(t, ".claude/skills/other/SKILL.md", "---\nname: other\ndescription: Other.\n---\nOther.\n")
	mustWriteFile(t, ".claude/skills/other/SKILL.md.bak", readFile(t, handEditSkill+".bak"))

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}

	if _, err := os.Stat(".agnostic-ai/skills/other/SKILL.md.bak"); err != nil {
		t.Errorf("import dropped an unrelated asset: %v", err)
	}
	if _, err := os.Stat(".agnostic-ai/skills/review/SKILL.md.bak"); err == nil {
		t.Error("import copied the sync backup")
	}
}

func TestSync_BacksUpEditedOrphanWhenSpecIsRemoved(t *testing.T) {
	handEditProject(t, "claude")
	edited := readFile(t, handEditSkill) + "my local tweak\n"
	mustWriteFile(t, handEditSkill, edited)
	if err := os.Remove(".agnostic-ai/skills/review/SKILL.md"); err != nil {
		t.Fatal(err)
	}
	log := captureLog(t)
	runSyncOK(t)
	if got, err := os.ReadFile(handEditSkill + ".bak"); err != nil || string(got) != edited {
		t.Fatalf("backup = %q, %v; want edit", got, err)
	}
	if _, err := os.Stat(handEditSkill); !os.IsNotExist(err) {
		t.Errorf("orphan still exists: %v", err)
	}
	if !strings.Contains(log.String(), "removed a hand edit to "+handEditSkill) {
		t.Errorf("missing removal warning: %s", log.String())
	}
	if out, err := runCLI(t, "revert"); err != nil {
		t.Fatalf("revert: %v\n%s", err, out)
	}
	if got := readFile(t, handEditSkill); got != edited {
		t.Errorf("restored = %q, want edit", got)
	}
}

func TestSync_BacksUpEditedOrphanWhenTargetIsDropped(t *testing.T) {
	handEditProject(t, "claude, gemini")
	edited := readFile(t, handEditSkill) + "my local tweak\n"
	mustWriteFile(t, handEditSkill, edited)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [gemini]\n")
	runSyncOK(t)
	if got, err := os.ReadFile(handEditSkill + ".bak"); err != nil || string(got) != edited {
		t.Fatalf("backup = %q, %v; want edit", got, err)
	}
	if out, err := runCLI(t, "revert"); err != nil {
		t.Fatalf("revert: %v\n%s", err, out)
	}
	if got := readFile(t, handEditSkill); got != edited {
		t.Errorf("restored = %q, want edit", got)
	}
}

func TestSync_OrphanBackupConflictsAndKeepEdits(t *testing.T) {
	for _, tc := range []struct {
		name         string
		backup, keep bool
	}{
		{"existing backup", true, false}, {"keep edits", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handEditProject(t, "claude")
			edited := readFile(t, handEditSkill) + "edit\n"
			mustWriteFile(t, handEditSkill, edited)
			if tc.backup {
				mustWriteFile(t, handEditSkill+".bak", "earlier\n")
			}
			if err := os.Remove(".agnostic-ai/skills/review/SKILL.md"); err != nil {
				t.Fatal(err)
			}
			log := captureLog(t)
			if tc.keep {
				runSyncOK(t, "--keep-edits")
			} else {
				runSyncOK(t)
			}
			if got := readFile(t, handEditSkill); got != edited {
				t.Errorf("kept = %q", got)
			}
			if tc.backup {
				if got := readFile(t, handEditSkill+".bak"); got != "earlier\n" {
					t.Errorf("backup = %q", got)
				}
				if !strings.Contains(log.String(), "already holds an earlier one") {
					t.Errorf("missing conflict warning: %s", log.String())
				}
			} else if _, err := os.Lstat(handEditSkill + ".bak"); !os.IsNotExist(err) {
				t.Errorf("unexpected backup: %v", err)
			}
		})
	}
}

func TestSync_UneditedOrphanIsRemovedWithoutBackup(t *testing.T) {
	handEditProject(t, "claude")
	if err := os.Remove(".agnostic-ai/skills/review/SKILL.md"); err != nil {
		t.Fatal(err)
	}
	runSyncOK(t)
	for _, p := range []string{handEditSkill, handEditSkill + ".bak"} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("%s remains: %v", p, err)
		}
	}
}

func TestSyncJSON_NamesBackupOfRemovedEdit(t *testing.T) {
	handEditProject(t, "claude")
	mustWriteFile(t, handEditSkill, readFile(t, handEditSkill)+"edit\n")
	if err := os.Remove(".agnostic-ai/skills/review/SKILL.md"); err != nil {
		t.Fatal(err)
	}
	out, err := runCLI(t, "sync", "--json")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	var got jsonOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	for _, w := range got.Writes {
		if w.Path == handEditSkill && w.Action == "delete" && w.Backup == handEditSkill+".bak" {
			return
		}
	}
	t.Errorf("missing removed edit backup: %s", out)
}

func TestSync_KeepsEditedOrphanWhenBackupIsASymlink(t *testing.T) {
	handEditProject(t, "claude")
	edited := readFile(t, handEditSkill) + "edit\n"
	mustWriteFile(t, handEditSkill, edited)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.Symlink(outside, handEditSkill+".bak"); err != nil {
		t.Skip(err)
	}
	if err := os.Remove(".agnostic-ai/skills/review/SKILL.md"); err != nil {
		t.Fatal(err)
	}
	runSyncOK(t)
	if got := readFile(t, handEditSkill); got != edited {
		t.Errorf("kept = %q", got)
	}
	if _, err := os.Lstat(outside); !os.IsNotExist(err) {
		t.Errorf("wrote through backup link: %v", err)
	}
}

func TestSync_ProtectsEditedCodexConfigWhenLastMCPIsRemoved(t *testing.T) {
	for _, tc := range []struct {
		name           string
		args           []string
		existing, keep bool
	}{
		{"text", nil, false, false}, {"json", []string{"--json"}, false, false},
		{"backup conflict", nil, true, false}, {"keep edits", []string{"--keep-edits"}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
			mustWriteFile(t, ".agnostic-ai/mcps/local.yaml", "name: local\ncommand: server\n")
			runSyncOK(t)
			const path = ".codex/config.toml"
			edited := readFile(t, path) + "# local edit\n"
			mustWriteFile(t, path, edited)
			if tc.existing {
				mustWriteFile(t, path+".bak", "earlier\n")
			}
			if err := os.Remove(".agnostic-ai/mcps/local.yaml"); err != nil {
				t.Fatal(err)
			}
			log := captureLog(t)
			out, err := runCLI(t, append([]string{"sync"}, tc.args...)...)
			if err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			if tc.existing || tc.keep {
				if got := readFile(t, path); got != edited {
					t.Errorf("edit = %q", got)
				}
				if tc.existing && readFile(t, path+".bak") != "earlier\n" {
					t.Error("earlier backup changed")
				}
				return
			}
			if got, err := os.ReadFile(path + ".bak"); err != nil || string(got) != edited {
				t.Fatalf("backup = %q, %v", got, err)
			}
			if tc.name == "json" {
				var result jsonOutput
				if err := json.Unmarshal([]byte(out), &result); err != nil {
					t.Fatal(err)
				}
				found := false
				for _, w := range result.Writes {
					if w.Path == path && w.Action == "delete" && w.Backup == path+".bak" {
						found = true
					}
				}
				if !found {
					t.Errorf("missing backup in JSON: %s", out)
				}
			} else if !strings.Contains(log.String(), "removed a hand edit to "+path) {
				t.Errorf("missing warning: %s", log.String())
			}
			if out, err := runCLI(t, "revert"); err != nil {
				t.Fatalf("revert: %v %s", err, out)
			}
			if got := readFile(t, path); got != edited {
				t.Error("revert did not restore edited bytes")
			}
		})
	}
}

func TestRevert_RestoresRemovedEditWithItsMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits")
	}
	for _, tc := range []struct {
		name string
		mode os.FileMode
		args []string
	}{
		{"private text", 0o600, nil}, {"executable json", 0o755, []string{"--json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handEditProject(t, "claude")
			edited := readFile(t, handEditSkill) + "edit\n"
			mustWriteFile(t, handEditSkill, edited)
			if err := os.Chmod(handEditSkill, tc.mode); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(".agnostic-ai/skills/review/SKILL.md"); err != nil {
				t.Fatal(err)
			}
			runSyncOK(t)
			if out, err := runCLI(t, append([]string{"revert"}, tc.args...)...); err != nil {
				t.Fatalf("revert: %v %s", err, out)
			}
			info, err := os.Stat(handEditSkill)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != tc.mode {
				t.Errorf("mode = %o, want %o", info.Mode().Perm(), tc.mode)
			}
			if readFile(t, handEditSkill) != edited {
				t.Error("restored bytes changed")
			}
		})
	}
}
