package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func runDoctorMCP(t *testing.T, mcps map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets:\n  - claude\n")
	for name, body := range mcps {
		mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", name+".yaml"), body)
	}
	testutil.Chdir(t, dir)
	root := NewRootCmd("test")
	root.SetArgs([]string{"doctor", "mcp"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})
	if err := root.Execute(); err != nil {
		t.Fatalf("doctor mcp: %v", err)
	}
	return out.String()
}

func TestDoctorMCP_ListsUnsetEnvRefsByName(t *testing.T) {
	t.Setenv("AA_TEST_SET_TOKEN", "secret-value")
	got := runDoctorMCP(t, map[string]string{
		"remote": "name: remote\nurl: https://example.com/${AA_TEST_HOST}/mcp\n" +
			"headers:\n  Authorization: Bearer ${AA_TEST_SET_TOKEN}\n  X-Team: ${AA_TEST_TEAM}\n",
		"local": "name: local\ncommand: sh\nargs: [\"--dir\", \"${AA_TEST_DIR}\"]\nenv:\n  KEY: ${AA_TEST_KEY}\n",
	})
	for _, want := range []string{
		"MCP environment references:",
		"✗ remote reads AA_TEST_HOST, AA_TEST_TEAM, unset in this shell.",
		"✗ local reads AA_TEST_DIR, AA_TEST_KEY, unset in this shell.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "secret-value") || strings.Contains(got, "AA_TEST_SET_TOKEN") {
		t.Errorf("a set variable or its value was printed:\n%s", got)
	}
}

func TestDoctorMCP_PassesWhenEveryEnvRefIsSet(t *testing.T) {
	t.Setenv("AA_TEST_KEY", "x")
	got := runDoctorMCP(t, map[string]string{
		"local": "name: local\ncommand: sh\nenv:\n  KEY: ${AA_TEST_KEY}\n",
	})
	if !strings.Contains(got, "✓ local\n") {
		t.Errorf("expected a pass line for local:\n%s", got)
	}
}

func TestDoctorMCP_SkipsRefsThatCannotBeUnset(t *testing.T) {
	got := runDoctorMCP(t, map[string]string{
		"defaulted": "name: defaulted\ncommand: sh\nenv:\n  KEY: ${AA_TEST_KEY:-fallback}\n",
		"escaped":   "name: escaped\ncommand: sh\nargs: [\"$${AA_TEST_LITERAL}\"]\n",
		"editor":    "name: editor\ncommand: sh\nargs: [\"${workspaceFolder}\"]\n",
		"off":       "name: off\ncommand: sh\ndisabled: true\nenv:\n  KEY: ${AA_TEST_OFF}\n",
	})
	if strings.Contains(got, "MCP environment references:") {
		t.Errorf("expected no references block:\n%s", got)
	}
}
