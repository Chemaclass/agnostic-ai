package emit

import (
	"os"
	"path/filepath"
	"testing"
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
