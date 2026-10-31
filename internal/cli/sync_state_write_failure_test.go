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

func TestSync_ReportsStateWriteFailureAfterOutputs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions do not deny writes on Windows")
	}
	for _, jsonMode := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "json"}[jsonMode], func(t *testing.T) {
			dir := setupFixture(t)
			testutil.Chdir(t, dir)
			silence(t)
			if out, err := runCLI(t, "sync", "-t", "claude"); err != nil {
				t.Fatalf("initial sync: %v: %s", err, out)
			}
			ledger, err := os.ReadFile(stateFilePath("."))
			if err != nil {
				t.Fatal(err)
			}
			mustWriteFile(t, ".agnostic-ai/rules/r1.md", "---\nname: r1\n---\nchanged rule body")
			stateDir := filepath.Join(dir, ".agnostic-ai")
			if err := os.Chmod(stateDir, 0o555); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(stateDir, 0o755) })
			probe := filepath.Join(stateDir, "write-probe")
			if err := os.WriteFile(probe, nil, 0o600); err == nil {
				_ = os.Remove(probe)
				t.Skip("directory permissions do not deny writes to this user")
			}
			args := []string{"sync", "-t", "claude"}
			if jsonMode {
				args = append(args, "--json")
			}
			out, err := runCLI(t, args...)
			if err == nil || !strings.Contains(err.Error(), "sync wrote files but could not record them") {
				t.Errorf("sync error = %v; output = %s", err, out)
			}
			if !strings.Contains(readFile(t, ".claude/rules/r1.md"), "changed rule body") {
				t.Error("new output was not kept")
			}
			after, err := os.ReadFile(stateFilePath("."))
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(ledger) {
				t.Error("previous ledger changed")
			}
			if jsonMode {
				var result struct {
					Errors []errorRecord `json:"errors"`
				}
				if err := json.Unmarshal([]byte(out), &result); err != nil {
					t.Fatalf("invalid JSON: %v: %s", err, out)
				}
				if len(result.Errors) == 0 || !strings.Contains(result.Errors[0].Message, "sync wrote files but could not record them") {
					t.Errorf("JSON errors = %+v", result.Errors)
				}
			}
		})
	}
}
