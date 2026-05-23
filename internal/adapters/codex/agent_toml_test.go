package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// The on-disk spec filename canonicalises to dash-case (so claude and
// codex collapse onto one file) but the emitted TOML must carry the
// codex runtime identifier — typically an underscored name. `x-codex.name`
// is the override hook for this divergence.
func TestAgentTOML_UsesXCodexNameForRuntimeIdentifier(t *testing.T) {
	a := spec.Entry{
		Kind: spec.KindAgent,
		Name: "changelog-keeper",
		Body: "instructions",
		Meta: map[string]any{
			"description": "changelog agent",
			"x-codex":     map[string]any{"name": "changelog_keeper"},
		},
	}
	got := agentTOML(a)
	if !strings.Contains(got, `name = "changelog_keeper"`) {
		t.Errorf("expected x-codex.name to override emitted runtime name:\n%s", got)
	}
	if strings.Contains(got, `name = "changelog-keeper"`) {
		t.Errorf("unexpected dashed name still emitted:\n%s", got)
	}
}

func TestAgentTOML_EscapesQuotesAndBackslashes(t *testing.T) {
	a := spec.Entry{
		Kind: spec.KindAgent,
		Name: "tricky",
		Body: `Body with "quotes" and a \ backslash and a """ run.`,
		Meta: map[string]any{"description": `Says "hi" with a \ slash`},
	}
	got := agentTOML(a)
	for _, want := range []string{
		`description = "Says \"hi\" with a \\ slash"`,
		`Body with \"quotes\" and a \\ backslash and a \"\"\" run.`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestAgentTOML_BodyFallsBackToDescription(t *testing.T) {
	a := spec.Entry{
		Kind: spec.KindAgent,
		Name: "minimal",
		Meta: map[string]any{"description": "describe me"},
	}
	got := agentTOML(a)
	if !strings.Contains(got, `description = "describe me"`) {
		t.Errorf("missing description:\n%s", got)
	}
	if !strings.Contains(got, "describe me\n\"\"\"") {
		t.Errorf("body should fall back to description when empty:\n%s", got)
	}
}

func TestAgentTOML_DescriptionFallsBackToName(t *testing.T) {
	a := spec.Entry{
		Kind: spec.KindAgent,
		Name: "explorer",
		Body: "Find things.",
	}
	got := agentTOML(a)
	if !strings.Contains(got, `description = "explorer"`) {
		t.Errorf("description should fall back to name:\n%s", got)
	}
}

func TestAgentTOML_XCodexOverridesTopLevel(t *testing.T) {
	a := spec.Entry{
		Kind: spec.KindAgent,
		Name: "ag",
		Body: "x",
		Meta: map[string]any{
			"model": "gpt-5",
			"x-codex": map[string]any{
				"model": "gpt-5.4",
			},
		},
	}
	got := agentTOML(a)
	if !strings.Contains(got, `model = "gpt-5.4"`) {
		t.Errorf("x-codex.model should win over top-level model:\n%s", got)
	}
	if strings.Contains(got, `model = "gpt-5"`+"\n") {
		t.Errorf("top-level model should not survive once x-codex.model is set:\n%s", got)
	}
}

func TestAgentTOML_ToolsFirstClass(t *testing.T) {
	a := spec.Entry{
		Kind: spec.KindAgent,
		Name: "scoped",
		Body: "Limited tool set.",
		Meta: map[string]any{
			"tools": []any{"bash", "edit", "read"},
		},
	}
	got := agentTOML(a)
	if !strings.Contains(got, `tools = ["bash", "edit", "read"]`) {
		t.Errorf("missing tools array:\n%s", got)
	}
}

func TestAgentTOML_XCodexToolsOverridesTopLevel(t *testing.T) {
	a := spec.Entry{
		Kind: spec.KindAgent,
		Name: "scoped",
		Body: "x",
		Meta: map[string]any{
			"tools": []any{"bash"},
			"x-codex": map[string]any{
				"tools": []any{"edit", "read"},
			},
		},
	}
	got := agentTOML(a)
	// Only one tools line, with x-codex content winning.
	if !strings.Contains(got, `tools = ["edit", "read"]`) {
		t.Errorf("x-codex.tools should win:\n%s", got)
	}
	if strings.Count(got, "tools = ") != 1 {
		t.Errorf("tools should emit exactly once:\n%s", got)
	}
}

// Agent-scoped mcp_servers passes through x-codex as a nested table so
// codex consumers see real `[mcp_servers.<name>]` headers instead of a
// flattened inline table (which codex would reject).
func TestAgentTOML_XCodexMCPServersEmitsAsNestedTable(t *testing.T) {
	a := spec.Entry{
		Kind: spec.KindAgent,
		Name: "ag",
		Body: "x",
		Meta: map[string]any{
			"x-codex": map[string]any{
				"mcp_servers": map[string]any{
					"fs": map[string]any{
						"command": "npx",
						"args":    []any{"server-filesystem", "."},
						"env":     map[string]any{"PATH": "/usr/bin"},
					},
				},
			},
		},
	}
	got := agentTOML(a)
	for _, want := range []string{
		"[mcp_servers.fs]",
		`command = "npx"`,
		`args = ["server-filesystem", "."]`,
		`env = { PATH = "/usr/bin" }`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// Scalar lines must come before the [mcp_servers.*] header.
	scalarIdx := strings.Index(got, `description = `)
	tableIdx := strings.Index(got, "[mcp_servers.fs]")
	if scalarIdx == -1 || tableIdx == -1 || scalarIdx > tableIdx {
		t.Errorf("nested table must follow top-level scalars (TOML rule):\n%s", got)
	}
}

func TestAgentTOML_XCodexNestedTablesSortedDeterministically(t *testing.T) {
	a := spec.Entry{
		Kind: spec.KindAgent,
		Name: "ag",
		Body: "x",
		Meta: map[string]any{
			"x-codex": map[string]any{
				"mcp_servers": map[string]any{
					"zeta":  map[string]any{"command": "z"},
					"alpha": map[string]any{"command": "a"},
				},
			},
		},
	}
	got := agentTOML(a)
	if strings.Index(got, "[mcp_servers.alpha]") > strings.Index(got, "[mcp_servers.zeta]") {
		t.Errorf("nested tables must emit in sorted order:\n%s", got)
	}
}

// Full emit -> decode round-trip: agent.toml with embedded
// `[mcp_servers.fs]` decodes back into the original Go map shape so the
// import path sees the same values it would on a fresh project.
func TestEmit_Agent_XCodexMCPServersDecodesBackToInput(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent,
			Name: "scoped",
			Body: "do things",
			Meta: map[string]any{
				"description": "scoped agent",
				"x-codex": map[string]any{
					"mcp_servers": map[string]any{
						"fs": map[string]any{
							"command": "npx",
							"args":    []any{"server-filesystem", "."},
						},
					},
				},
			},
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".agents/agents/scoped.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if _, err := toml.Decode(string(data), &doc); err != nil {
		t.Fatalf("emitted TOML did not decode: %v\n%s", err, data)
	}
	mcps, _ := doc["mcp_servers"].(map[string]any)
	fs, _ := mcps["fs"].(map[string]any)
	if got, _ := fs["command"].(string); got != "npx" {
		t.Errorf("command lost on round-trip, got %q\n%s", got, data)
	}
	args, _ := fs["args"].([]any)
	if len(args) != 2 || args[0] != "server-filesystem" {
		t.Errorf("args lost on round-trip, got %v\n%s", args, data)
	}
}

func TestAgentTOML_NicknameCandidatesIgnoresNonStringSlice(t *testing.T) {
	a := spec.Entry{
		Kind: spec.KindAgent,
		Name: "ag",
		Body: "x",
		Meta: map[string]any{
			"x-codex": map[string]any{
				"nickname_candidates": []any{"Atlas", 42}, // mixed types: skip
			},
		},
	}
	got := agentTOML(a)
	if strings.Contains(got, "nickname_candidates") {
		t.Errorf("non-string slice should be skipped:\n%s", got)
	}
}
