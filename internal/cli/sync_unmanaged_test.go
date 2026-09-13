package cli

import (
	"encoding/json"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const unmanagedAgentPath = ".claude/agents/x.md"

// setupUnmanagedFixture writes a claude-only project with two agents,
// one of them user-owned through sync.unmanaged and hand-edited on disk.
func setupUnmanagedFixture(t *testing.T) {
	t.Helper()
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"),
		"version: 1\ntargets: [claude]\nsync:\n  unmanaged:\n    - "+unmanagedAgentPath+"\n")
	for _, name := range []string{"x", "y"} {
		mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "agents", name+".md"),
			"---\nname: "+name+"\ndescription: Agent "+name+".\n---\n\nBody.\n")
	}
	mustWriteFile(t, filepath.Join(dir, unmanagedAgentPath), "hand-owned\n")
}

func TestRunSyncOnce_PrintsUnmanagedSkipLineOnce(t *testing.T) {
	setupUnmanagedFixture(t)
	silence(t)
	buf := captureLog(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	line := "  ~ skip (unmanaged) " + unmanagedAgentPath + "\n"
	if n := strings.Count(buf.String(), line); n != 1 {
		t.Errorf("want exactly one %q line, got %d in:\n%s", line, n, buf.String())
	}
	if got := readFile(t, unmanagedAgentPath); got != "hand-owned\n" {
		t.Errorf("user-owned agent rewritten: %q", got)
	}
}

func TestRunSyncOnce_UnmanagedPathAbsentFromLedger(t *testing.T) {
	setupUnmanagedFixture(t)
	silence(t)
	captureLog(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	var outputs []string
	for _, p := range readStateFile(".").Outputs {
		outputs = append(outputs, filepath.ToSlash(p)) // ledger keeps OS separators
	}
	if slices.Contains(outputs, unmanagedAgentPath) {
		t.Errorf("user-owned path in the ledger: %v", outputs)
	}
	if !slices.Contains(outputs, ".claude/agents/y.md") {
		t.Errorf("managed agent missing from the ledger: %v", outputs)
	}
}

func TestRunSyncJSON_ReportsUnmanagedInSkipped(t *testing.T) {
	setupUnmanagedFixture(t)
	silence(t)
	buf := &strings.Builder{}
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--json", "--gitignore", "off"})
	root.SetOut(buf)
	root.SetErr(io.Discard)

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	var out jsonOutput
	if err := json.Unmarshal([]byte(buf.String()), &out); err != nil {
		t.Fatalf("decode %q: %v", buf.String(), err)
	}
	want := fileRecord{Target: "agnostic-ai", Path: unmanagedAgentPath, Action: "unmanaged"}
	if !slices.Contains(out.Skipped, want) {
		t.Errorf("skipped = %+v, want it to contain %+v", out.Skipped, want)
	}
	for _, w := range out.Writes {
		if w.Path == unmanagedAgentPath {
			t.Errorf("user-owned path listed as a write: %+v", w)
		}
	}
}
