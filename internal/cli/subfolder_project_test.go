package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// projectWithSubfolder makes a project and moves into sub/deep inside it,
// returning the project root as the walk reports it.
func projectWithSubfolder(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	deep := filepath.Join(dir, "sub", "deep")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, deep)
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestErrorText_MissingConfigInsideAProjectNamesItInsteadOfInit(t *testing.T) {
	root := projectWithSubfolder(t)

	_, err := runCLI(t, "status")
	if err == nil {
		t.Fatal("status passed in a folder with no config")
	}
	text := ErrorText(err)
	if !strings.Contains(text, root) {
		t.Errorf("error does not name the project at %s:\n%s", root, text)
	}
	if strings.Contains(text, "agnostic-ai init") {
		t.Errorf("error suggests init inside an existing project:\n%s", text)
	}
}

func TestErrorText_MissingConfigOutsideAProjectStillSuggestsInit(t *testing.T) {
	testutil.TempCwd(t)

	_, err := runCLI(t, "status")
	if err == nil {
		t.Fatal("status passed in a folder with no config")
	}
	if text := ErrorText(err); !strings.Contains(text, "agnostic-ai init") {
		t.Errorf("error outside a project lost the init hint:\n%s", text)
	}
}

func TestInit_RefusesInsideAnExistingProject(t *testing.T) {
	root := projectWithSubfolder(t)

	cmd := NewRootCmd("test")
	cmd.SetIn(strings.NewReader("claude\n"))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"init"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "inside the agnostic-ai project at "+root) {
		t.Fatalf("init in a subfolder: err = %v\n%s", err, out.String())
	}
	if _, statErr := os.Stat("agnostic-ai.yaml"); !os.IsNotExist(statErr) {
		t.Errorf("init wrote a nested config: %v", statErr)
	}
}

func TestPacksList_InsideAProjectNamesItWhenNoPacksAreHere(t *testing.T) {
	root := projectWithSubfolder(t)

	out, err := runCLI(t, "packs", "list")
	if err != nil {
		t.Fatalf("packs list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "inside the agnostic-ai project at "+root) {
		t.Errorf("packs list did not name the enclosing project:\n%s", out)
	}
}

func TestDoctor_InsideAProjectNamesItInsteadOfInit(t *testing.T) {
	root := projectWithSubfolder(t)

	out, err := runCLI(t, "doctor", "--checks", "project")
	if err == nil {
		t.Fatalf("doctor passed in a folder with no config:\n%s", out)
	}
	if !strings.Contains(out, root) || strings.Contains(out, "agnostic-ai init") {
		t.Errorf("doctor should name %s and not suggest init:\n%s", root, out)
	}
}

func TestPacksUpdate_InsideAProjectNamesItWhenNoPacksAreHere(t *testing.T) {
	root := projectWithSubfolder(t)

	out, err := runCLI(t, "packs", "update")
	if err != nil {
		t.Fatalf("packs update: %v\n%s", err, out)
	}
	if !strings.Contains(out, "inside the agnostic-ai project at "+root) {
		t.Errorf("packs update did not name the enclosing project:\n%s", out)
	}
}

func TestErrorText_MissingConfigInAnotherFolderKeepsTheRegistryFix(t *testing.T) {
	projectWithSubfolder(t)
	elsewhere := t.TempDir()

	_, _, err := config.ResolveConfigPath(elsewhere)
	if err == nil {
		t.Fatal("an empty folder has a config")
	}
	if text := ErrorText(err); !strings.Contains(text, "agnostic-ai init") || strings.Contains(text, "inside the agnostic-ai project") {
		t.Errorf("a missing config outside the working directory got the subfolder hint:\n%s", text)
	}
}

func TestInit_StartsAProjectUnderAUserSourceThatIsTheHomeDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AGNOSTIC_AI_HOME", home)
	writeFile(t, filepath.Join(home, "agnostic-ai.yaml"), "version: 1\n")
	work := filepath.Join(home, "code", "new")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, work)

	cmd := NewRootCmd("test")
	cmd.SetIn(strings.NewReader("claude\n"))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"init"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init under the global home: %v\n%s", err, out.String())
	}
	if _, err := os.Stat("agnostic-ai.yaml"); err != nil {
		t.Errorf("init wrote no config: %v", err)
	}
}

func TestEnclosingProject_IgnoresGitLocationVariables(t *testing.T) {
	root := projectWithSubfolder(t)
	gitInit(t, root)
	t.Setenv("GIT_DIR", filepath.Join(root, ".git"))
	t.Setenv("GIT_WORK_TREE", filepath.Join(root, "sub", "deep"))

	if dir, ok := enclosingProject("."); !ok || dir != root {
		t.Errorf("enclosingProject under a git hook's variables = %q, %v; want %s", dir, ok, root)
	}
}

func TestImport_FromASubfolderFailsWithoutWritingThere(t *testing.T) {
	root := projectWithSubfolder(t)

	out, err := runCLI(t, "import", "claude")
	if err == nil {
		t.Fatalf("import passed in a folder with no config:\n%s", out)
	}
	text := ErrorText(err)
	if !strings.Contains(text, root) || strings.Contains(text, "agnostic-ai init") {
		t.Errorf("import error should name %s and not suggest init:\n%s", root, text)
	}
	for _, path := range []string{".agnostic-ai", ".gitignore"} {
		if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
			t.Errorf("import left %s in the subfolder", path)
		}
	}
}

func TestInsideAProject_EveryCommandUsesOneSentence(t *testing.T) {
	for _, tc := range []struct {
		args []string
		run  string
	}{
		{[]string{"status"}, "the command"},
		{[]string{"init"}, "the command"},
		{[]string{"doctor", "--checks", "project"}, "agnostic-ai doctor"},
		{[]string{"packs", "list"}, "agnostic-ai packs list"},
		{[]string{"use", "codex"}, "agnostic-ai use"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			root := projectWithSubfolder(t)
			cmd := NewRootCmd("test")
			cmd.SetIn(strings.NewReader("claude\n"))
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			text := out.String()
			if err != nil {
				text += ErrorText(err)
			}
			want := "this directory is inside the agnostic-ai project at " + root + "; run " + tc.run + " from there"
			if !strings.Contains(text, want) {
				t.Errorf("want %q in:\n%s", want, text)
			}
		})
	}
}
