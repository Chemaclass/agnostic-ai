package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportGrok_RejectsInlineAgentCredentialsBeforeWritingSource(t *testing.T) {
	for _, tc := range []struct{ name, fields string }{
		{"map MCP env", "mcpServers:\n  - docs:\n      command: server\n      env: {API_TOKEN: synthetic-private-value}\n"},
		{"named MCP header", "mcpServers:\n  - name: docs\n    url: https://docs.example/mcp\n    headers: {Authorization: 'Bearer synthetic-private-value'}\n"},
		{"ACP MCP env", "mcpServers:\n  - docs:\n      command: server\n      args: []\n      env: [{name: API_TOKEN, value: synthetic-private-value}]\n"},
		{"MCP argument", "mcpServers:\n  - docs:\n      command: server\n      args: [--api-token, synthetic-private-value]\n"},
		{"inline hook env", "hooks:\n  PreToolUse:\n    - hooks:\n        - type: command\n          command: echo safe\n          env: {API_TOKEN: synthetic-private-value}\n"},
		{"mixed-key inline hook env", "hooks:\n  PreToolUse:\n    - hooks:\n        - type: command\n          command: echo safe\n          env: {1: harmless, API_TOKEN: synthetic-private-value}\n"},
		{"inline hook URL", "hooks:\n  PreToolUse:\n    - hooks:\n        - type: http\n          url: https://example.test/hook?token=synthetic-private-value\n"},
		{"malformed metadata", "mcpServers: [synthetic-private-value\n"},
	} {
		for _, scope := range []string{"", "backend"} {
			t.Run(filepath.Join(tc.name, scope), func(t *testing.T) {
				dir := testutil.TempCwd(t)
				silence(t)
				writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [grok]\n")
				path := filepath.Join(dir, scope, ".grok/agents/reviewer.md")
				writeFile(t, path, "---\nname: reviewer\ndescription: Review work\n"+tc.fields+"---\nRead docs.\n")
				output, err := runCLI(t, "import", "grok")
				if err == nil {
					t.Fatal("unsafe native metadata imported")
				}
				if text := output + err.Error(); strings.Contains(text, "synthetic-private-value") || !strings.Contains(text, filepath.Join(scope, ".grok/agents/reviewer.md")) {
					t.Fatal("import error must identify the native path without metadata values")
				}
				if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai/agents", scope, "reviewer.md")); !os.IsNotExist(err) {
					t.Fatal("unsafe agent source was written")
				}
			})
		}
	}
}

func TestImportGrok_RejectsRetainedSkillCredentialsBeforeWritingSource(t *testing.T) {
	for _, scope := range []string{"", "backend"} {
		t.Run(scope, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [grok]\n")
			path := filepath.Join(scope, ".grok/skills/review/SKILL.md")
			body := "---\nname: review\ndescription: Review\nhooks:\n  PreToolUse:\n    - hooks:\n        - type: command\n          command: echo safe\n          env: {1: harmless, API_TOKEN: synthetic-private-value}\n---\nReview.\n"
			writeFile(t, path, body)
			output, err := runCLI(t, "import", "grok")
			if err == nil {
				t.Error("unsafe skill metadata imported")
			} else if strings.Contains(output+err.Error(), "synthetic-private-value") || !strings.Contains(err.Error(), path) {
				t.Error("import error must identify the native path without metadata values")
			}
			if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai/skills", scope, "review/SKILL.md")); !os.IsNotExist(err) {
				t.Error("unsafe skill source was written")
			}
			if got := readFile(t, path); got != body {
				t.Error("native skill changed")
			}
		})
	}
}

func TestImportGrok_SafeInlineMetadataSurvivesFreshOutput(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [grok]\n")
	fields := "mcpServers:\n  - shared\n  - docs:\n      command: server\n      env: {MODE: safe}\nhooks:\n  PreToolUse:\n    - hooks:\n        - type: command\n          command: echo ${API_TOKEN}\n          env: {MODE: safe}\n        - type: http\n          url: https://example.test/hooks/${API_TOKEN}\n"
	writeFile(t, filepath.Join(dir, ".grok/agents/reviewer.md"), "---\nname: reviewer\ndescription: Review work\n"+fields+"---\nRead docs.\n")
	execCLI(t, "import", "grok")
	if err := os.RemoveAll(filepath.Join(dir, ".grok")); err != nil {
		t.Fatal(err)
	}
	execCLI(t, "sync", "-t", "grok")
	got := readFile(t, filepath.Join(dir, ".grok/agents/reviewer.md"))
	for _, want := range []string{"mcpServers:", "shared", "MODE: safe", "echo ${API_TOKEN}", "https://example.test/hooks/${API_TOKEN}"} {
		if !strings.Contains(got, want) {
			t.Errorf("safe native metadata lost %q", want)
		}
	}
	execCLI(t, "sync", "--check")
}

func TestImportGrok_PluginsSurviveFreshSettingsDestination(t *testing.T) {
	for _, split := range []bool{false, true} {
		t.Run(map[bool]string{false: "native config", true: "split settings config"}[split], func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			configuration := "version: 1\ntargets: [grok]\n"
			path := filepath.Join(dir, ".grok/config.toml")
			if split {
				configuration += "outputs:\n  grok:\n    conf-file: settings/grok.toml\n"
				path = filepath.Join(dir, "settings/grok.toml")
				writeFile(t, filepath.Join(dir, ".grok/config.toml"), "[plugins]\nenabled = ['wrong-destination']\n")
			}
			writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), configuration)
			writeFile(t, path, "[plugins]\nenabled = ['team-plugin']\n[plugins.marketplaces.team]\nsource = 'https://example.test/plugins'\n")
			execCLI(t, "import", "grok")
			got := readFile(t, filepath.Join(dir, ".agnostic-ai/settings/grok.yaml"))
			if !strings.Contains(got, "x-grok:") || !strings.Contains(got, "team-plugin") || strings.Contains(got, "wrong-destination") {
				t.Fatal("plugins not captured from the selected native settings destination")
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			execCLI(t, "sync", "-t", "grok")
			got = readFile(t, path)
			for _, want := range []string{"plugins", "team-plugin", "https://example.test/plugins"} {
				if !strings.Contains(got, want) {
					t.Errorf("fresh output lost %q", want)
				}
			}
			execCLI(t, "sync", "--check")
			if err := os.Remove(filepath.Join(dir, ".agnostic-ai/settings/grok.yaml")); err != nil {
				t.Fatal(err)
			}
			execCLI(t, "import", "grok")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			execCLI(t, "sync", "-t", "grok")
			if got := readFile(t, path); !strings.Contains(got, "team-plugin") || !strings.Contains(got, "https://example.test/plugins") {
				t.Fatal("importing emitted plugins into fresh sources lost native configuration")
			}
			execCLI(t, "sync", "--check")
		})
	}
}
