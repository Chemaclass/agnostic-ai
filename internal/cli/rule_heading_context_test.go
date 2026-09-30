package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSync_RuleHeadingsNestInCodexAndGeminiRootContext(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	if err := os.MkdirAll(".agnostic-ai/rules", 0755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		"agnostic-ai.yaml":                    "version: 1\ntargets: [codex, gemini]\n",
		".agnostic-ai/rules/content.md":       "---\nname: content\ndescription: Content rules\n---\nTop text.\n\n### Doc versioning\n\nVersioning text.\n\n#### Version details\n\n```markdown\n### Code heading\n```\n",
		".agnostic-ai/rules/working-style.md": "---\nname: working-style\n---\nStyle text.\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := NewRootCmd("test")
	cmd.SetArgs([]string{"sync"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"AGENTS.md", "GEMINI.md"} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"### content\n", "#### Doc versioning\n", "##### Version details\n", "### working-style\n", "```markdown\n### Code heading\n```"} {
			if !strings.Contains(string(body), want) {
				t.Errorf("%s lacks %q:\n%s", path, want, body)
			}
		}
		if strings.Contains(string(body), "\n### Doc versioning\n") {
			t.Errorf("subsection is a sibling rule in %s", path)
		}
	}
	cmd = NewRootCmd("test")
	cmd.SetArgs([]string{"sync", "--check"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}
