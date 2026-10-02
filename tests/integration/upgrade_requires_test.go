package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestUpgradeRequires_UpdatesProjectPinsAndSyncs(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	testutil.Chdir(t, project)
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	const configPath = "agnostic-ai.yaml"
	const oldConfig = "# yaml-language-server: $schema=https://raw.githubusercontent.com/Chemaclass/agnostic-ai/v0.76.0/docs/schemas/config.schema.json\n" +
		"# Keep this project note.\nversion: 1\nrequires: \"0.76.0\" # Keep this release note.\n" +
		"targets: [claude]\ngitignore:\n  enabled: true\n"
	must(t, os.WriteFile(configPath, []byte(oldConfig), 0o644))
	must(t, os.WriteFile("package.json", []byte("{\"private\":true,\"devDependencies\":{\"agnostic-ai\":\"0.77.0\"}}\n"), 0o644))
	must(t, os.WriteFile("pnpm-lock.yaml", []byte("lockfileVersion: '9.0'\n"), 0o644))
	rule := filepath.Join(".agnostic-ai", "rules", "release-check.md")
	must(t, os.MkdirAll(filepath.Dir(rule), 0o755))
	must(t, os.WriteFile(rule, []byte("---\nname: release-check\n---\nUpdated portable guidance.\n"), 0o644))

	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(project, "node_modules", "agnostic-ai", "bin", name)
	must(t, os.MkdirAll(filepath.Dir(binary), 0o755))
	build := exec.Command("go", "build", "-ldflags",
		"-X github.com/chemaclass/agnostic-ai/internal/cli.candidateVersion=0.77.0 -X main.version=0.77.0",
		"-o", binary, "./cmd/agnostic-ai")
	build.Dir = filepath.Join(packageDir, "..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build released CLI candidate: %v\n%s", err, out)
	}
	run := func(args ...string) (string, error) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = project
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	out, err := run("sync")
	if err == nil {
		t.Fatal("sync accepted the old exact requirement with the newer installed release")
	}
	for _, want := range []string{"AAI-005", "upgrade --requires"} {
		if !strings.Contains(out, want) {
			t.Errorf("requires failure lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "upgrade --version v0.76.0") {
		t.Errorf("requires failure suggests downgrading the installed package:\n%s", out)
	}

	if out, err := run("upgrade", "--requires"); err != nil {
		t.Fatalf("upgrade --requires: %v\n%s", err, out)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Requires string `yaml:"requires"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Requires != "0.77.0" {
		t.Errorf("requires = %q, want 0.77.0", cfg.Requires)
	}
	assertContains(t, configPath,
		"https://raw.githubusercontent.com/Chemaclass/agnostic-ai/v0.77.0/docs/schemas/config.schema.json",
		"# Keep this project note.", "# Keep this release note.")
	assertContains(t, filepath.Join(".claude", "rules", "release-check.md"), "Updated portable guidance.")
	if out, err := run("sync", "--check"); err != nil {
		t.Fatalf("sync --check after upgrading project pins: %v\n%s", err, out)
	}
}
