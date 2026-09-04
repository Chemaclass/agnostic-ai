package crush

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestName(t *testing.T) {
	if got := New().Name(); got != "crush" {
		t.Errorf("Name() = %q, want %q", got, "crush")
	}
}

// The project-root AGENTS.md is written centrally by sync, never by
// this adapter: Crush has no per-rule or per-agent surface of its own.
func TestEmit_NoRootAGENTSMd_ByDefault(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write AGENTS.md, err=%v", err)
	}
}

// Skills emit natively as one folder per skill under .agents/skills/,
// the shared cross-tool tree Crush scans first.
func TestEmit_Skill_WritesSharedSkillFolder(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindSkill,
			Name: "yaml-validator",
			Meta: map[string]any{"description": "Validate YAML."},
			Body: "Validate against schema.",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/skills/yaml-validator/SKILL.md"))
	for _, want := range []string{"name: yaml-validator", "description: Validate YAML.", "Validate against schema."} {
		if !strings.Contains(got, want) {
			t.Errorf("SKILL.md missing %q:\n%s", want, got)
		}
	}
}

// crush README documents `user-invocable: true` skill frontmatter to add
// the skill to the command palette (ctrl+p). It reaches crush's SKILL.md
// through the generic x-crush passthrough the shared renderer already
// honors, so no crush-specific code path is needed. See #540.
func TestEmit_Skill_UserInvocablePassesThroughUnderXCrush(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindSkill,
			Name: "yaml-validator",
			Meta: map[string]any{
				"description": "Validate YAML.",
				"x-crush":     map[string]any{"user-invocable": true},
			},
			Body: "Validate against schema.",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/skills/yaml-validator/SKILL.md"))
	if !strings.Contains(got, "user-invocable: true") {
		t.Errorf("SKILL.md missing user-invocable: true:\n%s", got)
	}
}

func TestEmit_Skill_SkillsDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"crush": {SkillsDir: "custom/skills"}},
	}
	entries := []spec.Entry{{Kind: spec.KindSkill, Name: "yaml-validator", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/skills/yaml-validator/SKILL.md")); err != nil {
		t.Errorf("expected custom/skills/yaml-validator/SKILL.md: %v", err)
	}
}

