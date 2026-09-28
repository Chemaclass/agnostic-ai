package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func runInitCapturingStdout(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var err error
	out := captureStdout(t, func() {
		root := NewRootCmd("test")
		root.SetIn(strings.NewReader(""))
		root.SetErr(&strings.Builder{})
		root.SetArgs(append([]string{"init"}, args...))
		err = root.Execute()
	})
	return out, err
}

func TestInitFromDryRun_ListsWhatTheImportWouldWrite(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	captureLogOut(t)
	writeScopedFile(t, ".claude/skills/demo/SKILL.md", "---\nname: demo\ndescription: Demo.\n---\n\nBody.\n")

	out, err := runInitCapturingStdout(t, "--from", "claude", "--dry-run")
	if err != nil {
		t.Fatalf("init --from claude --dry-run: %v", err)
	}
	for _, want := range []string{"create: agnostic-ai.yaml", "would write " + filepath.FromSlash(".agnostic-ai/skills/demo/SKILL.md")} {
		if !strings.Contains(out, want) {
			t.Errorf("output should contain %q, got:\n%s", want, out)
		}
	}
	for _, p := range []string{"agnostic-ai.yaml", ".agnostic-ai"} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("dry run left %s on disk (stat err: %v)", p, err)
		}
	}
}

// A real init refuses an existing project, so its preview does too.
func TestInitFromDryRun_RefusesAnExistingProject(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	captureLogOut(t)
	if err := os.WriteFile("agnostic-ai.yaml", []byte("version: 1\ntargets: [claude]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := runInitCapturingStdout(t, "--from", "claude", "--dry-run")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v, want the existing agnostic-ai.yaml named", err)
	}
}
