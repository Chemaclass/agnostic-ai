package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportOpencode_PreservesV2SkillPriority(t *testing.T) {
	binary := builtBinary(t)
	project := t.TempDir()
	files := map[string]string{"agnostic-ai.yaml": "version: 1\ntargets: [opencode]\n"}
	for _, root := range []string{".claude", ".agents", ".opencode"} {
		for _, skill := range []string{"compatibility", "native"} {
			if root == ".opencode" && skill == "compatibility" {
				continue
			}
			path := root + "/skills/" + skill + "/"
			files[path+"SKILL.md"] = "---\nname: " + skill + "\ndescription: Review changes\n---\n" + root + " body\n"
			files[path+"guide.txt"] = root + " guide\n"
		}
	}
	for path, content := range files {
		path = filepath.Join(project, path)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) {
		t.Helper()
		command := exec.Command(binary, args...)
		command.Dir = project
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	run("import", "opencode")
	run("sync")
	run("sync", "--check")
	run("lint")
	for skill, winner := range map[string]string{"compatibility": ".agents", "native": ".opencode"} {
		for _, root := range []string{".agnostic-ai", ".opencode"} {
			for _, file := range []string{"SKILL.md", "guide.txt"} {
				path := filepath.Join(project, root, "skills", skill, file)
				got, err := os.ReadFile(path)
				if err != nil || !strings.Contains(string(got), winner) {
					t.Errorf("%s lost %s winner: %v\n%s", path, winner, err, got)
				}
			}
		}
	}
}