// Stdio MCP emits to crush.json under mcp.<name> with type "stdio".
func TestEmit_MCP_StdioWritesCrushJSON(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command": "npx",
				"args":    []any{"-y", "@modelcontextprotocol/server-filesystem", "."},
				"env":     map[string]any{"ALLOWED_PATHS": "."},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "crush.json"))
	for _, want := range []string{
		`"mcp"`,
		`"fs"`,
		`"type": "stdio"`,
		`"command": "npx"`,
		`"ALLOWED_PATHS"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestEmit_MCP_HTTPWritesURL(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "linear",
			Meta: map[string]any{
				"type":    "http",
				"url":     "https://mcp.linear.app",
				"headers": map[string]any{"Authorization": "Bearer x"},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "crush.json"))
	for _, want := range []string{
		`"linear"`,
		`"type": "http"`,
		`"url": "https://mcp.linear.app"`,
		`"Authorization"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// crush's own config.go declares MCPSSE and MCPHttp as distinct enum
// values, and createTransport in internal/agent/tools/mcp/init.go routes
// them to two different SDK transports (SSEClientTransport vs
// StreamableClientTransport). A `type: sse` spec entry must therefore
// emit `"type": "sse"`, not collapse into "http": an SSE-only server
// does not speak Streamable HTTP. See #586.
func TestEmit_MCP_SSEWritesTypeSSE(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "events",
			Meta: map[string]any{
				"type": "sse",
				"url":  "https://mcp.example.com/sse",
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "crush.json"))
	for _, want := range []string{
		`"events"`,
		`"type": "sse"`,
		`"url": "https://mcp.example.com/sse"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, `"type": "http"`) {
		t.Errorf("sse entry must not emit type http: %s", got)
	}
}

// Crush's own MCPType enum has no "remote" value; a spec's `type: remote`
// has no native Crush transport to map to, so it keeps defaulting to
// http rather than being dropped.
func TestEmit_MCP_RemoteDefaultsToHTTPType(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "remote-thing",
			Meta: map[string]any{
				"type": "remote",
				"url":  "https://mcp.example.com",
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "crush.json"))
	if !strings.Contains(got, `"type": "http"`) {
		t.Errorf("remote entry should default to type http: %s", got)
	}
}

// crush README documents oauth/oauth_client_id/oauth_client_secret/
// oauth_callback_port on http/sse entries (v0.87.0, "MCP OAuth
// implementation"). See #531.
func TestEmit_MCP_HTTPWritesOAuthFields(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "linear",
			Meta: map[string]any{
				"type":                "http",
				"url":                 "https://mcp.linear.app",
				"oauth":               true,
				"oauth_client_id":     "abc123",
				"oauth_client_secret": "shh",
				"oauth_callback_port": 8080,
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "crush.json"))
	for _, want := range []string{
		`"oauth": true`,
		`"oauth_client_id": "abc123"`,
		`"oauth_client_secret": "shh"`,
		`"oauth_callback_port": 8080`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// oauth is only meaningful for http/sse entries; a stdio entry must not
// pick up an oauth block accidentally.
func TestEmit_MCP_StdioIgnoresOAuthFields(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command": "npx",
				"oauth":   true,
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "crush.json"))
	if strings.Contains(got, "oauth") {
		t.Errorf("stdio entry should not emit oauth fields: %s", got)
	}
}

// disabled is Crush's own documented key ("Whether this MCP server is
// disabled", schema.json `$defs.MCPConfig.properties.disabled`) and was
// dropped silently until #641. That drop was worse than a warning: one
// spec synced to crush and trae printed a note for trae and nothing for
// crush, so the silence read as success.
func TestEmit_MCP_DisabledEmits(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{"command": "npx", "disabled": true},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "crush.json"))
	if !strings.Contains(got, `"disabled": true`) {
		t.Errorf("expected disabled to reach crush.json: %s", got)
	}
}

// sessionless, enabled_tools and disabled_tools are properties of
// MCPConfig itself rather than of a transport variant, so each emits on
// whichever transport the spec declares (#634). Explicit field mapping,
// not a generic x-crush merge: MCPConfig sets
// `"additionalProperties": false`, so a typo would produce a config
// Crush rejects outright.
func TestEmit_MCP_PassesThroughSessionlessAndToolLists(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "github",
			Meta: map[string]any{
				"type": "http", "url": "https://api.githubcopilot.com/mcp/",
				"sessionless":   true,
				"enabled_tools": []any{"read_file", "list_issues"},
			},
		},
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command":        "npx",
				"disabled_tools": []any{"delete_file"},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "crush.json"))
	for _, want := range []string{
		`"sessionless": true`,
		`"enabled_tools"`, `"list_issues"`,
		`"disabled_tools"`, `"delete_file"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestEmit_MCP_PreservesExistingUserKeys(t *testing.T) {
	dir := testutil.TempCwd(t)

	existing := `{"models": {"large": "x"}, "providers": {"anthropic": {}}, "options": {"debug": true}}`
	if err := os.WriteFile(filepath.Join(dir, "crush.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "crush.json"))
	for _, want := range []string{
		`"large": "x"`,
		`"anthropic"`,
		`"debug": true`,
		`"mcp"`,
		`"fs"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestEmit_MCP_FileOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"crush": {MCPFile: "vendor/crush.json"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "x"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/crush.json")); err != nil {
		t.Errorf("expected override path written: %v", err)
	}
}

func TestEmit_NoCrushJSONWhenNoMCPs(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "x"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "crush.json")); !os.IsNotExist(err) {
		t.Errorf("expected no crush.json when no MCP entries, err=%v", err)
	}
}

// Stdio entries without a command are dropped: there is nothing for
// Crush to run.
func TestEmit_MCP_SkipsStdioWithoutCommand(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "bad", Meta: map[string]any{}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "crush.json")); !os.IsNotExist(err) {
		t.Errorf("expected no crush.json when entry has no command, err=%v", err)
	}
}

func TestEmit_EmptyBundle_WritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "crush.json")); !os.IsNotExist(err) {
		t.Errorf("expected no crush.json for empty bundle, err=%v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
