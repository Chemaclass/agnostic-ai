package kilo

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
	if got := New().Name(); got != "kilo" {
		t.Errorf("Name() = %q, want %q", got, "kilo")
	}
}

// The project-root AGENTS.md is written centrally by sync, never by
// this adapter: Kilo Code has no per-rule surface of its own.
func TestEmit_NoRootAGENTSMd_ByDefault(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write AGENTS.md, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".kilo")); !os.IsNotExist(err) {
		t.Errorf("a rule-only bundle should not create .kilo/, err=%v", err)
	}
}

// name and tools are confirmed no-ops on real Kilo Code (target-audit
// 2026-08-01, B2): the agent name comes from the filename, and the
// full agent option table has no tools key at all. A spec's tools
// allowlist must never look like it restricted the agent.
func TestEmit_Agent_WritesAgentFile(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent, Name: "reviewer",
			Meta: map[string]any{"description": "Reviews diffs.", "model": "sonnet", "tools": []any{"Read", "Grep"}},
			Body: "Review the diff for correctness.",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".kilo/agents/reviewer.md"))
	if !strings.HasPrefix(got, "---\n") {
		t.Fatalf("frontmatter must be first, got:\n%s", got)
	}
	for _, want := range []string{
		"description: Reviews diffs.",
		"model: sonnet",
		"Review the diff for correctness.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"name:", "tools:"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("Kilo Code has no %q frontmatter key; must never emit it, got:\n%s", unwanted, got)
		}
	}
}

func TestEmit_Agent_DescriptionFallsBackToName(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "no-desc", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".kilo/agents/no-desc.md"))
	if !strings.Contains(got, "description: no-desc") {
		t.Errorf("expected description fallback to agent name, got:\n%s", got)
	}
	if strings.Contains(got, "model:") || strings.Contains(got, "tools:") || strings.Contains(got, "name:") {
		t.Errorf("expected no name/model/tools keys when absent from meta, got:\n%s", got)
	}
}

// x-kilo cannot resurrect name or tools: both are confirmed no-ops on
// Kilo Code (see the package doc), so the escape hatch that lets
// arbitrary custom keys through must not reintroduce either. permission
// is not excluded, so it remains the documented way to reach Kilo
// Code's native per-tool access control.
func TestEmit_Agent_XKiloCannotReintroduceNameOrTools(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent, Name: "alpha",
			Meta: map[string]any{
				"description": "d",
				"x-kilo": map[string]any{
					"name":       "override",
					"tools":      []any{"Read"},
					"permission": map[string]any{"read": "allow"},
				},
			},
			Body: "body",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".kilo/agents/alpha.md"))
	if strings.Contains(got, "name:") || strings.Contains(got, "tools:") {
		t.Errorf("x-kilo must not reintroduce name or tools, got:\n%s", got)
	}
	if !strings.Contains(got, "permission:") {
		t.Errorf("x-kilo.permission is Kilo Code's documented access-control surface and must pass through, got:\n%s", got)
	}
}

// An agent spec that sets tools surfaces a coverage note: Kilo Code
// has no tools key and this adapter does not guess a permission
// mapping (see the package doc), so the gap must not stay silent.
func TestEmit_Agent_ToolsSurfacesCoverageNote(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "a1", Meta: map[string]any{"tools": []any{"Read"}}, Body: "body"},
		{Kind: spec.KindAgent, Name: "a2", Body: "body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if n := emit.PendingCoverageNotesCount(); n != 1 {
		t.Errorf("expected one coverage note (only a1 declares tools), got %d", n)
	}
}

// A bundle where no agent sets tools must not surface a coverage note.
func TestEmit_Agent_NoToolsNoCoverageGap(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "a1", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if n := emit.PendingCoverageNotesCount(); n != 0 {
		t.Errorf("expected no coverage note when no agent sets tools, got %d", n)
	}
}

// Arbitrary x-kilo keys pass through so the full agent schema is
// reachable without waiting on this adapter's allowlist.
func TestEmit_Agent_XKiloPassthrough(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent, Name: "alpha",
			Meta: map[string]any{"description": "d", "x-kilo": map[string]any{"temperature": "0.2"}},
			Body: "body",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".kilo/agents/alpha.md"))
	if !strings.Contains(got, "temperature: \"0.2\"") && !strings.Contains(got, "temperature: 0.2") {
		t.Errorf("expected x-kilo key to pass through, got:\n%s", got)
	}
}

