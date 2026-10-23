package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSyncKimi_DefaultTargetsKeepScopedToolAliases(t *testing.T) {
	for _, field := range []string{"tools: [Read, Bash(git diff *)]", "can: [read, shell(git diff *)]"} {
		t.Run(field, func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\n")
			writeFile(t, ".agnostic-ai/agents/reviewer.md", "---\nname: reviewer\ndescription: Review changes\n"+field+"\n---\nReview changes.\n")
			if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
				t.Fatalf("default sync rejected existing scoped tools: %v\n%s", err, out)
			}
			native := readFile(t, ".claude/agents/reviewer.md")
			if !strings.Contains(native, "Read") || !strings.Contains(native, "Bash") || !strings.Contains(native, "Review changes.") {
				t.Errorf("default target lost the existing agent: %s", native)
			}
			if got := readFile(t, ".opencode/agents/reviewer.md"); !strings.Contains(got, "git diff *") {
				t.Errorf("OpenCode lost the scoped restriction: %s", got)
			}
			if _, err := os.Stat(".kimi-code"); !os.IsNotExist(err) {
				t.Errorf("default sync selected Kimi: %v", err)
			}
			if _, err := runCLI(t, "sync", "-t", "kimi", "--gitignore=off"); err == nil || !strings.Contains(err.Error(), "cannot express scoped tool") {
				t.Errorf("explicit Kimi must retain native restriction guard: %v", err)
			}
			if _, err := os.Stat(".kimi-code/agents/reviewer.md"); !os.IsNotExist(err) {
				t.Errorf("Kimi wrote an unrepresentable agent: %v", err)
			}
			if got := readFile(t, ".claude/agents/reviewer.md"); got != native {
				t.Error("failed explicit Kimi sync changed the existing agent")
			}
		})
	}
}
