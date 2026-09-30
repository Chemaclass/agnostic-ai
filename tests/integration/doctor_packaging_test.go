package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDoctor_PackagingIgnoresGeneratedSkillAssets(t *testing.T) {
	repository, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "agnostic-ai")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, "./cmd/agnostic-ai")
	build.Dir = repository
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v: %s", err, output)
	}
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", t.TempDir())
	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/skills/review"), 0755))
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [codex]\noutputs:\n  codex:\n    skills-dir: native/skills\n"), 0644))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/skills/review/SKILL.md"), []byte("---\nname: review\ndescription: Review code.\n---\nReview.\n"), 0644))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/skills/review/helper.sh"), []byte("echo review\n"), 0644))
	ignore := []byte(".claude/**\n.codex/**\n.agents/**\n")
	must(t, os.WriteFile(filepath.Join(dir, ".vscodeignore"), ignore, 0644))
	sync := exec.Command(bin, "sync")
	sync.Dir = dir
	if output, err := sync.CombinedOutput(); err != nil {
		t.Fatalf("sync: %v: %s", err, output)
	}
	doctor := exec.Command(bin, "doctor")
	doctor.Dir = dir
	output, err := doctor.CombinedOutput()
	if err != nil {
		t.Errorf("packaging advisory failed doctor: %v: %s", err, output)
	}
	for _, text := range []string{".vscodeignore", "native/skills/review/SKILL.md", "native/skills/review/helper.sh"} {
		if !strings.Contains(string(output), text) {
			t.Errorf("warning lacks %s: %s", text, output)
		}
	}
	unchanged, err := os.ReadFile(filepath.Join(dir, ".vscodeignore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != string(ignore) {
		t.Errorf("doctor changed packaging ignore: %s", unchanged)
	}
	must(t, os.WriteFile(filepath.Join(dir, ".vscodeignore"), []byte("**\n"), 0644))
	doctor = exec.Command(bin, "doctor")
	doctor.Dir = dir
	output, err = doctor.CombinedOutput()
	if err != nil {
		t.Errorf("covered paths failed doctor: %v: %s", err, output)
	}
	if strings.Contains(string(output), "Packaging ignores:") {
		t.Errorf("covered paths warned: %s", output)
	}
}
