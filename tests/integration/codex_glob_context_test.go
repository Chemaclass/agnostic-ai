package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestCodexGlobContext_SyncKeepsRootContextSmall(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = filepath.Join(packageDir, "..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	dir := t.TempDir()
	for name, body := range map[string]string{
		"agnostic-ai.yaml":             "version: 1\ntargets: [codex]\n",
		".agnostic-ai/AGNOSTIC_AI.md":  "# Project\n\nRoot guidance.\n",
		".agnostic-ai/rules/api.md":    "---\nname: api\nglobs: [src/app/api/**, prisma/**]\n---\nAPI and database conventions.\n",
		".agnostic-ai/rules/deploy.md": "---\nname: deploy\nglobs: [Dockerfile, scripts/*.sh, .kamal/**]\n---\nDeployment conventions.\n",
	} {
		must(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755))
		must(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0644))
	}
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	out := run("sync", "--gitignore=off")
	if !strings.Contains(out, `rule "deploy" is always loaded from AGENTS.md`) {
		t.Errorf("missing named fallback: %s", out)
	}
	assertNoFileContains(t, filepath.Join(dir, "AGENTS.md"), "API and database conventions.")
	assertContains(t, filepath.Join(dir, "AGENTS.md"), "Root guidance.", "Deployment conventions.")
	for _, path := range []string{"src/app/api/AGENTS.md", "prisma/AGENTS.md"} {
		assertContains(t, filepath.Join(dir, path), "API and database conventions.")
	}
	if _, err := os.Stat(filepath.Join(dir, ".kamal/AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("mixed selector was silently narrowed: %v", err)
	}
	run("sync", "--check", "--gitignore=off")
	testutil.AssertGoldenTree(t, dir, filepath.Join(packageDir, "fixtures", "codex-glob-context"), "agnostic-ai.yaml")
}
