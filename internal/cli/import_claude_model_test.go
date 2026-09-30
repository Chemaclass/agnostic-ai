package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportFromClaude_ScopesClaudeAgentModelToClaude(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude", "agents", "reviewer.md"),
		"---\nname: reviewer\ndescription: Reviews diffs.\nmodel: sonnet\n---\n\nReview.\n")
	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "agents", "reviewer.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := "---\nname: reviewer\ndescription: Reviews diffs.\nmodel: {claude: sonnet}\n---\n\nReview.\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestScopeClaudeModel(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"alias", "---\nmodel: opus\n---\nBody.\n", "---\nmodel: {claude: opus}\n---\nBody.\n"},
		{"inherit", "---\nmodel: inherit\n---\n", "---\nmodel: {claude: inherit}\n---\n"},
		{"claude id", "---\nmodel: claude-sonnet-4-6\n---\n", "---\nmodel: {claude: claude-sonnet-4-6}\n---\n"},
		{"quoted with comment", "---\nmodel: \"haiku\" # cheap\n---\n", "---\nmodel: {claude: \"haiku\"} # cheap\n---\n"},
		{"flow indicator in value", "---\nmodel: claude-opus-5[1m]\n---\n", "---\nmodel:\n  claude: claude-opus-5[1m]\n---\n"},
		{"other vendor model", "---\nmodel: gpt-5.5\n---\n", "---\nmodel: gpt-5.5\n---\n"},
		{"already a map", "---\nmodel: {claude: sonnet, default: gpt-5.5}\n---\n", "---\nmodel: {claude: sonnet, default: gpt-5.5}\n---\n"},
		{"nested model key", "---\nx-codex:\n  model: sonnet\n---\n", "---\nx-codex:\n  model: sonnet\n---\n"},
		{"no frontmatter", "model: sonnet\n", "model: sonnet\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := scopeClaudeModel(c.in); got != c.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, c.want)
			}
		})
	}
}
