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
			importHookConfig = &config.Config{Targets: []string{"goose", "openhands", "copilot"}, Models: map[string]config.ModelTier{"fast": {}}}
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

func TestImportGoose_ReimportOfAnEditedSharedAgentModelSyncs(t *testing.T) {
	for name, edited := range map[string]string{
		"changed": "model: gpt-6\n",
		"deleted": "",
	} {
		t.Run(name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [goose, openhands]\n")
			native := filepath.Join(dir, ".agents", "agents", "reviewer.md")
			writeFile(t, native, "---\nname: reviewer\ndescription: Reviews diffs.\nmodel: gpt-5.5\n---\n\nReview.\n")
			execCLI(t, "import", "goose")
			execCLI(t, "sync")

			writeFile(t, native, strings.Replace(readFile(t, native), "model: gpt-5.5\n", edited, 1))
			execCLI(t, "import", "goose")
			execCLI(t, "sync")
			execCLI(t, "sync", "--check")

			got := readFile(t, native)
			if edited == "" && strings.Contains(got, "model:") || edited != "" && !strings.Contains(got, edited) {
				t.Errorf("shared agent file did not follow the edit:\n%s", got)
			}
		})
	}
}

func TestImportOpenCode_ReimportOfAnEditedCommandModelStaysScoped(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [opencode, claude]\n")
	native := filepath.Join(dir, ".opencode", "commands", "check.md")
	writeFile(t, native, "---\ndescription: Check.\nmodel: gpt-4.1\n---\n\nCheck.\n")
	execCLI(t, "import", "opencode")
	execCLI(t, "sync")

	writeFile(t, native, strings.Replace(readFile(t, native), "gpt-4.1", "gpt-5", 1))
	execCLI(t, "import", "opencode")
	execCLI(t, "sync")

	if got := readFile(t, filepath.Join(dir, ".claude", "commands", "check.md")); strings.Contains(got, "model") {
		t.Errorf("claude got OpenCode's model:\n%s", got)
	}
	if got := readFile(t, native); !strings.Contains(got, "gpt-5") {
		t.Errorf("opencode lost its edited model:\n%s", got)
	}
}

func TestImport_CommandModelsFromTwoToolsStayPerTool(t *testing.T) {
	for _, sources := range [][]string{{"claude", "opencode"}, {"opencode", "junie"}} {
		t.Run(strings.Join(sources, "+"), func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+strings.Join(sources, ", ")+"]\n")
			native := map[string]string{
				"claude":   ".claude/commands/check.md",
				"opencode": ".opencode/commands/check.md",
				"junie":    ".junie/commands/check.md",
			}
			model := map[string]string{"claude": "sonnet", "opencode": "gpt-5", "junie": "junie-model"}
			for _, s := range sources {
				writeFile(t, filepath.Join(dir, native[s]), "---\ndescription: Check.\nmodel: "+model[s]+"\n---\n\nCheck.\n")
			}
			execCLI(t, append([]string{"import"}, sources...)...)
			execCLI(t, "sync")

			for _, s := range sources {
				got := readFile(t, filepath.Join(dir, native[s]))
				if !strings.Contains(got, "model: "+model[s]) {
					t.Errorf("%s lost its own model:\n%s", s, got)
				}
				for _, other := range sources {
					if other != s && strings.Contains(got, model[other]) {
						t.Errorf("%s got %s's model:\n%s", s, other, got)
					}
				}
			}
		})
	}
}

func TestImport_SecondToolWithTheSameAgentModelImportsCleanly(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [copilot, cursor]\n")
	writeFile(t, filepath.Join(dir, ".github", "agents", "reviewer.agent.md"), "---\nname: reviewer\ndescription: Reviews diffs.\nmodel: gpt-5\n---\n\nReview.\n")
	writeFile(t, filepath.Join(dir, ".cursor", "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Reviews diffs.\nmodel: gpt-5\n---\n\nReview.\n")

	execCLI(t, "import", "copilot")
	execCLI(t, "import", "cursor")
	execCLI(t, "sync")
	execCLI(t, "sync", "--check")
}
