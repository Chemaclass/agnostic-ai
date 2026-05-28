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
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/config.toml"))
	for _, want := range []string{
		`description = "Filesystem MCP"`,
		"disabled = true",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
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
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(nil), cfg, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(nil), cfg, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/config.toml")); !os.IsNotExist(err) {
		t.Error("expected no config.toml when CodexConfig is empty")
	}
}
