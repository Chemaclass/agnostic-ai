package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportOpenhands_RefusesLiteralMCPCredential(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [openhands]\n")
	writeFile(t, filepath.Join(dir, ".agents/agents/web.md"), "---\nname: web\ndescription: w\nmcp_servers:\n  api:\n    command: npx\n    env:\n      API_KEY: sk-literal-123\n---\nBody\n")
	root := NewRootCmd("test")
	root.SetArgs([]string{"import", "openhands"})
	if err := root.Execute(); err == nil {
		t.Fatal("import kept a literal credential")
	}
	if exists(filepath.Join(dir, ".agnostic-ai/agents/web.md")) {
		t.Error("spec written with a literal credential")
	}
}

func TestImportOpenhands_RefusesLiteralMCPConfigCredential(t *testing.T) {
	_, err := openhandsNativeAgentFields([]byte("---\nname: web\ndescription: w\nmcp_config:\n  mcpServers:\n    api:\n      url: https://x.example\n      headers:\n        Authorization: Bearer abc123\n---\nBody\n"))
	if err == nil {
		t.Fatal("mcp_config kept a literal credential")
	}
}

func TestImportOpenhands_KeepsMCPEnvReferences(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [openhands]\n")
	writeFile(t, filepath.Join(dir, ".agents/agents/web.md"), "---\nname: web\ndescription: w\nmcp_servers:\n  api:\n    command: npx\n    env:\n      API_KEY: ${API_KEY}\n      OTHER: $OTHER\n---\nBody\n")
	execCLI(t, "import", "openhands")
	got := readFile(t, filepath.Join(dir, ".agnostic-ai/agents/web.md"))
	for _, want := range []string{"${API_KEY}", "$OTHER"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}
