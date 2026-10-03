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
		".warp/.mcp.json":    "x-chroma-token: ${X_CHROMA_TOKEN}",
		".zed/settings.json": "x-chroma-token: ${X_CHROMA_TOKEN}",
		".codex/config.toml": "x-chroma-token: ${X_CHROMA_TOKEN}",
		".mcp.json":          "x-chroma-token: ${X_CHROMA_TOKEN}",
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

// OpenCode reads `{env:NAME}`, so its native `$${env:X}` is text. Import
// keeps it as text, with no masking byte, and sync writes it back.
func TestImportMCP_OpenCodeEscapedToolFormStaysText(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	captureLog(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [opencode, claude]\n")
	native := `{"mcp": {"srv": {"type": "local", "command": ["srv", "--t", "$${env:X}"]}}}`
	writeFile(t, filepath.Join(dir, "opencode.json"), native)

	execCLI(t, "import", "opencode")

	got := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "srv.yaml"))
	if strings.ContainsRune(got, 0) || !strings.Contains(got, "$$${env:X}") {
		t.Fatalf("spec should hold $$${env:X} and no NUL:\n%q", got)
	}
	execCLI(t, "sync")
	emitted := snapshotEmitted(t, dir)
	for _, file := range []string{"opencode.json", ".mcp.json"} {
		if strings.ContainsRune(emitted[file], 0) || !strings.Contains(emitted[file], "$${env:X}") {
			t.Errorf("%s should hold $${env:X} and no NUL:\n%q", file, emitted[file])
		}
	}
	if out, err := runCLI(t, "lint"); err != nil || strings.Contains(out, "LINT028") {
		t.Errorf("an escaped tool form is not LINT028: %v\n%s", err, out)
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after import: %v\n%s", err, out)
	}
}

// Gemini reads `$${X}` as `$` plus the value of X and documents no
// escape. Import keeps those bytes, so each import and sync cycle writes
// the same file.
func TestImportMCP_GeminiNativeEscapeRoundTrips(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	captureLog(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [gemini]\n")
	writeFile(t, filepath.Join(dir, ".gemini", "settings.json"), `{"mcpServers": {"srv": {"command": "srv", "args": ["--price", "$${PRICE}", "${TOKEN}"]}}}`)

	for i := range 3 {
		execCLI(t, "import", "gemini")
		execCLI(t, "sync")
		settings := snapshotEmitted(t, dir)[".gemini/settings.json"]
		if !strings.Contains(settings, `"$${PRICE}"`) || !strings.Contains(settings, `"${TOKEN}"`) {
			t.Fatalf("cycle %d changed the args:\n%s", i+1, settings)
		}
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after import: %v\n%s", err, out)
	}
}

func TestNormalizeImportedMCP_GeminiHTTPURLKeepsNativeEscape(t *testing.T) {
	servers := map[string]any{"api": map[string]any{"httpUrl": "https://example.com/$${TENANT}/mcp"}}
	normalizeImportedMCP("gemini", servers)
	server := servers["api"].(map[string]any)
	if server["type"] != "http" || server["url"] != "https://example.com/$$${TENANT}/mcp" {
		t.Errorf("httpUrl should become an escaped url, got %v", server)
	}
}
