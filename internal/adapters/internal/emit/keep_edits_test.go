package emit

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
)

func TestKeepEdits_SkipsFileEditedSinceItsRecordedSum(t *testing.T) {
	dir := t.TempDir()
	edited := filepath.Join(dir, "AGENTS.md")
	unedited := filepath.Join(dir, "CLAUDE.md")
	unrecorded := filepath.Join(dir, "GEMINI.md")
	for _, p := range []string{edited, unedited, unrecorded} {
		if err := os.WriteFile(p, []byte("synced\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(edited, []byte("hand edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sess := NewSession()
	sess.KeepEditsSince(map[string]string{edited: ContentSum("synced\n"), unedited: ContentSum("synced\n")})
	sess.StartDetailedRecording()
	for _, p := range []string{edited, unedited, unrecorded} {
		if err := sess.WriteFile(p, "new\n", false); err != nil {
			t.Fatal(err)
		}
	}
	writes := sess.StopDetailedRecording()

	if got, _ := os.ReadFile(edited); string(got) != "hand edit\n" {
		t.Errorf("edited file = %q, want the hand edit kept", got)
	}
	for _, p := range []string{unedited, unrecorded} {
		if got, _ := os.ReadFile(p); string(got) != "new\n" {
			t.Errorf("%s = %q, want it rewritten", filepath.Base(p), got)
		}
	}
	if len(writes) != 3 || writes[0].Action != "edited" || writes[0].Sum != ContentSum("synced\n") {
		t.Errorf("writes = %+v, want the edited file recorded as edited with its prior sum", writes)
	}
	if kept := sess.KeptEdits(); len(kept) != 1 || kept[0] != edited {
		t.Errorf("KeptEdits() = %v, want [%s]", kept, edited)
	}
}

func TestKeepEdits_OffByDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	if err := os.WriteFile(path, []byte("hand edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess := NewSession()
	if err := sess.WriteFile(path, "new\n", false); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new\n" {
		t.Errorf("file = %q, want it rewritten", got)
	}
	if sess.KeepsEdits() {
		t.Error("KeepsEdits() = true on a new session")
	}
}

func TestKeepEdits_RecordsEachKeptEditInOrder(t *testing.T) {
	for _, detailing := range []bool{true, false} {
		t.Run(map[bool]string{true: "detailed", false: "summary only"}[detailing], func(t *testing.T) {
			// The temp root sits under a linked folder on macOS, which
			// would keep every removal for that reason alone.
			dir, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			generated := "<!-- " + header.Marker + " -->\nhand edit\n"
			put := func(name, content string) string {
				t.Helper()
				p := filepath.Join(dir, name)
				if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
				return p
			}
			keptWrite := put("kept-write.md", "hand edit\n")
			keptRemoval := put("kept-removal.md", generated)
			linked := filepath.Join(dir, "linked.md")
			if err := os.Symlink(put("link-target.md", generated), linked); err != nil {
				t.Fatal(err)
			}
			blockedRemoval := put("blocked-removal.md", generated)
			put("blocked-removal.md.bak", "older backup\n")
			blockedWrite := put("blocked-write.md", "hand edit\n")
			put("blocked-write.md.bak", "older backup\n")
			writeSum := ContentSum("synced write\n")
			removalSum := ContentSum("synced removal\n")
			orphanSum := ContentSum("synced orphan\n")
			backupSum := ContentSum("synced backup\n")

			sess := NewSession()
			sess.KeepEditsSince(map[string]string{keptWrite: writeSum, keptRemoval: removalSum})
			sess.BackUpEditsSince(map[string]string{blockedWrite: backupSum})
			if detailing {
				sess.StartDetailedRecording()
			}
			if err := sess.WriteFile(keptWrite, "new\n", false); err != nil {
				t.Fatal(err)
			}
			for _, p := range []string{keptRemoval, linked, blockedRemoval} {
				if removed, err := sess.RemoveOwnedOrphan(p, orphanSum); err != nil || removed {
					t.Fatalf("RemoveOwnedOrphan(%s) = %v, %v, want the edit kept", filepath.Base(p), removed, err)
				}
			}
			if err := sess.WriteFile(blockedWrite, "newer\n", false); err != nil {
				t.Fatal(err)
			}
			writes := sess.StopDetailedRecording()

			var want []WrittenFile
			if detailing {
				want = []WrittenFile{
					{Path: keptWrite, Bytes: len("new\n"), Action: "edited", Sum: writeSum},
					{Path: keptRemoval, Action: "edited", Sum: removalSum},
					{Path: linked, Action: "edited", Sum: orphanSum},
					{Path: blockedRemoval, Action: "edited", Sum: orphanSum},
					{Path: blockedWrite, Bytes: len("newer\n"), Action: "edited", Sum: backupSum},
				}
			}
			if !reflect.DeepEqual(writes, want) {
				t.Errorf("writes = %+v, want %+v", writes, want)
			}
			if kept, want := sess.KeptEdits(), []string{keptWrite, keptRemoval, linked}; !slices.Equal(kept, want) {
				t.Errorf("KeptEdits() = %v, want %v", kept, want)
			}
			if blocked, want := sess.BackupBlockedEdits(), []string{blockedRemoval, blockedWrite}; !slices.Equal(blocked, want) {
				t.Errorf("BackupBlockedEdits() = %v, want %v", blocked, want)
			}
		})
	}
}
