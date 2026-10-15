package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestProject_BunInstallUsesFrozenLockfile(t *testing.T) {
	for _, tc := range []struct{ name, manifest, lock string }{
		{"devEngines object", `{"devDependencies":{"agnostic-ai":"0.82.0"},"devEngines":{"packageManager":{"name":"bun","version":"1.4.2","onFail":"download"}}}`, "bun.lock"},
		{"devEngines list", `{"devDependencies":{"agnostic-ai":"0.82.0"},"devEngines":{"packageManager":[{"name":"bun"}]}}`, "bun.lock"},
		{"packageManager", `{"devDependencies":{"agnostic-ai":"0.82.0"},"packageManager":"bun@1.4.2"}`, "bun.lockb"},
		{"lockfile only", `{"devDependencies":{"agnostic-ai":"0.82.0"}}`, "bun.lock"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			mustWriteFile(t, filepath.Join(dir, "package.json"), tc.manifest)
			mustWriteFile(t, filepath.Join(dir, tc.lock), "fixture")
			pkg, err := readProjectPackage(dir)
			if err != nil {
				t.Fatal(err)
			}
			command, err := projectInstallCommand(dir, pkg)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(command.Args, " "); !strings.Contains(got, "bun install --frozen-lockfile") {
				t.Errorf("install command: %s", got)
			}
		})
	}
}

func TestProject_BootstrapRefusesWithManagerNextStep(t *testing.T) {
	for _, tc := range []struct{ name, manifest, lock, want string }{
		{"bun without lockfile", `{"devDependencies":{"agnostic-ai":"0.82.0"},"devEngines":{"packageManager":{"name":"bun"}}}`, "", "bun.lock"},
		{"yarn", `{"devDependencies":{"agnostic-ai":"0.82.0"},"packageManager":"yarn@4.0.0"}`, "yarn.lock", "yarn install"},
		{"conflicting declarations", `{"devDependencies":{"agnostic-ai":"0.82.0"},"packageManager":"pnpm@10.0.0","devEngines":{"packageManager":{"name":"bun"}}}`, "bun.lock", "devEngines"},
		{"undeclared", `{"devDependencies":{"agnostic-ai":"0.82.0"}}`, "", "npm, pnpm, or bun"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := budgetProject(t, "targets: [codex]\n")
			mustWriteFile(t, filepath.Join(root, "package.json"), tc.manifest)
			if tc.lock != "" {
				mustWriteFile(t, filepath.Join(root, tc.lock), "fixture")
			}
			_, err := runCLI(t, "project", "--bootstrap")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("bootstrap error: %v", err)
			}
			_, err = runCLI(t, "project")
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "run `agnostic-ai project --bootstrap`") {
				t.Errorf("project error must name the reason, not a bootstrap that cannot help: %v", err)
			}
		})
	}
}

func TestProjectContract_BunDevEnginesBootstraps(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer fixture")
	}
	root := budgetProject(t, "targets: [codex]\n")
	local := filepath.Join(root, "node_modules", ".bin", "agnostic-ai")
	mustWriteFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"agnostic-ai":"0.82.0"},"devEngines":{"packageManager":{"name":"bun","version":"1.4.2","onFail":"download"}}}`)
	mustWriteFile(t, filepath.Join(root, "bun.lock"), "{}\n")
	installer := filepath.Join(t.TempDir(), "bun")
	mustWriteFile(t, installer, "#!/bin/sh\necho \"$*\" > '"+filepath.Join(root, "install.log")+"'\nmkdir -p '"+filepath.Dir(local)+"'\nprintf '#!/bin/sh\\nif [ \"$1\" = --version ]; then echo agnostic-ai 0.82.0; else exit 0; fi\\n' > '"+local+"'\nchmod +x '"+local+"'\n")
	if err := os.Chmod(installer, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(installer)+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, err := runCLI(t, "project", "--bootstrap"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "install.log"))
	if err != nil || string(data) != "install --frozen-lockfile\n" {
		t.Errorf("bun install: %q %v", data, err)
	}
}

func TestProjectHook_MissingCLINamesManagerInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX hook fixture")
	}
	for _, tc := range []struct{ lock, want string }{
		{"bun.lock", "run bun install"},
		{"bun.lockb", "run bun install"},
		{"pnpm-lock.yaml", "run pnpm install"},
		{"package-lock.json", "run npm ci"},
		{"yarn.lock", "run yarn install"},
		{"", "install it, then run agnostic-ai project --bootstrap"},
	} {
		t.Run(tc.lock, func(t *testing.T) {
			dir := t.TempDir()
			mustWriteFile(t, filepath.Join(dir, "package.json"), `{"devDependencies":{"agnostic-ai":"0.82.0"}}`)
			if tc.lock != "" {
				mustWriteFile(t, filepath.Join(dir, tc.lock), "fixture")
			}
			hook := exec.Command("sh", "-c", "agnostic_ai_bin=agnostic-ai-missing-fixture\n"+projectHookCapability)
			hook.Dir = dir
			out, err := hook.CombinedOutput()
			if err == nil || !strings.Contains(string(out), tc.want) {
				t.Errorf("hook output %q, err %v", out, err)
			}
		})
	}
}

func TestInstallHookPostCheckout_UpgradesMissingCLIMessage(t *testing.T) {
	dir := setupGitRepo(t)
	testutil.Chdir(t, dir)
	path := postCheckoutHookPath(dir)
	old := strings.Replace(postCheckoutHook.checks, projectHookCapability, projectHookCapabilityV1, 1)
	if old == postCheckoutHook.checks {
		t.Fatal("fixture did not swap the capability check")
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+postCheckoutHook.sentinel+"\n"+old+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := runInstallHook("--post-checkout"); err != nil {
		t.Fatal(err)
	}
	got := readHook(t, path)
	if strings.Contains(got, projectHookCapabilityV1) || !strings.Contains(got, "run bun install") {
		t.Errorf("hook not upgraded:\n%s", got)
	}
}
