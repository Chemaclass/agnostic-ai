package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func runImportSummary(t *testing.T, args ...string) string {
	t.Helper()
	buf := captureSummary(t)
	root := NewRootCmd("test")
	root.SetArgs(append([]string{"import"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("import %v: %v", args, err)
	}
	return buf.String()
}

func TestImportAll_AfterSyncKeepsAClaudePathsRuleSingle(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex]\n")
	writeFile(t, filepath.Join(dir, ".claude", "rules", "api.md"), "---\npaths:\n  - src/api/**\n---\n\nReturn typed errors.\n")
	execCLI(t, "import", "claude")
	execCLI(t, "sync")

	execCLI(t, "import", "all")

	entries, _ := os.ReadDir(filepath.Join(dir, ".agnostic-ai", "rules"))
	for _, e := range entries {
		if e.Name() != "api.md" {
			t.Errorf("import all after sync wrote %s", e.Name())
		}
	}
}

func colonCommandProject(t *testing.T, extra map[string]string) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, gemini]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "commands", "git-commit.md"), "---\nname: git:commit\ndescription: Commit\n---\n\nWrite a commit message.\n")
	for path, body := range extra {
		writeFile(t, filepath.Join(dir, path), body)
	}
	execCLI(t, "sync")
	return dir
}

func TestImportClaude_EditedFlatCommandKeepsItsNamespacedName(t *testing.T) {
	dir := colonCommandProject(t, nil)
	native := filepath.Join(dir, ".claude", "commands", "git-commit.md")
	writeFile(t, native, strings.Replace(readFile(t, native), "Write a commit message.", "Write a short commit message.", 1))

	execCLI(t, "import", "claude")
	execCLI(t, "sync")

	got := readFile(t, filepath.Join(dir, ".agnostic-ai", "commands", "git-commit.md"))
	if !strings.Contains(got, "name: git:commit") || !strings.Contains(got, "short commit message") {
		t.Errorf("spec must keep its name and take the edit:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gemini", "commands", "git", "commit.toml")); err != nil {
		t.Errorf("gemini command lost: %v", err)
	}
}

func TestImportClaude_AfterSyncWithAColonCommandSaysNoSpecChanged(t *testing.T) {
	colonCommandProject(t, nil)
	if out := runImportSummary(t, "claude"); !strings.Contains(out, "no spec changed") {
		t.Errorf("a clean re-import must say no spec changed:\n%s", out)
	}
}

func TestImportClaude_EditToANumberedFlatCommandReachesItsSpec(t *testing.T) {
	dir := colonCommandProject(t, map[string]string{
		".agnostic-ai/commands/commit.md": "---\nname: git-commit\ndescription: Plain\n---\n\nPlain commit.\n",
	})
	native := filepath.Join(dir, ".claude", "commands", "git-commit-2.md")
	writeFile(t, native, strings.Replace(readFile(t, native), "Write a commit message.", "Write a short commit message.", 1))

	execCLI(t, "import", "claude")

	if got := readFile(t, filepath.Join(dir, ".agnostic-ai", "commands", "git-commit.md")); !strings.Contains(got, "short commit message") || !strings.Contains(got, "name: git:commit") {
		t.Errorf("the namespaced spec must take the edit:\n%s\nnative:\n%s", got, readFile(t, native))
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "commands", "git-commit-2.md")); err == nil {
		t.Error("the edit must not become a new spec")
	}
}

func legacyWindsurfRulesProject(t *testing.T) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [windsurf]\n")
	writeFile(t, filepath.Join(dir, ".windsurf", "rules", "agent-reviewer.md"), "Review carefully.\n")
	execCLI(t, "import", "windsurf")
	execCLI(t, "sync")
	return dir
}

func TestImportWindsurf_HandWrittenAgentPrefixStaysARuleAfterSync(t *testing.T) {
	dir := legacyWindsurfRulesProject(t)
	execCLI(t, "import", "windsurf")
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md")); err == nil {
		t.Error("the rule agent-reviewer became an agent after sync")
	}
}

func TestImportWindsurf_DeleteNoteRepeatsAfterSync(t *testing.T) {
	legacyWindsurfRulesProject(t)
	if out := runImportSummary(t, "windsurf"); !strings.Contains(out, "delete .windsurf/rules/agent-reviewer.md") {
		t.Errorf("the delete note must repeat while the file stays:\n%s", out)
	}
}
