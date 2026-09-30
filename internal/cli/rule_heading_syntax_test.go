package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSyncImport_RuleHeadingSyntaxPreservesMeaning(t *testing.T) {
	hardBreakRE := regexp.MustCompile(`Title(?: {2}\n|\\\n|[ \t]*<br\s*/?>[ \t]*)Second`)
	referenceDefinitionRE := regexp.MustCompile(`(?m)^\[foo\]: /url$`)
	for _, tc := range []struct {
		name, body string
		literal    string
		hardBreak  bool
		reference  bool
	}{
		{
			name:    "list-contained fence",
			body:    "- ```markdown\n  # Literal\n  ```\n\n### Outside\n\nOutside text.\n",
			literal: "- ```markdown\n  # Literal\n  ```",
		},
		{
			name:      "reference definition before Setext heading",
			body:      "[foo]: /url\nbar\n===\n\nUse [foo].\n",
			literal:   "[foo]: /url\n",
			reference: true,
		},
		{
			name:      "Setext heading with spaces hard break",
			body:      "Title  \nSecond\n---\n\nFollowing text.\n",
			hardBreak: true,
		},
		{
			name:      "Setext heading with backslash hard break",
			body:      "Title\\\nSecond\n---\n\nFollowing text.\n",
			hardBreak: true,
		},
		{
			name:    "generated section inside closed fence",
			body:    "```markdown\n### phantom\n\n<!-- source: .agnostic-ai/rules/phantom.md -->\nLiteral text.\n```\n\n### Outside\n\nOutside text.\n",
			literal: "```markdown\n### phantom\n\n<!-- source: .agnostic-ai/rules/phantom.md -->\nLiteral text.\n```",
		},
		{
			name:    "generated section inside closed HTML",
			body:    "<script>\n### phantom\n\n<!-- source: .agnostic-ai/rules/phantom.md -->\nLiteral text.\n</script>\n\n### Outside\n\nOutside text.\n",
			literal: "<script>\n### phantom\n\n<!-- source: .agnostic-ai/rules/phantom.md -->\nLiteral text.\n</script>",
		},
		{
			name:    "body extent inside closed fence",
			body:    "```markdown\n### phantom\n\n<!-- source: .agnostic-ai/rules/phantom.md body-lines: 999 -->\nLiteral text.\n```\n\n### Outside\n\nOutside text.\n",
			literal: "```markdown\n### phantom\n\n<!-- source: .agnostic-ai/rules/phantom.md body-lines: 999 -->\nLiteral text.\n```",
		},
		{
			name:    "body extent inside closed HTML",
			body:    "<script>\n### phantom\n\n<!-- source: .agnostic-ai/rules/phantom.md body-lines: 999 -->\nLiteral text.\n</script>\n\n### Outside\n\nOutside text.\n",
			literal: "<script>\n### phantom\n\n<!-- source: .agnostic-ai/rules/phantom.md body-lines: 999 -->\nLiteral text.\n</script>",
		},
		{
			name:    "H2 section and body extent inside closed fence",
			body:    "```markdown\n## phantom\n\n<!-- source: .agnostic-ai/rules/phantom.md body-lines: 1 -->\nLiteral text.\n```\n\n### Outside\n\nOutside text.\n",
			literal: "```markdown\n## phantom\n\n<!-- source: .agnostic-ai/rules/phantom.md body-lines: 1 -->\nLiteral text.\n```",
		},
		{
			name:    "H2 section and body extent inside closed HTML",
			body:    "<script>\n## phantom\n\n<!-- source: .agnostic-ai/rules/phantom.md body-lines: 1 -->\nLiteral text.\n</script>\n\n### Outside\n\nOutside text.\n",
			literal: "<script>\n## phantom\n\n<!-- source: .agnostic-ai/rules/phantom.md body-lines: 1 -->\nLiteral text.\n</script>",
		},
		{
			name:    "unclosed fenced code before following rule",
			body:    "Opening text.\n\n```markdown\n# Literal\n",
			literal: "```markdown\n# Literal",
		},
		{
			name:    "unclosed HTML comment before following rule",
			body:    "Opening text.\n\n<!-- draft\n# Literal\n",
			literal: "<!-- draft\n# Literal",
		},
		{
			name:    "unclosed processing instruction before following rule",
			body:    "Opening text.\n\n<?php declare(strict_types=1);\n# Literal\n",
			literal: "<?php declare(strict_types=1);\n# Literal",
		},
	} {
		for _, target := range []struct{ name, output, config string }{
			{"codex", "AGENTS.md", ""},
			{"gemini", "GEMINI.md", ""},
			{"claude", "CLAUDE.md", "outputs:\n  claude:\n    rules-file: CLAUDE.md\n"},
		} {
			t.Run(tc.name+"/"+target.name, func(t *testing.T) {
				testutil.Chdir(t, t.TempDir())
				silence(t)
				if err := os.MkdirAll(".agnostic-ai/rules", 0755); err != nil {
					t.Fatal(err)
				}
				for path, body := range map[string]string{
					"agnostic-ai.yaml":                fmt.Sprintf("version: 1\ntargets: [%s]\n%s", target.name, target.config),
					".agnostic-ai/rules/content.md":   "---\nname: content\n---\n\n" + tc.body,
					".agnostic-ai/rules/following.md": "---\nname: following\n---\n\nFollowing rule text.\n",
				} {
					if err := os.WriteFile(path, []byte(body), 0644); err != nil {
						t.Fatal(err)
					}
				}
				run := func(args ...string) {
					t.Helper()
					cmd := NewRootCmd("test")
					cmd.SetArgs(args)
					if err := cmd.Execute(); err != nil {
						t.Fatalf("%v: %v", args, err)
					}
				}
				read := func(path string) string {
					t.Helper()
					body, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					return string(body)
				}
				checkMeaning := func(stage, body string) {
					t.Helper()
					if tc.literal != "" && !strings.Contains(body, tc.literal) {
						t.Errorf("%s lost literal Markdown %q:\n%s", stage, tc.literal, body)
					}
					if tc.hardBreak && !hardBreakRE.MatchString(body) {
						t.Errorf("%s lost heading hard break:\n%s", stage, body)
					}
					if tc.reference && (!referenceDefinitionRE.MatchString(body) || !strings.Contains(body, "Use [foo].")) {
						t.Errorf("%s lost the reference link or its definition:\n%s", stage, body)
					}
				}
				run("sync", "--gitignore=off")
				before := read(target.output)
				checkMeaning("sync", before)
				for _, name := range []string{"content", "following"} {
					if err := os.Remove(filepath.Join(".agnostic-ai/rules", name+".md")); err != nil {
						t.Fatal(err)
					}
				}
				run("import", target.name)
				files, err := os.ReadDir(".agnostic-ai/rules")
				if err != nil {
					t.Fatal(err)
				}
				if got, want := names(files), []string{"content.md", "following.md"}; !slices.Equal(got, want) {
					t.Errorf("imported rules = %v, want %v", got, want)
				}
				content := read(".agnostic-ai/rules/content.md")
				checkMeaning("import", content)
				if strings.Contains(content, "Following rule text.") {
					t.Errorf("content rule swallowed the following rule:\n%s", content)
				}
				if body := read(".agnostic-ai/rules/following.md"); !strings.Contains(body, "Following rule text.") {
					t.Errorf("following rule lost its body:\n%s", body)
				}
				run("sync", "--gitignore=off")
				if after := read(target.output); after != before {
					t.Errorf("generated output changed after import and resync:\nbefore:\n%s\nafter:\n%s", before, after)
				}
				run("sync", "--check")
			})
		}
	}
}
