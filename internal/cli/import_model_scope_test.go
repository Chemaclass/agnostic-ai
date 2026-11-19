package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestScopeImportedModel(t *testing.T) {
	cases := []struct {
		name, target, model, want string
	}{
		{"vendor model", "copilot", "gpt-4", "model: {copilot: gpt-4}\n"},
		{"inherit stays portable", "copilot", "inherit", "model: inherit\n"},
		{"tier stays portable", "copilot", "fast", "model: fast\n"},
		{"claude scopes its own names", "claude", "gpt-4", "model: gpt-4\n"},
		{"no source", "", "gpt-4", "model: gpt-4\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			priorTarget, priorCfg := importScopeTarget, importHookConfig
			t.Cleanup(func() { importScopeTarget, importHookConfig = priorTarget, priorCfg })
			importScopeTarget = c.target
			importHookConfig = &config.Config{Models: map[string]config.ModelTier{"fast": {}}}

			got := string(scopeImportedModel([]byte("---\nname: rev\nmodel: " + c.model + "\n---\n\nBody.\n")))
			if want := "---\nname: rev\n" + c.want + "---\n\nBody.\n"; got != want {
				t.Errorf("got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestImportThenUseClaude_KeepsAnotherToolsModelOutOfClaude(t *testing.T) {
	cases := []struct {
		name, source string
		native       map[string]string
	}{
		{"codex agent", "codex", map[string]string{".codex/agents/reviewer.toml": "name = \"reviewer\"\ndescription = \"Reviews diffs.\"\nmodel = \"gpt-5.5\"\ndeveloper_instructions = \"Review.\"\n"}},
		{"gemini settings", "gemini", map[string]string{".gemini/settings.json": `{"model":{"name":"gemini-2.5-pro"}}`}},
		{"copilot agent", "copilot", map[string]string{".github/agents/reviewer.agent.md": "---\nname: reviewer\ndescription: Reviews diffs.\nmodel: gpt-4.1\n---\n\nReview.\n"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+c.source+"]\n")
			for path, body := range c.native {
				writeFile(t, filepath.Join(dir, path), body)
			}
			execCLI(t, "import", c.source)
			execCLI(t, "use", "claude")

			for _, path := range []string{".claude/agents/reviewer.md", ".claude/settings.json"} {
				raw, _ := os.ReadFile(filepath.Join(dir, path))
				if got := string(raw); strings.Contains(got, "gpt-") || strings.Contains(got, "gemini-") {
					t.Errorf("%s got another tool's model:\n%s", path, got)
				}
			}
		})
	}
}