func TestEmit_AgentsDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{Outputs: map[string]config.Output{"kilo": {AgentsDir: "custom/agents"}}}
	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "a1", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/agents/a1.md")); err != nil {
		t.Errorf("expected override dir to hold the agent file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".kilo/agents/a1.md")); !os.IsNotExist(err) {
		t.Errorf("expected no output at the default agents dir, err=%v", err)
	}
}

// Stdio MCP merges into kilo.jsonc under mcp.<name> (target-audit
// 2026-08-01, B1: Kilo Code reads mcp, not the deprecated mcpServers
// key). command and args combine into one command array, and type:
// "local" is required.
func TestEmit_MCP_StdioWritesKiloJSONC(t *testing.T) {
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
	got := readFile(t, filepath.Join(dir, "kilo.jsonc"))
	for _, want := range []string{
		`"mcp"`,
		`"fs"`,
		`"type": "local"`,
		`"npx"`,
		`"-y"`,
		`"environment"`,
		`"ALLOWED_PATHS"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, `"mcpServers"`) {
		t.Errorf("mcpServers is the deprecated MCP-spec key; Kilo Code reads mcp, got:\n%s", got)
	}
	if strings.Contains(got, `"command": "npx"`) {
		t.Errorf("command must be an array, not a bare string, got:\n%s", got)
	}
	if strings.Contains(got, `"args"`) {
		t.Errorf("command and args must combine into one command array, got:\n%s", got)
	}
	if strings.Contains(got, `"env"`) {
		t.Errorf("env must be renamed to environment, got:\n%s", got)
	}
}

// HTTP/SSE/remote MCP entries set type: "remote" (target-audit
// 2026-08-01, B1).
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
	got := readFile(t, filepath.Join(dir, "kilo.jsonc"))
	for _, want := range []string{
		`"mcp"`,
		`"linear"`,
		`"type": "remote"`,
		`"url": "https://mcp.linear.app"`,
		`"headers"`,
		`"Authorization"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// A stdio entry with no args still gets a one-element command array,
// never a bare string.
func TestEmit_MCP_StdioNoArgsStillArray(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "bare", Meta: map[string]any{"command": "x"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "kilo.jsonc"))
	if strings.Contains(got, `"command": "x"`) {
		t.Errorf("command must stay an array even with no args, got:\n%s", got)
	}
	if !strings.Contains(got, `"x"`) || !strings.Contains(got, `"type": "local"`) {
		t.Errorf("expected a one-element command array and type local, got:\n%s", got)
	}
}

// The merge only touches the mcp key: pre-existing user-managed keys
// in kilo.jsonc must survive the sync untouched.
func TestEmit_MCP_PreservesExistingUserKeys(t *testing.T) {
	dir := testutil.TempCwd(t)

	existing := `{"editor": {"theme": "dark"}, "provider": {"anthropic": {}}}`
	if err := os.WriteFile(filepath.Join(dir, "kilo.jsonc"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "kilo.jsonc"))
	for _, want := range []string{
		`"theme": "dark"`,
		`"anthropic"`,
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
			"kilo": {MCPFile: "vendor/kilo.jsonc"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "x"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/kilo.jsonc")); err != nil {
		t.Errorf("expected override path written: %v", err)
	}
}

func TestEmit_NoKiloJSONCWhenNoMCPs(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{Kind: spec.KindRule, Name: "r1", Body: "x"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "kilo.jsonc")); !os.IsNotExist(err) {
		t.Errorf("expected no kilo.jsonc when no MCP entries, err=%v", err)
	}
}

// Stdio entries without a command are dropped: there is nothing for
// Kilo Code to run.
func TestEmit_MCP_SkipsStdioWithoutCommand(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "bad", Meta: map[string]any{}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "kilo.jsonc")); !os.IsNotExist(err) {
		t.Errorf("expected no kilo.jsonc when entry has no command, err=%v", err)
	}
}

func TestEmit_EmptyBundle_WritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "kilo.jsonc")); !os.IsNotExist(err) {
		t.Errorf("expected no kilo.jsonc for empty bundle, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".kilo")); !os.IsNotExist(err) {
		t.Errorf("expected no .kilo/ for empty bundle, err=%v", err)
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
