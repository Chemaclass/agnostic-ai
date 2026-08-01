package openhands

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
	if got := New().Name(); got != "openhands" {
		t.Errorf("Name() = %q, want %q", got, "openhands")
	}
}

// The project-root AGENTS.md is written centrally by sync, never by
// this adapter: OpenHands has no per-rule or per-agent surface of its
// own.
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
// the shared cross-tool tree OpenHands scans.
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

func TestEmit_Skill_SkillsDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"openhands": {SkillsDir: "custom/skills"}},
	}
	entries := []spec.Entry{{Kind: spec.KindSkill, Name: "yaml-validator", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/skills/yaml-validator/SKILL.md")); err != nil {
		t.Errorf("expected custom/skills/yaml-validator/SKILL.md: %v", err)
	}
}

func TestEmit_EmptyBundle_WritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents")); !os.IsNotExist(err) {
		t.Errorf("expected no .agents dir for an empty bundle, err=%v", err)
	}
}

// Stdio MCP merges into ./config.toml under [[mcp.stdio_servers]]
// (target-audit 2026-08-01, MISSING: openhands MCP).
func TestEmit_MCP_StdioWritesStdioServersTable(t *testing.T) {
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
	got := readFile(t, filepath.Join(dir, "config.toml"))
	for _, want := range []string{
		"[mcp]", "[[mcp.stdio_servers]]", `name = "fs"`,
		`command = "npx"`, `args = ["-y", "@modelcontextprotocol/server-filesystem", "."]`,
		`env = { ALLOWED_PATHS = "." }`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// SSE and HTTP (shttp) MCP entries render as bare URL strings under
// sse_servers / shttp_servers, OpenHands' two documented forms for a
// remote server with no further config.
func TestEmit_MCP_SSEAndHTTPWriteURLArrays(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "events", Meta: map[string]any{"type": "sse", "url": "https://example.test/sse"}},
		{Kind: spec.KindMCP, Name: "search", Meta: map[string]any{"type": "http", "url": "https://example.test/mcp"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "config.toml"))
	for _, want := range []string{
		`sse_servers = ["https://example.test/sse"]`,
		`shttp_servers = ["https://example.test/mcp"]`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// sse_servers/shttp_servers must render before any
// [[mcp.stdio_servers]] block: TOML forbids reopening [mcp] for a
// direct key once a nested array-of-tables under it has been written.
func TestEmit_MCP_DirectKeysPrecedeStdioServersTables(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
		{Kind: spec.KindMCP, Name: "search", Meta: map[string]any{"type": "http", "url": "https://example.test/mcp"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "config.toml"))
	shttpIdx := strings.Index(got, "shttp_servers")
	stdioIdx := strings.Index(got, "[[mcp.stdio_servers]]")
	if shttpIdx < 0 || stdioIdx < 0 {
		t.Fatalf("expected both shttp_servers and [[mcp.stdio_servers]] in:\n%s", got)
	}
	if shttpIdx > stdioIdx {
		t.Errorf("shttp_servers must precede [[mcp.stdio_servers]], got:\n%s", got)
	}
}

// A transport OpenHands documents no array for (e.g. ws, confirmed
// elsewhere for Claude Code and Qoder) has no home in [mcp] and must
// not be guessed into one; it surfaces a coverage note instead of
// vanishing silently now that KindMCP is capability-declared.
func TestEmit_MCP_UnmappedTransportSurfacesCoverageNote(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "socket", Meta: map[string]any{"type": "ws", "url": "wss://example.test/ws"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "1 mcp") || !strings.Contains(buf.String(), "openhands") {
		t.Errorf("expected a coverage note naming the unmapped mcp entry, got: %s", buf.String())
	}
}

func TestEmit_MCP_FileOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"openhands": {MCPFile: "vendor/openhands-config.toml"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/openhands-config.toml")); err != nil {
		t.Errorf("expected override path written: %v", err)
	}
}

func TestEmit_NoConfigTOMLWhenNoMCPs(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{Kind: spec.KindRule, Name: "r1", Body: "x"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.toml")); !os.IsNotExist(err) {
		t.Errorf("expected no config.toml when no MCP entries, err=%v", err)
	}
}

// Stdio entries without a command, and sse/http entries without a
// url, are dropped: there is nothing for OpenHands to run or connect
// to (mirrors kilo and codex).
func TestEmit_MCP_SkipsEntriesMissingRequiredField(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "bad-stdio", Meta: map[string]any{}},
		{Kind: spec.KindMCP, Name: "bad-http", Meta: map[string]any{"type": "http"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.toml")); !os.IsNotExist(err) {
		t.Errorf("expected no config.toml when every entry is missing its required field, err=%v", err)
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
