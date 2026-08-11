package warp

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// docs.warp.dev/agents/capabilities/mcp documents `working_directory`
// ("Working directory path where the command is run, used for resolving
// relative paths") on the CLI Server (Command) table, and warns to
// "Always set `working_directory` explicitly when your MCP server
// command or args include relative paths." That is Warp's own name for
// the cross-tool spec's `cwd` field, which the shared MCP builder never
// read at all, so it reached `.warp/.mcp.json` for no target. See #606.
func TestEmit_MCP_StdioWritesWorkingDirectory(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command": "npx",
				"args":    []any{"-y", "@acme/mcp"},
				"cwd":     "./backend",
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".warp/.mcp.json"))
	if !strings.Contains(got, `"working_directory": "./backend"`) {
		t.Errorf("missing working_directory in %s", got)
	}
	if strings.Contains(got, `"cwd"`) {
		t.Errorf("cwd must be renamed to working_directory, not passed through verbatim:\n%s", got)
	}
}

// No cwd in the spec means no working_directory key, not an empty one.
func TestEmit_MCP_NoWorkingDirectoryWhenCwdUnset(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".warp/.mcp.json"))
	if strings.Contains(got, "working_directory") {
		t.Errorf("unexpected working_directory with no cwd set:\n%s", got)
	}
}

// working_directory is documented only on the CLI Server (Command) tab;
// a remote entry's `cwd` (meaningless for a server the user does not
// launch) must not leak the key either.
func TestEmit_MCP_RemoteServerIgnoresCwd(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "remote",
			Meta: map[string]any{"type": "http", "url": "https://example.test/mcp", "cwd": "./backend"},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".warp/.mcp.json"))
	if strings.Contains(got, "working_directory") || strings.Contains(got, `"cwd"`) {
		t.Errorf("remote entry must not emit a working directory:\n%s", got)
	}
}

// description, disabled, and roots predate this bespoke builder (they
// came from the shared emit.MCPSchemaServersMapNoType path this adapter
// used before #606); moving to a dedicated builder must not drop them.
func TestEmit_MCP_DescriptionDisabledAndRootsStillEmit(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command":     "npx",
				"description": "Filesystem access",
				"disabled":    true,
				"roots": []any{
					map[string]any{"uri": "file:///workspace", "name": "workspace"},
				},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".warp/.mcp.json"))
	for _, want := range []string{
		`"description": "Filesystem access"`,
		`"disabled": true`,
		`"uri": "file:///workspace"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}
