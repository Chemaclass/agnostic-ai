package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Sync saves a hand edit to RULES.md as RULES.md.bak. An output there
// writes over that backup in the same run, so the edit is lost.
func TestSync_OutputOnAnotherOutputsBackup_Stops(t *testing.T) {
	for _, args := range [][]string{
		{"sync", "--gitignore=off", "--jobs=1"},
		{"sync", "--check"},
		{"sync", "--dry-run"},
		{"sync", "--gitignore=off", "--target", "codex"},
		{"sync", "--gitignore=off", "--target", "claude"},
		{"validate"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "example.md"), "---\nname: example\n---\nKeep changes small.\n")
			mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\noutputs:\n  codex:\n    rules-file: RULES.md\n")
			if out, err := runCLI(t, "sync", "--gitignore=off", "--jobs=1"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			data, _ := os.ReadFile(filepath.Join(dir, "RULES.md"))
			edited := string(data) + "\nUNIQUE USER NOTE\n"
			mustWriteFile(t, filepath.Join(dir, "RULES.md"), edited)
			if out, err := runCLI(t, "sync", "--gitignore=off", "--jobs=1"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			mustWriteFile(t, filepath.Join(dir, "RULES.md"), edited)
			mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex, claude]\noutputs:\n  codex:\n    rules-file: RULES.md\n  claude:\n    rules-file: ./RULES.md.bak\n")

			out, err := runCLI(t, args...)

			msg := out
			if err != nil {
				msg += err.Error()
			}
			if err == nil || !strings.Contains(msg, "RULES.md.bak") || !strings.Contains(msg, "outputs.claude.rules-file") || !strings.Contains(msg, "outputs.codex.rules-file") {
				t.Fatalf("%v must stop and name both outputs, got %v\n%s", args, err, out)
			}
			if got, _ := os.ReadFile(filepath.Join(dir, "RULES.md")); string(got) != edited {
				t.Errorf("RULES.md must keep the hand edit, got:\n%s", got)
			}
			if bak, _ := os.ReadFile(filepath.Join(dir, "RULES.md.bak")); string(bak) != edited {
				t.Errorf("RULES.md.bak must keep the saved edit, got:\n%s", bak)
			}
		})
	}
}

// A file named *.bak is fine when no other output sits at its base.
func TestSync_OutputNamedBakWithoutABase_Passes(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "example.md"), "---\nname: example\n---\nKeep changes small.\n")
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\noutputs:\n  codex:\n    rules-file: NOTES.md.bak\n")

	if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
}
