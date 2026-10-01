package cli

import (
	"os"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// doctor --fix must not delete a merged file an older ledger lists
// without a key record: it may hold the user's keys.
func TestDoctorFix_LegacyLedgerKeepsMergedUserSettings(t *testing.T) {
	const settings = ".gemini/settings.json"
	syncedGeminiWithHook(t, userSettings)
	dropMergedRecords(t)
	removeSpecs(t, geminiFmtHookSpec)
	if _, err := runCLI(t, "doctor", "--fix"); err == nil {
		t.Error("doctor --fix passed with a file it could not release")
	}
	if got := readJSONMap(t, settings); got["userKey"] != "mine" {
		t.Errorf("user key lost: %#v", got)
	}
}

// A confirmed removal that empties a merged file keeps its bytes as
// .bak under --backup, as a whole-file removal does.
func TestDoctorFix_BackupKeepsConfirmedMergedRelease(t *testing.T) {
	const settings = ".gemini/settings.json"
	syncedGeminiWithHook(t, "")
	doc := readJSONMap(t, settings)
	doc["hooks"] = map[string]any{"Mine": []any{}}
	writeJSONFile(t, settings, doc)
	original, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	removeSpecs(t, geminiFmtHookSpec)
	runSyncOK(t)
	cfg, err := config.Load(".")
	if err != nil {
		t.Fatal(err)
	}
	reports := []driftReport{{Target: "agnostic-ai", Orphaned: []string{settings}}}
	if _, err := offerOrphanRemoval(cfg, reports, true, func(string) (bool, error) { return true, nil }); err != nil {
		t.Fatal(err)
	}
	if fileExists(settings) {
		t.Errorf("%s stayed after a confirmed release emptied it", settings)
	}
	backup, err := os.ReadFile(settings + ".bak")
	if err != nil || string(backup) != string(original) {
		t.Errorf("backup = %q, %v; want %q", backup, err, original)
	}
}

// doctor --fix fails while a merged leftover it could not release fully
// is still there, as sync --check does.
func TestDoctorFix_ReportsMergedLeftoverItKept(t *testing.T) {
	const settings = ".gemini/settings.json"
	for _, tc := range []struct {
		name string
		edit func(t *testing.T)
	}{
		{"edited value", func(t *testing.T) {
			doc := readJSONMap(t, settings)
			doc["hooks"] = map[string]any{"Mine": []any{}}
			writeJSONFile(t, settings, doc)
		}},
		{"malformed", func(t *testing.T) { mustWriteFile(t, settings, "{\"hooks\":") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			syncedGeminiWithHook(t, userSettings)
			tc.edit(t)
			removeSpecs(t, geminiFmtHookSpec)
			_, doctorErr := runCLI(t, "doctor", "--fix")
			_, checkErr := runCLI(t, "sync", "--check")
			if doctorErr == nil && checkErr != nil {
				t.Errorf("doctor --fix passed while sync --check fails: %v", checkErr)
			}
		})
	}
}
