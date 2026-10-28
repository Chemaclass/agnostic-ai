package cli

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var importGlobalNextCommand = regexp.MustCompile("run `agnostic-ai (sync --global [^`]*)`")

func importClaudeUserFiles(t *testing.T, home string) {
	t.Helper()
	mustWriteGlobalTest(t, filepath.Join(home, ".claude.json"),
		`{"mcpServers":{"docs":{"type":"stdio","command":"docs-mcp","args":["--stdio"]}}}`+"\n")
	mustWriteGlobalTest(t, filepath.Join(home, ".claude", "settings.json"), `{"model":"opus"}`+"\n")
}

// printedGlobalCommand returns the sync arguments of the command import
// prints as its next step.
func printedGlobalCommand(t *testing.T, out string) []string {
	t.Helper()
	match := importGlobalNextCommand.FindStringSubmatch(strings.ToLower(out))
	if match == nil {
		t.Fatalf("no sync command in import output:\n%s", out)
	}
	return strings.Fields(strings.TrimPrefix(match[1], "sync --global"))
}

// The command import prints as its next step must pass on the user files
// it read from, before any sync has recorded ownership (#2647).
func TestImportGlobal_PrintedCheckPassesRightAfterImport(t *testing.T) {
	home, _ := globalAgentTestHome(t)
	importClaudeUserFiles(t, home)

	out, warnings, err := runImportGlobalTest("claude")
	if err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	if !strings.Contains(out, "next full `agnostic-ai sync --global` also writes the imported MCP servers") {
		t.Errorf("hint does not say the other tools get the imported specs:\n%s", out)
	}
	args := printedGlobalCommand(t, out)

	checkOut, checkWarnings, err := runGlobalAgentTest(args...)
	if err != nil {
		t.Fatalf("%v: %v\n%s%s", args, err, checkOut, checkWarnings)
	}
	if !strings.Contains(checkWarnings, "no ownership record at") {
		t.Errorf("check does not note the missing ownership record:\n%s", checkWarnings)
	}
	jsonOut, _, err := runGlobalAgentTest(append(args, "--json")...)
	if err != nil {
		t.Fatalf("%v --json: %v\n%s", args, err, jsonOut)
	}
	if !strings.Contains(jsonOut, "no ownership record at") {
		t.Errorf("json check does not note the missing ownership record:\n%s", jsonOut)
	}
}

// A home that synced before has an ownership record without the newly
// imported server; the printed check still passes (#2647).
func TestImportGlobal_PrintedCheckPassesInAHomeThatSynced(t *testing.T) {
	home, _ := globalAgentTestHome(t)
	importClaudeUserFiles(t, home)
	if _, warnings, err := runImportGlobalTest("claude"); err != nil {
		t.Fatalf("first import: %v\n%s", err, warnings)
	}
	if _, warnings, err := runGlobalAgentTest("--target", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	mustWriteGlobalTest(t, filepath.Join(home, ".claude.json"),
		`{"mcpServers":{"docs":{"type":"stdio","command":"docs-mcp","args":["--stdio"]},"web":{"type":"http","url":"https://example.com/mcp"}}}`+"\n")

	out, warnings, err := runImportGlobalTest("claude")
	if err != nil {
		t.Fatalf("second import: %v\n%s", err, warnings)
	}
	args := printedGlobalCommand(t, out)
	if checkOut, checkWarnings, err := runGlobalAgentTest(args...); err != nil {
		t.Fatalf("%v: %v\n%s%s", args, err, checkOut, checkWarnings)
	} else if !strings.Contains(checkWarnings, "is out of date") {
		t.Errorf("check does not note the outdated ownership record:\n%s", checkWarnings)
	}
}

// Sync writes each imported MCP server to every tool, so an import from several
// tools promises no passing check (#2647).
func TestImportGlobal_SeveralToolsPointAtADryRun(t *testing.T) {
	home, _ := globalAgentTestHome(t)
	importClaudeUserFiles(t, home)

	out, warnings, err := runImportGlobalTest()
	if err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	if strings.Contains(out, "--check") {
		t.Errorf("import from every tool promises a passing check:\n%s", out)
	}
	args := printedGlobalCommand(t, out)
	if dryOut, dryWarnings, err := runGlobalAgentTest(args...); err != nil {
		t.Fatalf("%v: %v\n%s%s", args, err, dryOut, dryWarnings)
	}
}

// A missing ownership record still drifts beside a tool file that does.
func TestSyncGlobalCheck_MissingStateDriftsBesideAChangedFile(t *testing.T) {
	home, source := globalAgentTestHome(t)
	importClaudeUserFiles(t, home)
	if _, warnings, err := runImportGlobalTest("claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	mustWriteGlobalTest(t, filepath.Join(home, ".claude", "settings.json"), `{"model":"sonnet"}`+"\n")
	statePath := filepath.ToSlash(filepath.Join(source, "state", "global.json"))

	out, _, err := runGlobalAgentTest("--target", "claude", "--check", "--diff")
	if err == nil || !strings.Contains(err.Error(), statePath) {
		t.Fatalf("check error = %v, want drift naming %s", err, statePath)
	}
	if !strings.Contains(out, "would create ownership state "+statePath) {
		t.Errorf("diff does not show the ownership state:\n%s", out)
	}
	jsonOut, _, err := runGlobalAgentTest("--target", "claude", "--check", "--json")
	if err == nil || !strings.Contains(jsonOut, statePath) {
		t.Errorf("json check error = %v, want a record for %s:\n%s", err, statePath, jsonOut)
	}
}

// A home `targets` list without the imported tool keeps sync away from
// it, so the hint says to add it before the check (#2647).
func TestImportGlobal_HintAsksToAddAnUnmanagedTool(t *testing.T) {
	home, source := globalAgentTestHome(t)
	importClaudeUserFiles(t, home)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")

	out, warnings, err := runImportGlobalTest("claude")
	if err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	if !strings.Contains(out, "add it to `targets`") {
		t.Errorf("hint does not ask to add claude to targets:\n%s", out)
	}
	args := printedGlobalCommand(t, out)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "version: 1\ntargets: [codex, claude]\n")
	if checkOut, checkWarnings, err := runGlobalAgentTest(args...); err != nil {
		t.Fatalf("%v: %v\n%s%s", args, err, checkOut, checkWarnings)
	}
}
