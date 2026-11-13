package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestGolden_AgentsAsSkills(t *testing.T) {
	binary := builtBinary(t)
	packageDir, err := os.Getwd()
	must(t, err)
	for _, target := range []string{"amp", "crush", "warp", "zed"} {
		t.Run(target, func(t *testing.T) {
			project := t.TempDir()
			write := func(path, body string) {
				t.Helper()
				path = filepath.Join(project, path)
				must(t, os.MkdirAll(filepath.Dir(path), 0o755))
				must(t, os.WriteFile(path, []byte(body), 0o644))
			}
			write("agnostic-ai.yaml", "version: 1\ntargets: ["+target+"]\nbuiltins: []\noutputs:\n  "+target+":\n    agents: skill\n")
			const agent = "---\nname: reviewer\ndescription: Reviews diffs.\n---\n\nReview the diff.\n"
			write(".agnostic-ai/agents/reviewer.md", agent)
			run := func(args ...string) {
				t.Helper()
				cmd := exec.Command(binary, args...)
				cmd.Dir = project
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%v: %v\n%s", args, err, out)
				}
			}
			run("sync", "--gitignore=off")
			testutil.AssertGoldenTree(t, filepath.Join(project, ".agents", "skills"),
				filepath.Join(packageDir, "fixtures", "golden", "agents-as-skills", target))
			run("import", target)
			if _, err := os.Stat(filepath.Join(project, ".agnostic-ai", "skills", "reviewer")); !os.IsNotExist(err) {
				t.Errorf("import must leave the inline skill to the agent spec, stat err = %v", err)
			}
			got, err := os.ReadFile(filepath.Join(project, ".agnostic-ai", "agents", "reviewer.md"))
			must(t, err)
			if string(got) != agent {
				t.Errorf("import changed the agent spec:\n%s", got)
			}
			run("sync", "--check", "--gitignore=off")
			run("lint")
		})
	}
}
