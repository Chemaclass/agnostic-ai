package codex

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

// Stdio MCP emits a [mcp_servers.<name>] table with command/args/env.
func TestEmit_MCP_StdioWritesConfigToml(t *testing.T) {
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
	got := readFile(t, filepath.Join(dir, ".codex/config.toml"))
	for _, want := range []string{
		"[mcp_servers.fs]",
		`command = "npx"`,
		`args = ["-y", "@modelcontextprotocol/server-filesystem", "."]`,
		`env = { ALLOWED_PATHS = "." }`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// learn.chatgpt.com/docs/config-file/config-reference documents `cwd`
// ("Working directory for the MCP stdio server process") on stdio
// entries. See #532.
func TestEmit_MCP_StdioWritesCwd(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command": "npx",
				"cwd":     "/workspace/project",
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/config.toml"))
	if !strings.Contains(got, `cwd = "/workspace/project"`) {
		t.Errorf("missing cwd in %s", got)
	}
}

// Codex accepts an env_vars array containing environment variable names or
// named source tables. Preserve both documented forms.
func TestEmit_MCP_StdioWritesEnvVars(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{
		Kind: spec.KindMCP,
		Name: "remote",
		Meta: map[string]any{
			"command": "mcp-server",
			"env_vars": []any{
				"LOCAL_TOKEN",
				map[string]any{"name": "REMOTE_TOKEN", "source": "remote"},
			},
		},
	}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/config.toml"))
	want := `env_vars = ["LOCAL_TOKEN", { name = "REMOTE_TOKEN", source = "remote" }]`
	if !strings.Contains(got, want) {
		t.Errorf("missing %q in %s", want, got)
	}
}

func TestEmit_MCP_HTTPWritesURL(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "github",
			Meta: map[string]any{
				"type":                 "http",
				"url":                  "https://api.githubcopilot.com/mcp/",
				"bearer_token_env_var": "GH_TOKEN",
				"headers":              map[string]any{"X-Trace": "1"},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/config.toml"))
	for _, want := range []string{
		"[mcp_servers.github]",
		`url = "https://api.githubcopilot.com/mcp/"`,
		`bearer_token_env_var = "GH_TOKEN"`,
		`http_headers = { X-Trace = "1" }`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// learn.chatgpt.com/docs/config-file/config-reference documents `auth`
// (`oauth | chatgpt`) on http entries. See #532.
func TestEmit_MCP_HTTPWritesAuth(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "github",
			Meta: map[string]any{
				"type": "http",
				"url":  "https://api.githubcopilot.com/mcp/",
				"auth": "oauth",
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/config.toml"))
	if !strings.Contains(got, `auth = "oauth"`) {
		t.Errorf("missing auth in %s", got)
	}
}

// env_http_headers maps HTTP header names to environment variable names.
func TestEmit_MCP_HTTPWritesEnvHTTPHeaders(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{
		Kind: spec.KindMCP,
		Name: "github",
		Meta: map[string]any{
			"type":             "http",
			"url":              "https://example.com/mcp",
			"env_http_headers": map[string]any{"Authorization": "MCP_TOKEN"},
		},
	}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/config.toml"))
	want := `env_http_headers = { Authorization = "MCP_TOKEN" }`
	if !strings.Contains(got, want) {
		t.Errorf("missing %q in %s", want, got)
	}
}

// learn.chatgpt.com/docs/config-file/config-reference documents
// http_headers_helper on http entries only ("Supported only for locally
// connected HTTP MCP servers"). See #661.
func TestEmit_MCP_HTTPWritesHeadersHelper(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "github",
			Meta: map[string]any{
				"type":                "http",
				"url":                 "https://api.githubcopilot.com/mcp/",
				"http_headers_helper": "./scripts/mcp-headers.sh",
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/config.toml"))
	if !strings.Contains(got, `http_headers_helper = "./scripts/mcp-headers.sh"`) {
		t.Errorf("missing http_headers_helper in %s", got)
	}
}

// learn.chatgpt.com/docs/config-file/config-reference documents
// enabled_tools / disabled_tools as shared mcp_servers.<id> keys, not
// restricted to a transport, so they emit alongside description/enabled
// via writeMCPSharedFields. See #661.
func TestEmit_MCP_EnabledAndDisabledTools(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command":        "npx",
				"enabled_tools":  []any{"read_file", "list_dir"},
				"disabled_tools": []any{"delete_file"},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/config.toml"))
	for _, want := range []string{
		`enabled_tools = ["read_file", "list_dir"]`,
		`disabled_tools = ["delete_file"]`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// Hooks emit into .codex/hooks.json grouped per event in the same shape
// Claude's settings.json hooks block uses.
func TestEmit_Hook_GroupsByEvent(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook,
			Name: "pre-bash",
			Meta: map[string]any{
				"event":   "PreToolUse",
				"matcher": "Bash",
				"command": "echo pre-tool",
			},
		},
		{
			Kind: spec.KindHook,
			Name: "session",
			Meta: map[string]any{
				"event":   "SessionStart",
				"command": "echo started",
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/hooks.json"))
	for _, want := range []string{
		`"PreToolUse"`,
		`"matcher": "Bash"`,
		`"command": "echo pre-tool"`,
		`"SessionStart"`,
		`"command": "echo started"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// Hooks scoped to another target via `target:` must be skipped entirely
// from .codex/hooks.json so a claude-only hook does not leak into codex.
func TestEmit_Hook_TargetScopingFiltersOtherTargets(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "claude-only",
			Meta: map[string]any{
				"event": "PostToolUse", "matcher": "Edit",
				"command": "echo claude", "target": "claude",
			},
		},
		{
			Kind: spec.KindHook, Name: "codex-rule",
			Meta: map[string]any{
				"event": "PostToolUse", "matcher": "Edit",
				"command": "echo codex", "target": "codex",
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/hooks.json"))
	if !strings.Contains(got, `"command": "echo codex"`) {
		t.Errorf("codex-scoped hook missing in codex hooks.json:\n%s", got)
	}
	if strings.Contains(got, `"command": "echo claude"`) {
		t.Errorf("claude-scoped hook must not leak into codex hooks.json:\n%s", got)
	}
}

// A hook spec carrying a command array emits one entry per command
// inside the same matcher group.
func TestEmit_Hook_CommandArrayExpandsToMultipleBlocks(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook,
			Name: "post-format",
			Meta: map[string]any{
				"event":   "PostToolUse",
				"matcher": "Edit|Write",
				"command": []any{".codex/hooks/format-php.sh", ".codex/hooks/format-phel.sh"},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/hooks.json"))
	for _, want := range []string{
		`"command": ".codex/hooks/format-php.sh"`,
		`"command": ".codex/hooks/format-phel.sh"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Count(got, `"PostToolUse"`) != 1 {
		t.Errorf("expected one PostToolUse event key, got:\n%s", got)
	}
}

// A hook spec authored against another tool's hooks directory must
// rewrite the `.<sibling>/hooks/` prefix to `.codex/hooks/` so the
// emitted hooks.json points at a path that actually exists under the
// codex tree.
func TestEmit_Hook_RewritesSiblingHookPathToCodex(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook,
			Name: "h1",
			Meta: map[string]any{
				"event":   "PreToolUse",
				"matcher": "Edit",
				"command": ".claude/hooks/protect-files.sh",
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/hooks.json"))
	if !strings.Contains(got, `"command": ".codex/hooks/protect-files.sh"`) {
		t.Errorf("expected sibling claude path rewritten to .codex/hooks/, got:\n%s", got)
	}
	if strings.Contains(got, ".claude/hooks/") {
		t.Errorf("emitted hooks.json still references .claude/hooks/:\n%s", got)
	}
}

// Hooks with no event are skipped.
func TestEmit_Hook_SkipsWhenNoEvent(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook,
			Name: "bad",
			Meta: map[string]any{"command": "echo"},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/config.toml")); !os.IsNotExist(err) {
		t.Errorf("expected no config.toml when hook has no event, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/hooks.json")); !os.IsNotExist(err) {
		t.Errorf("expected no hooks.json when hook has no event, err=%v", err)
	}
}

// outputs.codex.mcp-file relocates the config.
func TestEmit_ConfigToml_FileOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"codex": {MCPFile: "vendor/codex.toml"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "x"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/codex.toml")); err != nil {
		t.Errorf("expected override path written: %v", err)
	}
}

func TestEmit_ConfigToml_NoFileWhenNoMCPOrHooks(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "x"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/config.toml")); !os.IsNotExist(err) {
		t.Errorf("expected no config.toml when no MCP/hook entries, err=%v", err)
	}
}

func TestEmit_ConfigToml_RemovesOrphanWhenNothingToRender(t *testing.T) {
	dir := testutil.TempCwd(t)

	orphan := filepath.Join(dir, ".codex/config.toml")
	if err := os.MkdirAll(filepath.Dir(orphan), 0o755); err != nil {
		t.Fatal(err)
	}
	stale := "# Generated by agnostic-ai. Do not edit this file directly; edit specs under .agnostic-ai/ and run `agnostic-ai sync`.\n[mcp_servers.gone]\ncommand = \"old\"\n"
	if err := os.WriteFile(orphan, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "x"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Errorf("expected stale config.toml removed, stat err=%v", err)
	}
}

func TestEmit_ConfigToml_PreservesHandAuthoredWhenNothingToRender(t *testing.T) {
	dir := testutil.TempCwd(t)

	path := filepath.Join(dir, ".codex/config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	user := "# user-authored config\nmodel = \"o1\"\n"
	if err := os.WriteFile(path, []byte(user), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "x"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("user file unexpectedly removed: %v", err)
	}
	if string(got) != user {
		t.Errorf("user file modified: got %q want %q", got, user)
	}
}

func TestEmit_SweepsLegacyAgentsAndSkillsDirs(t *testing.T) {
	dir := testutil.TempCwd(t)

	legacyAgent := filepath.Join(dir, ".agents/agents/old-agent.toml")
	if err := os.MkdirAll(filepath.Dir(legacyAgent), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyAgent, []byte("# Generated by agnostic-ai\nname = \"old\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// v0.26..v0.42 emitted skills to .codex/skills/, a path Codex CLI
	// never scans; the sweep now runs in that direction.
	legacySkill := filepath.Join(dir, ".codex/skills/old/SKILL.md")
	if err := os.MkdirAll(filepath.Dir(legacySkill), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacySkill, []byte("<!-- Generated by agnostic-ai -->\n---\nname: old\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "current", Body: "x"},
		{Kind: spec.KindSkill, Name: "fresh", Body: "y"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(legacyAgent); !os.IsNotExist(err) {
		t.Errorf("expected legacy agent removed, stat err=%v", err)
	}
	if _, err := os.Stat(legacySkill); !os.IsNotExist(err) {
		t.Errorf("expected legacy skill removed, stat err=%v", err)
	}
	// .agents/ now hosts the current skills tree, so it must survive the
	// legacy-agents sweep.
	if _, err := os.Stat(filepath.Join(dir, ".agents/skills/fresh/SKILL.md")); err != nil {
		t.Errorf("current skills tree must survive the sweep: %v", err)
	}
}

func TestEmit_LegacyAgentSweepPreservesSharedAgentsRoot(t *testing.T) {
	dir := testutil.TempCwd(t)

	legacyAgent := filepath.Join(dir, ".agents/agents/old-agent.toml")
	if err := os.MkdirAll(filepath.Dir(legacyAgent), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyAgent, []byte("# Generated by agnostic-ai\nname = \"old\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "current", Body: "x"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".agents")); err != nil {
		t.Errorf("shared .agents root must survive the Codex legacy sweep: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents/agents")); !os.IsNotExist(err) {
		t.Errorf("legacy Codex agents directory should still be removed, stat err=%v", err)
	}
}

func TestEmit_SweepSkippedWhenUserOptsIntoLegacyDir(t *testing.T) {
	dir := testutil.TempCwd(t)

	legacyAgent := filepath.Join(dir, ".agents/agents/keeper.toml")
	if err := os.MkdirAll(filepath.Dir(legacyAgent), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyAgent, []byte("# Generated by agnostic-ai\nname = \"keeper\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"codex": {AgentsDir: ".agents/agents"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "active", Body: "x"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(legacyAgent); err != nil {
		t.Errorf("opt-in legacy dir must be preserved, stat err=%v", err)
	}
}

// Codex CLI's config-reference documents `mcp_servers.<id>.enabled:
// boolean`, not `disabled`. The spec's `disabled: true` must map to the
// key Codex actually reads: `enabled = false`. A literal `disabled = true`
// key is not one Codex parses, so it silently fails to stop the server.
func TestEmit_MCP_DescriptionAndDisabled(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command":     "npx",
				"description": "Filesystem MCP",
				"disabled":    true,
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/config.toml"))
	for _, want := range []string{
		`description = "Filesystem MCP"`,
		"enabled = false",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, "disabled") {
		t.Errorf("codex does not read a `disabled` key; must not emit it: %s", got)
	}
}

func TestEmit_MCP_Roots(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command": "npx",
				"roots": []any{
					map[string]any{"uri": "file:///workspace", "name": "workspace"},
					map[string]any{"uri": "file:///tmp"},
				},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/config.toml"))
	if !strings.Contains(got, `roots = [{ uri = "file:///workspace", name = "workspace" }, { uri = "file:///tmp" }]`) {
		t.Errorf("missing roots inline array in:\n%s", got)
	}
}

// MCP env values with backslashes / quotes survive the TOML round-trip.
func TestEmit_MCP_EnvEscapesQuotesAndBackslashes(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "tricky",
			Meta: map[string]any{
				"command": "x",
				"env":     map[string]any{"K": `has "quotes" and \ slash`},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/config.toml"))
	if !strings.Contains(got, `K = "has \"quotes\" and \\ slash"`) {
		t.Errorf("expected escaped env value, got: %s", got)
	}
}

func TestEmit_CodexConfig_SandboxAndApprovalPolicy(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"codex": {
				Config: &config.CodexConfig{
					Sandbox:        "workspace-write",
					ApprovalPolicy: "on-failure",
					Model:          "o4-mini",
				},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/config.toml"))
	for _, want := range []string{
		`sandbox = "workspace-write"`,
		`approval_policy = "on-failure"`,
		`model = "o4-mini"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestEmit_CodexConfig_ModelReasoningAndHistory(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"codex": {
				Config: &config.CodexConfig{
					ModelReasoningEffort:  "medium",
					ModelReasoningSummary: "auto",
					HistoryPersistence:    "enabled",
				},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/config.toml"))
	for _, want := range []string{
		`model_reasoning_effort = "medium"`,
		`model_reasoning_summary = "auto"`,
		"[history]",
		`persistence = "enabled"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestEmit_CodexConfig_Notify(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"codex": {
				Config: &config.CodexConfig{
					Notify: []string{"python3", "/etc/codex/notify.py"},
				},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/config.toml"))
	if !strings.Contains(got, `notify = ["python3", "/etc/codex/notify.py"]`) {
		t.Errorf("missing notify array in:\n%s", got)
	}
}

func TestEmit_CodexConfig_Profiles(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"codex": {
				Config: &config.CodexConfig{
					Profiles: map[string]config.CodexProfile{
						"work": {
							Model:          "o4-mini",
							Sandbox:        "workspace-write",
							ApprovalPolicy: "on-failure",
						},
						"oss": {
							Model:         "gpt-oss-20b",
							ModelProvider: "ollama",
						},
					},
				},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/config.toml"))
	for _, want := range []string{
		"[profiles.oss]",
		`model = "gpt-oss-20b"`,
		`model_provider = "ollama"`,
		"[profiles.work]",
		`model = "o4-mini"`,
		`sandbox = "workspace-write"`,
		`approval_policy = "on-failure"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// Sort order: oss before work alphabetically.
	if strings.Index(got, "[profiles.oss]") > strings.Index(got, "[profiles.work]") {
		t.Error("expected profiles to emit in sorted order (oss before work)")
	}
}

func TestEmit_CodexConfig_ModelProviders(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"codex": {
				Config: &config.CodexConfig{
					ModelProviders: map[string]config.CodexModelProvider{
						"ollama": {
							Name:    "Ollama",
							BaseURL: "http://localhost:11434/v1",
							WireAPI: "responses",
						},
						"azure": {
							Name:      "Azure",
							BaseURL:   "https://my.openai.azure.com/openai",
							WireAPI:   "responses",
							APIKeyEnv: "AZURE_OPENAI_API_KEY",
						},
					},
				},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/config.toml"))
	for _, want := range []string{
		"[model_providers.azure]",
		`name = "Azure"`,
		`base_url = "https://my.openai.azure.com/openai"`,
		`api_key_env = "AZURE_OPENAI_API_KEY"`,
		"[model_providers.ollama]",
		`name = "Ollama"`,
		`base_url = "http://localhost:11434/v1"`,
		`wire_api = "responses"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Index(got, "[model_providers.azure]") > strings.Index(got, "[model_providers.ollama]") {
		t.Error("expected model providers to emit in sorted order (azure before ollama)")
	}
}

// Overlay carries user-authored keys outside hooks/mcp_servers and is
// concatenated before the spec-derived sections.
func TestEmit_CodexConfig_OverlayLayered(t *testing.T) {
	dir := testutil.TempCwd(t)

	if err := os.MkdirAll(filepath.Join(dir, ".agnostic-ai/overlays"), 0o755); err != nil {
		t.Fatal(err)
	}
	overlay := `model = "gpt-5"

[profiles.work]
model = "gpt-5"
`
	if err := os.WriteFile(filepath.Join(dir, ".agnostic-ai/overlays/codex.config.toml"), []byte(overlay), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{"command": "npx"},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/config.toml"))
	for _, want := range []string{
		`model = "gpt-5"`,
		`[profiles.work]`,
		`[mcp_servers.fs]`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Index(got, "[profiles.work]") > strings.Index(got, "[mcp_servers.fs]") {
		t.Error("overlay should precede spec-derived mcp_servers section")
	}
}

// Overlay-set scalars must not be duplicated by outputs.codex.config
// (TOML forbids duplicate top-level keys).
func TestEmit_CodexConfig_OverlayWinsOnDuplicate(t *testing.T) {
	dir := testutil.TempCwd(t)

	if err := os.MkdirAll(filepath.Join(dir, ".agnostic-ai/overlays"), 0o755); err != nil {
		t.Fatal(err)
	}
	overlay := `model = "from-overlay"
`
	if err := os.WriteFile(filepath.Join(dir, ".agnostic-ai/overlays/codex.config.toml"), []byte(overlay), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"codex": {Config: &config.CodexConfig{Model: "from-cfg", Sandbox: "workspace-write"}},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/config.toml"))
	if strings.Count(got, "model = ") != 1 {
		t.Errorf("expected exactly one model line (overlay wins), got:\n%s", got)
	}
	if !strings.Contains(got, `model = "from-overlay"`) {
		t.Errorf("expected overlay model value to win:\n%s", got)
	}
	if !strings.Contains(got, `sandbox = "workspace-write"`) {
		t.Errorf("expected outputs.codex.config.sandbox to still emit:\n%s", got)
	}
}

func TestEmit_CodexConfig_NoFileWhenEmpty(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"codex": {Config: &config.CodexConfig{}},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/config.toml")); !os.IsNotExist(err) {
		t.Error("expected no config.toml when CodexConfig is empty")
	}
}
