package emit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestWritePluginManifests_KeepsUnownedAcrossModes(t *testing.T) {
	for _, mode := range []string{"write", "dry-run", "capture"} {
		t.Run(mode, func(t *testing.T) {
			testutil.TempCwd(t)
			ResetCoverageNotes()
			oldWarner, oldSum := Warner, PriorOutputSum
			var notes strings.Builder
			Warner, PriorOutputSum = &notes, nil
			t.Cleanup(func() { Warner, PriorOutputSum = oldWarner, oldSum; ResetCoverageNotes() })
			root := ".agents/plugins/team"
			path := filepath.Join(root, "plugin.json")
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			want := "{\n  \"name\": \"custom\", \"version\": \"2.0.0\"\n}\n"
			if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
				t.Fatal(err)
			}
			sess := NewSession()
			sess.StartRecording()
			if mode == "capture" {
				sess.StartCapture()
			}
			if err := WritePluginManifests(sess, []string{root}, mode == "dry-run"); err != nil {
				t.Fatal(err)
			}
			if got := sess.StopRecording(); len(got) != 0 {
				t.Errorf("claimed unowned manifest: %v", got)
			}
			if mode == "capture" {
				if got := sess.StopCapture(); len(got) != 0 {
					t.Errorf("captured unowned manifest: %v", got)
				}
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != want {
				t.Errorf("unowned manifest changed: %s", got)
			}
			FlushCoverageNotes()
			if !strings.Contains(notes.String(), "kept") || !strings.Contains(notes.String(), filepath.ToSlash(path)) {
				t.Errorf("missing preservation note: %s", &notes)
			}
		})
	}
}

func TestWritePluginManifests_UpdatesOwnedManifest(t *testing.T) {
	testutil.TempCwd(t)
	root := ".agents/plugins/team"
	path := filepath.Join(root, "plugin.json")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	old := `{"name":"team","version":"0.1.0"}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := PriorOutputSum
	PriorOutputSum = func(p string) string {
		if p == path {
			return ContentSum(old)
		}
		return ""
	}
	t.Cleanup(func() { PriorOutputSum = prev })
	if err := WritePluginManifests(NewSession(), []string{root}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"version": "1.0.0"`) {
		t.Errorf("owned manifest was not updated: %s", got)
	}
}
