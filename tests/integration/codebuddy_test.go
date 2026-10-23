package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestCodebuddy_NativeImportRecreatesAnEmptyOutputTree(t *testing.T) {
	repository, err := filepath.Abs("../..")
	must(t, err)
	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = repository
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	project := testutil.TempCwd(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	t.Setenv("AGNOSTIC_AI_NO_UPDATE_CHECK", "1")
	files := map[string]string{
		"agnostic-ai.yaml":                             "version: 1\ntargets: [codebuddy]\noutputs:\n  codebuddy:\n    root-mcp-file: .mcp.json\n",
		"CODEBUDDY.md":                                 "# Team\n\nReview every change.\n",
		"backend/CODEBUDDY.md":                         "Keep backend changes local.\n",
		".codebuddy/rules/go.md":                       "---\nalwaysApply: false\npaths: ['**/*.go']\n---\n\nCheck errors.\n",
		".codebuddy/agents/reviewer.md":                "---\nname: reviewer\ndescription: Review code\ntools: Read, Grep\neffort: high\nmaxTurns: 3\n---\n\nReview changes.\n",
		".codebuddy/skills/check/SKILL.md":             "---\nname: check\ndescription: Check changes\nallowed-tools: Read\ncontext: fork\nmodel: lite\n---\n\nRead references/guide.txt.\n",
		".codebuddy/skills/check/references/guide.txt": "Read the entire diff.\n",
		".codebuddy/commands/check.md":                 "---\ndescription: Check changes\nargument-hint: files\n---\n\nCheck $ARGUMENTS.\n",
		".codebuddy/settings.json":                     `{"nativeControl":true,"model":"lite","reasoningEffort":"high","permissions":{"defaultMode":"plan","deny":["Bash(rm *)"]},"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo guard","timeout":5}]}]}}`,
		".mcp.json":                                    `{"mcpServers":{"docs":{"type":"http","url":"https://docs.example.com/mcp","defer_loading":true}}}`,
	}
	for path, body := range files {
		must(t, os.MkdirAll(filepath.Dir(path), 0755))
		must(t, os.WriteFile(path, []byte(body), 0644))
	}
	run := func(args ...string) {
		t.Helper()
		command := exec.Command(binary, args...)
		command.Dir = project
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	run("import", "codebuddy")
	stash := t.TempDir()
	for i, path := range []string{".codebuddy", "CODEBUDDY.md", "backend/CODEBUDDY.md", ".mcp.json"} {
		must(t, os.Rename(path, filepath.Join(stash, []string{"native", "root", "scoped", "mcp"}[i])))
	}
	run("sync")
	run("sync", "--check")
	run("lint")
	for path, want := range map[string]string{"CODEBUDDY.md": "Review every change.", "backend/CODEBUDDY.md": "Keep backend changes local.", ".codebuddy/rules/go.md": "alwaysApply: false", ".codebuddy/agents/reviewer.md": "tools: Read, Grep", ".codebuddy/skills/check/SKILL.md": "model: lite", ".codebuddy/skills/check/references/guide.txt": "Read the entire diff.", ".codebuddy/commands/check.md": "Check $ARGUMENTS.", ".codebuddy/settings.json": "nativeControl", ".mcp.json": "defer_loading"} {
		assertContains(t, path, want)
	}
}
