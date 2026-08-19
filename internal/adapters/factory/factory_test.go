package factory

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
	if got := New().Name(); got != "factory" {
		t.Errorf("Name() = %q, want %q", got, "factory")
	}
}

// The project-root AGENTS.md is written centrally by sync, never by
// this adapter: Droid CLI has no per-rule surface of its own.
func TestEmit_NoRootAGENTSMd_ByDefault(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write AGENTS.md, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".factory")); !os.IsNotExist(err) {
		t.Errorf("a rule-only bundle should not create .factory/, err=%v", err)
	}
}

func TestEmit_Agent_WritesDroidFile(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent, Name: "release-manager",
			Meta: map[string]any{"description": "Ships releases.", "model": "opus", "tools": []any{"Read", "Bash"}},
			Body: "Run the release checklist.",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".factory/droids/release-manager.md"))
	if !strings.HasPrefix(got, "---\n") {
		t.Fatalf("frontmatter must be first, got:\n%s", got)
	}
	for _, want := range []string{
		"name: release-manager",
		"description: Ships releases.",
		"model: opus",
		"tools:",
		"- Read",
		"- Execute",
		"Run the release checklist.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// DroidValidator fails the whole file on one unknown ID, so the
	// Claude spelling must not survive alongside the translation.
	if strings.Contains(got, "- Bash") {
		t.Errorf("Bash must be translated to Execute, not emitted verbatim:\n%s", got)
	}
}

func TestEmit_Agent_UnknownToolIsDroppedNotEmitted(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent, Name: "scout",
			Meta: map[string]any{"tools": []any{"Read", "Task"}},
			Body: "body",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".factory/droids/scout.md"))
	if !strings.Contains(got, "- Read") {
		t.Errorf("a translatable name must still emit:\n%s", got)
	}
	if strings.Contains(got, "Task") {
		t.Errorf("Task has no Factory ID and must not reach the file:\n%s", got)
	}
}

func TestEmit_Agent_DescriptionFallsBackToName(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "no-desc", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".factory/droids/no-desc.md"))
	if !strings.Contains(got, "description: no-desc") {
		t.Errorf("expected description fallback to agent name, got:\n%s", got)
	}
	if strings.Contains(got, "model:") || strings.Contains(got, "tools:") {
		t.Errorf("expected no model/tools keys when absent from meta, got:\n%s", got)
	}
}

// Arbitrary x-factory keys pass through so the full droid schema is
// reachable without waiting on this adapter's allowlist.
func TestEmit_Agent_XFactoryPassthrough(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent, Name: "alpha",
			Meta: map[string]any{"description": "d", "x-factory": map[string]any{"reasoning_effort": "high"}},
			Body: "body",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".factory/droids/alpha.md"))
	if !strings.Contains(got, "reasoning_effort: high") {
		t.Errorf("expected x-factory key to pass through, got:\n%s", got)
	}
}

// Droid CLI's own schema says the body after the frontmatter "is the
// system prompt and cannot be empty" (docs.factory.ai/harness/subagents).
// A spec with an empty body must not become a frontmatter-only file
// Droid CLI itself calls invalid; it skips instead, and the skip
// surfaces through a coverage note rather than staying silent.
func TestEmit_Agent_EmptyBodySkipsFileAndNotesGap(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prevWarner := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prevWarner })

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "empty", Meta: map[string]any{"description": "d"}, Body: ""},
		{Kind: spec.KindAgent, Name: "hasbody", Body: "Run the checklist."},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".factory/droids/empty.md")); !os.IsNotExist(err) {
		t.Errorf("expected no file for an empty-body agent, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".factory/droids/hasbody.md")); err != nil {
		t.Errorf("expected the non-empty-body agent to still write, err=%v", err)
	}
	if n := emit.PendingCoverageNotesCount(); n != 1 {
		t.Errorf("expected one coverage note for the empty-body agent, got %d", n)
	}
	emit.FlushCoverageNotes()
	out := buf.String()
	if !strings.Contains(out, "1 agent reaches factory only in the source dir") {
		t.Errorf("expected a coverage note naming the skipped agent, got: %s", out)
	}
	if !strings.Contains(out, "non-empty system prompt") {
		t.Errorf("expected the note to explain Droid CLI's non-empty-body requirement, got: %s", out)
	}
}

// TrimSpace already governs the body written into the frontmatter
// block; a whitespace-only body must trip the same empty-body skip as
// a literal empty string rather than writing a body of pure
// whitespace, which Droid CLI's parser would also reject.
func TestEmit_Agent_WhitespaceOnlyBodyTreatedAsEmpty(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "blank", Body: "   \n\t  "}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".factory/droids/blank.md")); !os.IsNotExist(err) {
		t.Errorf("expected no file for a whitespace-only body, err=%v", err)
	}
	if n := emit.PendingCoverageNotesCount(); n != 1 {
		t.Errorf("expected one coverage note for the whitespace-only body, got %d", n)
	}
}

func TestEmit_AgentsDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{Outputs: map[string]config.Output{"factory": {AgentsDir: "custom/droids"}}}
	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "a1", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/droids/a1.md")); err != nil {
		t.Errorf("expected override dir to hold the droid file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".factory/droids/a1.md")); !os.IsNotExist(err) {
		t.Errorf("expected no output at the default droids dir, err=%v", err)
	}
}

// Stdio MCP merges into .factory/mcp.json under the standard
// mcpServers map (target-audit 2026-08-01, MISSING: factory MCP).
func TestEmit_MCP_StdioWritesFactoryMCPJSON(t *testing.T) {
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
	got := readFile(t, filepath.Join(dir, ".factory/mcp.json"))
	for _, want := range []string{`"mcpServers"`, `"fs"`, `"command": "npx"`, `"ALLOWED_PATHS"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, `"type"`) {
		t.Errorf("stdio is the inferred default; type must stay unset, got:\n%s", got)
	}
}

func TestEmit_MCP_HTTPWritesTypeAndURL(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "github",
			Meta: map[string]any{
				"type": "http",
				"url":  "https://api.githubcopilot.com/mcp/",
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".factory/mcp.json"))
	for _, want := range []string{`"type": "http"`, `"url": "https://api.githubcopilot.com/mcp/"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// Factory's own schema documents a working `disabled` boolean (unlike
// Claude Code, Cursor, and Copilot), so this adapter must not strip it
// the way those three do.
func TestEmit_MCP_DisabledPassesThrough(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx", "disabled": true}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".factory/mcp.json"))
	if !strings.Contains(got, `"disabled": true`) {
		t.Errorf("Factory documents a real disabled key; must pass it through, got:\n%s", got)
	}
}

func TestEmit_MCP_FileOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"factory": {MCPFile: "vendor/factory-mcp.json"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/factory-mcp.json")); err != nil {
		t.Errorf("expected override path written: %v", err)
	}
}

func TestEmit_NoMCPJSONWhenNoMCPs(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{Kind: spec.KindRule, Name: "r1", Body: "x"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".factory/mcp.json")); !os.IsNotExist(err) {
		t.Errorf("expected no .factory/mcp.json when no MCP entries, err=%v", err)
	}
}

func TestEmit_EmptyBundle_WritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".factory")); !os.IsNotExist(err) {
		t.Errorf("expected no .factory/ for an empty bundle, err=%v", err)
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
