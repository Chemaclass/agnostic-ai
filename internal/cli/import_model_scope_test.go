package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestScopeNewImportedModel(t *testing.T) {
	cases := []struct {
		name, source, file, existing, model, want string
	}{
		{"vendor model", "copilot", "rev.md", "", "gpt-4", "model: {copilot: gpt-4}\n"},
		{"shared agent file", "goose", "rev.md", "", "gpt-4", "model: {goose: gpt-4, openhands: gpt-4}\n"},
		{"existing spec without a model", "copilot", "rev.md", "---\nname: rev\n---\n\nBody.\n", "gpt-4", "model: {copilot: gpt-4}\n"},
		{"existing scalar keeps its shape", "copilot", "rev.md", "---\nname: rev\nmodel: gpt-3\n---\n\nBody.\n", "gpt-4", "model: gpt-4\n"},
		{"inherit stays portable", "copilot", "rev.md", "", "inherit", "model: inherit\n"},
		{"tier stays portable", "copilot", "rev.md", "", "fast", "model: fast\n"},
		{"skills stay as written", "codex", "SKILL.md", "", "sonnet", "model: sonnet\n"},
		{"claude scopes its own names", "claude", "rev.md", "", "gpt-4", "model: gpt-4\n"},
		{"no source", "", "rev.md", "", "gpt-4", "model: gpt-4\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			priorTarget, priorCfg := importScopeTarget, importHookConfig
			t.Cleanup(func() { importScopeTarget, importHookConfig = priorTarget, priorCfg })
			importScopeTarget = c.source
			importHookConfig = &config.Config{Models: map[string]config.ModelTier{"fast": {}}}
			var existing []byte
			if c.existing != "" {
				existing = []byte(c.existing)
			}

			got := string(scopeNewImportedModel(existing, filepath.Join("x", c.file), []byte("---\nname: rev\nmodel: "+c.model+"\n---\n\nBody.\n")))
			if want := "---\nname: rev\n" + c.want + "---\n\nBody.\n"; got != want {
				t.Errorf("got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestImportThenUseClaude_KeepsAnotherToolsModelOutOfClaude(t *testing.T) {
	cases := []struct {
		name, sources, claudeFile string
		native                    map[string]string
	}{
		{
			name: "codex agent", sources: "codex", claudeFile: ".claude/agents/reviewer.md",
			native: map[string]string{".codex/agents/reviewer.toml": "name = \"reviewer\"\ndescription = \"Reviews diffs.\"\nmodel = \"gpt-5.5\"\ndeveloper_instructions = \"Review.\"\n"},
		},
		{
			name: "copilot agent", sources: "copilot", claudeFile: ".claude/agents/reviewer.md",
			native: map[string]string{".github/agents/reviewer.agent.md": "---\nname: reviewer\ndescription: Reviews diffs.\nmodel: gpt-4.1\n---\n\nReview.\n"},
		},
		{
			name: "copilot agent over a claude agent without a model", sources: "claude,copilot", claudeFile: ".claude/agents/reviewer.md",
			native: map[string]string{
				".claude/agents/reviewer.md":       "---\nname: reviewer\ndescription: Reviews diffs.\n---\n\nReview.\n",
				".github/agents/reviewer.agent.md": "---\nname: reviewer\ndescription: Reviews diffs.\nmodel: gpt-4.1\n---\n\nReview.\n",
			},
		},
		{
			name: "opencode command", sources: "opencode", claudeFile: ".claude/commands/check.md",
			native: map[string]string{".opencode/commands/check.md": "---\ndescription: Check.\nmodel: gpt-4.1\n---\n\nCheck.\n"},
		},
		{
			name: "gemini settings", sources: "gemini",
			native: map[string]string{".gemini/settings.json": `{"model":{"name":"gemini-2.5-pro"}}`},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+c.sources+"]\n")
			for path, body := range c.native {
				writeFile(t, filepath.Join(dir, path), body)
			}
			execCLI(t, append([]string{"import"}, strings.Split(c.sources, ",")...)...)
			execCLI(t, "use", "claude")

			if c.claudeFile != "" {
				if got := readFile(t, filepath.Join(dir, c.claudeFile)); strings.Contains(got, "model") {
					t.Errorf("%s got another tool's model:\n%s", c.claudeFile, got)
				}
			}
			if raw, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json")); err == nil && strings.Contains(string(raw), "gemini-") {
				t.Errorf(".claude/settings.json got Gemini's model:\n%s", raw)
			}
		})
	}
}

func TestImportGoose_SharedAgentFileSyncsWithOpenHands(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [goose, openhands]\n")
	writeFile(t, filepath.Join(dir, ".agents", "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Reviews diffs.\nmodel: gpt-5.5\n---\n\nReview.\n")

	execCLI(t, "import", "goose")
	execCLI(t, "sync")
	execCLI(t, "sync", "--check")

	if got := readFile(t, filepath.Join(dir, ".agents", "agents", "reviewer.md")); !strings.Contains(got, "model: gpt-5.5") {
		t.Errorf("shared agent file lost its model:\n%s", got)
	}
}
