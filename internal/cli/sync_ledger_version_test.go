package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A release that does not know a ledger's format misreads its paths and
// would strip keys or delete files it believes it created.
func TestSync_RefusesALedgerFromANewerRelease(t *testing.T) {
	for _, args := range [][]string{
		{"sync", "--gitignore=off"},
		{"sync", "--gitignore=off", "--json"},
		{"sync", "--check"},
		{"sync", "--dry-run"},
		{"sync", "--dry-run", "--json"},
		{"sync", "--plan"},
		{"use", "codex"},
		{"doctor", "--fix"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [amp]\n")
			mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "gh.yaml"), "name: gh\ncommand: npx\nargs: [gh-mcp]\n")
			if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			data, err := os.ReadFile(stateFilePath("."))
			if err != nil {
				t.Fatal(err)
			}
			var raw map[string]any
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatal(err)
			}
			raw["version"] = syncStateVersion + 1
			newer, _ := json.Marshal(raw)
			mustWriteFile(t, stateFilePath("."), string(newer))
			mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")

			out, err := runCLI(t, args...)

			if err == nil || !strings.Contains(err.Error(), "newer agnostic-ai") || !strings.Contains(err.Error(), "upgrade") {
				t.Fatalf("%v must refuse the ledger and name the upgrade, got %v\n%s", args, err, out)
			}
			if got, _ := os.ReadFile(stateFilePath(".")); string(got) != string(newer) {
				t.Errorf("the ledger must stay as the newer release wrote it:\n%s", got)
			}
			if settings, _ := os.ReadFile(filepath.Join(dir, ".amp", "settings.json")); !strings.Contains(string(settings), "gh-mcp") {
				t.Errorf(".amp/settings.json must keep the gh server:\n%s", settings)
			}
		})
	}
}

func TestUpdateStateFile_RefusesALedgerFromANewerRelease(t *testing.T) {
	testutil.TempCwd(t)
	if err := replaceStateFile(".", syncStateFile{Version: syncStateVersion + 1}); err != nil {
		t.Fatal(err)
	}

	for name, update := range map[string]func() error{
		"updateStateFile":  func() error { return updateStateFile(".", func(*syncStateFile) {}) },
		"recordOutputSums": func() error { return recordOutputSums(".", map[string]string{"AGENTS.md": "x"}) },
		"readStateFileStrict": func() error {
			_, err := readStateFileStrict(".")
			return err
		},
	} {
		if err := update(); err == nil || !strings.Contains(err.Error(), "newer agnostic-ai") {
			t.Errorf("%s must refuse the ledger, got %v", name, err)
		}
	}
}

func TestReadLedger_RefusesANewerLedgerThatDoesNotDecode(t *testing.T) {
	testutil.TempCwd(t)
	mustWriteFile(t, stateFilePath("."), `{"version":99,"outputs":[{"path":"a"}]}`)

	if err := readLedger(".").refuseNewer(); err == nil || !strings.Contains(err.Error(), "newer agnostic-ai") {
		t.Errorf("a newer ledger must be refused even when a field does not decode, got %v", err)
	}
}

func TestSync_RefusesANewerLedgerBeforeWritingAnything(t *testing.T) {
	for _, args := range [][]string{{"sync", "--gitignore=off"}, {"sync", "--json"}, {"use", "codex"}} {
		for _, ledger := range []string{`{"version":99}`, `{"synced_at":0,"version":99}`} {
			t.Run(strings.Join(args, " ")+" "+ledger, func(t *testing.T) {
				dir := testutil.TempCwd(t)
				silence(t)
				mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [amp]\n")
				mustWriteFile(t, stateFilePath("."), ledger)
				mustWriteFile(t, filepath.Join(dir, ".gitignore"), "my-user-ignore\n")

				out, err := runCLI(t, args...)

				if err == nil || !strings.Contains(err.Error(), "newer agnostic-ai") {
					t.Fatalf("%v must refuse the ledger, got %v\n%s", args, err, out)
				}
				if got, _ := os.ReadFile(filepath.Join(dir, ".gitignore")); string(got) != "my-user-ignore\n" {
					t.Errorf(".gitignore must stay as it was:\n%s", got)
				}
				if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", ".command-lock")); err == nil {
					t.Error("the refusal must come before the command lock")
				}
				if got, _ := os.ReadFile(stateFilePath(".")); string(got) != ledger {
					t.Errorf("the ledger must stay as it was:\n%s", got)
				}
			})
		}
	}
}

func newerLedgerProject(t *testing.T) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	silence(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [amp]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\n---\nUse tabs.\n")
	if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	mustWriteFile(t, filepath.Join(dir, "AGENTS.md.bak"), "MINE\n")
	mustWriteFile(t, stateFilePath("."), `{"version":99,"synced_at":"2026-11-12T00:00:00Z","outputs":["AGENTS.md"],"backups":{"AGENTS.md.bak":"x"}}`)
	return dir
}

// Commands that write refuse a newer ledger.
func TestWritingCommands_RefuseALedgerFromANewerRelease(t *testing.T) {
	for _, args := range [][]string{{"revert"}, {"cleanup"}, {"import", "claude"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := newerLedgerProject(t)

			out, err := runCLI(t, args...)

			if err == nil || !strings.Contains(err.Error(), "newer agnostic-ai") {
				t.Fatalf("%v must refuse the ledger, got %v\n%s", args, err, out)
			}
			if got, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md.bak")); string(got) != "MINE\n" {
				t.Errorf("%v must leave AGENTS.md.bak alone, got %q", args, got)
			}
		})
	}
}

// Commands that only report run and say the ledger is newer.
func TestReadingCommands_WarnAboutALedgerFromANewerRelease(t *testing.T) {
	for _, args := range [][]string{{"status"}, {"status", "--json"}, {"why", "AGENTS.md"}, {"doctor"}, {"doctor", "--json"}, {"cleanup", "--dry-run"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			newerLedgerProject(t)
			var warned strings.Builder
			ledgerWarnOut = &warned
			t.Cleanup(func() { ledgerWarnOut = os.Stderr })

			out, err := runCLI(t, args...)

			if err != nil && strings.Contains(err.Error(), "newer agnostic-ai") {
				t.Fatalf("%v must not refuse a ledger it only reads, got %v\n%s", args, err, out)
			}
			if !strings.Contains(warned.String()+out, "newer agnostic-ai") {
				t.Errorf("%v must say a newer release wrote the ledger, got:\n%s%s", args, warned.String(), out)
			}
		})
	}
}
