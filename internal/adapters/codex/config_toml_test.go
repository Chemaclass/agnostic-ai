package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
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

// Hooks group by event into [[hooks.<event>]] arrays of tables.
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
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/config.toml"))
	for _, want := range []string{
		"[[hooks.PreToolUse]]",
		`matcher = "Bash"`,
		`command = "echo pre-tool"`,
		"[[hooks.SessionStart]]",
		`command = "echo started"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
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
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/config.toml")); !os.IsNotExist(err) {
		t.Errorf("expected no config.toml when hook has no event, err=%v", err)
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
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/config.toml")); !os.IsNotExist(err) {
		t.Errorf("expected no config.toml when no MCP/hook entries, err=%v", err)
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
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(nil), cfg, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(nil), cfg, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(nil), cfg, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(nil), cfg, false); err != nil {
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

func TestEmit_CodexConfig_NoFileWhenEmpty(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"codex": {Config: &config.CodexConfig{}},
		},
	}
	if err := New().Emit(spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/config.toml")); !os.IsNotExist(err) {
		t.Error("expected no config.toml when CodexConfig is empty")
	}
}
