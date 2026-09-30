package cli

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestImportFromCodex_ScopesAgentModelToCodex(t *testing.T) {
	cases := []struct {
		name        string
		claudeModel string
		want        map[string]any
	}{
		{"claude has no model", "", map[string]any{"codex": "gpt-5.5"}},
		{"claude model alias", "model: sonnet\n", map[string]any{"claude": "sonnet", "codex": "gpt-5.5"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, ".claude", "agents", "reviewer.md"),
				"---\nname: reviewer\ndescription: Reviews diffs.\n"+c.claudeModel+"---\n\nReview.\n")
			writeFile(t, filepath.Join(dir, ".codex", "agents", "reviewer.toml"),
				"name = \"reviewer\"\ndescription = \"Reviews diffs.\"\nmodel = \"gpt-5.5\"\ndeveloper_instructions = \"Review.\"\n")
			if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
				t.Fatal(err)
			}
			if err := importFromCodex(dir, rootSources()); err != nil {
				t.Fatal(err)
			}
			fm := agentFrontmatter(t, filepath.Join(dir, "agents", "reviewer.md"))
			if !reflect.DeepEqual(fm["model"], c.want) {
				t.Errorf("model = %#v, want %#v", fm["model"], c.want)
			}
			if xcodex, _ := fm["x-codex"].(map[string]any); xcodex != nil {
				if _, set := xcodex["model"]; set {
					t.Errorf("x-codex.model must stay unset: %#v", xcodex)
				}
			}
		})
	}
}

func TestMergeCodexAgentIntoExisting_Model(t *testing.T) {
	cases := []struct {
		name, existing string
		wantModel      any
		wantXCodex     any
	}{
		{"per-target map with codex keeps it", "model: {claude: opus, codex: gpt-5}\n", map[string]any{"claude": "opus", "codex": "gpt-5"}, nil},
		{"per-target map with default adds codex", "model: {default: gpt-5}\n", map[string]any{"default": "gpt-5", "codex": "gpt-5.5"}, nil},
		{"shared value that differs", "model: sonnet\n", "sonnet", "gpt-5.5"},
		{"shared value that matches", "model: gpt-5.5\n", "gpt-5.5", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			existing := "---\nname: reviewer\n" + c.existing + "---\n\nReview.\n"
			out, err := mergeCodexAgentIntoExisting(existing, "reviewer", map[string]any{"model": "gpt-5.5"})
			if err != nil {
				t.Fatal(err)
			}
			fm := parseAgentFrontmatter(t, out)
			if !reflect.DeepEqual(fm["model"], c.wantModel) {
				t.Errorf("model = %#v, want %#v", fm["model"], c.wantModel)
			}
			xcodex, _ := fm["x-codex"].(map[string]any)
			if got := xcodex["model"]; !reflect.DeepEqual(got, c.wantXCodex) {
				t.Errorf("x-codex.model = %#v, want %#v", got, c.wantXCodex)
			}
		})
	}
}

func agentFrontmatter(t *testing.T, path string) map[string]any {
	t.Helper()
	return parseAgentFrontmatter(t, readFile(t, path))
}

func parseAgentFrontmatter(t *testing.T, doc string) map[string]any {
	t.Helper()
	rest, ok := strings.CutPrefix(doc, "---\n")
	if !ok {
		t.Fatalf("no frontmatter:\n%s", doc)
	}
	front, _, ok := strings.Cut(rest, "\n---")
	if !ok {
		t.Fatalf("unterminated frontmatter:\n%s", doc)
	}
	fm := map[string]any{}
	if err := yaml.Unmarshal([]byte(front), &fm); err != nil {
		t.Fatal(err)
	}
	return fm
}
