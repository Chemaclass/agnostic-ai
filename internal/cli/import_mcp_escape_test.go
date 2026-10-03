package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const warpChromaMCP = `{"mcpServers": {"chroma": {"command": "npx", "args": ["-y", "mcp-remote", "https://mcp.example.com", "--header", "x-chroma-token: ${X_CHROMA_TOKEN}"], "env": {"X_CHROMA_TOKEN": "${X_CHROMA_TOKEN}"}}}}`

// Warp expands nothing in MCP args, so `${X_CHROMA_TOKEN}` there is text
// for mcp-remote to expand. Import keeps it as the `$${...}` escape, and
// sync writes the same text back to every tool.
func TestImportMCP_WarpLiteralPlaceholderRoundTrips(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	captureLog(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [warp, zed, codex, claude]\n")
	writeFile(t, filepath.Join(dir, ".warp", ".mcp.json"), warpChromaMCP)

	execCLI(t, "import", "warp")

	got := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "chroma.yaml"))
	for _, want := range []string{"x-chroma-token: $${X_CHROMA_TOKEN}", "X_CHROMA_TOKEN: $${X_CHROMA_TOKEN}"} {
		if !strings.Contains(got, want) {
			t.Errorf("spec lacks %q:\n%s", want, got)
		}
	}

	execCLI(t, "sync")
	emitted := snapshotEmitted(t, dir)
	for file, want := range map[string]string{
		filepath.Join(".warp", ".mcp.json"):    "x-chroma-token: ${X_CHROMA_TOKEN}",
		filepath.Join(".zed", "settings.json"): "x-chroma-token: ${X_CHROMA_TOKEN}",
		filepath.Join(".codex", "config.toml"): "x-chroma-token: ${X_CHROMA_TOKEN}",
		".mcp.json":                            "x-chroma-token: ${X_CHROMA_TOKEN}",
	} {
		if !strings.Contains(emitted[file], want) || strings.Contains(emitted[file], "$${") {
			t.Errorf("%s should hold %q and no escape:\n%s", file, want, emitted[file])
		}
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after import: %v\n%s", err, out)
	}
}

// Codex expands nothing inline: a `${NAME}` in its args or env is text,
// while `env_vars` still forwards a variable as a reference.
func TestImportMCP_CodexKeepsLiteralsApartFromForwardedVariables(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	captureLog(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	writeFile(t, filepath.Join(dir, ".codex", "config.toml"), "[mcp_servers.chroma]\ncommand = \"npx\"\nargs = [\"mcp-remote\", \"--header\", \"x-token: ${X_TOKEN}\"]\nenv_vars = [\"API_KEY\"]\n\n[mcp_servers.chroma.env]\nLABEL = \"${LABEL}\"\n")

	execCLI(t, "import", "codex")

	got := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "chroma.yaml"))
	for _, want := range []string{"x-token: $${X_TOKEN}", "LABEL: $${LABEL}", "API_KEY: ${API_KEY}"} {
		if !strings.Contains(got, want) {
			t.Errorf("spec lacks %q:\n%s", want, got)
		}
	}
	execCLI(t, "sync")
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after import: %v\n%s", err, out)
	}
}

// A default is text that may be a secret: import still replaces the
// value with a reference instead of committing it.
func TestImportMCP_EscapedDefaultIsNotCommitted(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	captureLog(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [warp]\n")
	writeFile(t, filepath.Join(dir, ".warp", ".mcp.json"), `{"mcpServers": {"srv": {"command": "srv", "env": {"TOKEN": "${TOKEN:-sk-real}"}}}}`)

	execCLI(t, "import", "warp")

	if got := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "srv.yaml")); strings.Contains(got, "sk-real") {
		t.Errorf("the default must not reach the spec:\n%s", got)
	}
}
