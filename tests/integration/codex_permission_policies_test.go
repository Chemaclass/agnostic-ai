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

func TestCodexPermissionPolicies_SyncCheckUnsupportedAndLint(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "agnostic-ai")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	t.Setenv("CODEX_HOME", t.TempDir())
	run := func(args ...string) (string, error) {
		command := exec.Command(binary, args...)
		command.Dir = dir
		output, err := command.CombinedOutput()
		return string(output), err
	}
	configPath := filepath.Join(dir, "agnostic-ai.yaml")
	cfg := `targets: [codex]
outputs:
  codex:
    exec-policies-from-permissions: true
  claude:
    settings:
      permissions:
        allow: ["Bash(npm run check)", "Bash(npx vitest run:*)", "Bash(git diff:*)"]
`
	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/settings"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/settings/security.yaml"), []byte("name: security\npermissions:\n  deny: [\"Bash(rm -rf:*)\"]\n  ask: [\"Bash(git push:*)\"]\n"), 0o644))
	must(t, os.WriteFile(configPath, []byte(cfg), 0o644))
	if out, err := run("sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	policyPath := filepath.Join(dir, ".codex/rules/default.rules")
	data, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`pattern = ["npm", "run", "check"]`, `pattern = ["npx", "vitest", "run"]`, `pattern = ["git", "diff"]`, `decision = "forbidden"`, `decision = "prompt"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("translated policy lacks %s:\n%s", want, data)
		}
	}
	if out, err := run("sync", "--check"); err != nil {
		t.Fatalf("check after sync: %v\n%s", err, out)
	}
	unsupported := strings.Replace(cfg, "Bash(npm run check)", "Bash(npm * check)", 1) + "on-unsupported: error\n"
	must(t, os.WriteFile(configPath, []byte(unsupported), 0o644))
	if out, err := run("sync"); err == nil || !strings.Contains(out, "Bash(npm * check)") || !strings.Contains(out, "agnostic-ai.yaml") {
		t.Errorf("unsupported rule should fail and name its source: %v\n%s", err, out)
	}
	after, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(data) {
		t.Error("unsupported error rewrote installed policies")
	}
	explicit := strings.Replace(cfg, "exec-policies-from-permissions: true", "exec-policies-from-permissions: true\n    exec-policies: [{pattern: [git], decision: forbidden}]", 1)
	must(t, os.WriteFile(configPath, []byte(explicit), 0o644))
	if out, err := run("lint", "--strict"); err == nil || !strings.Contains(out, "LINT021") || !strings.Contains(out, "Bash(git diff:*)") {
		t.Errorf("policy drift should fail strict lint: %v\n%s", err, out)
	}
	if out, err := run("sync"); err != nil {
		t.Fatalf("explicit policy sync: %v\n%s", err, out)
	}
	data, err = os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "npm") || strings.Contains(string(data), `decision = "allow"`) {
		t.Errorf("explicit native intent changed:\n%s", data)
	}
}
