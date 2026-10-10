package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestProject_PrefersLocalAndCheckNeverInstalls(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX launcher fixture")
	}
	dir := budgetProject(t, "requires: '0.82.0'\ntargets: [codex]\n")
	mustWriteFile(t, filepath.Join(dir, "package.json"), `{"devDependencies":{"agnostic-ai":"0.82.0"},"packageManager":"pnpm@10.0.0"}`)
	log := filepath.Join(dir, "calls")
	local := filepath.Join(dir, "node_modules", ".bin", "agnostic-ai")
	mustWriteFile(t, local, "#!/bin/sh\nif [ \"$1\" = --version ]; then echo 'agnostic-ai version 0.82.0'; else printf '%s\\n' \"$*\" >> '"+log+"'; fi\n")
	if err := os.Chmod(local, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "project", "--check"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "sync --check\n" {
		t.Errorf("local calls: %s", data)
	}
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\nrequires: '0.83.0'\ntargets: [codex]\n")
	_, err = runCLI(t, "project", "--check")
	if err == nil || !strings.Contains(err.Error(), "0.82.0") || !strings.Contains(err.Error(), "0.83.0") || !strings.Contains(err.Error(), "project --bootstrap") {
		t.Errorf("mismatch: %v", err)
	}
	data, err = os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "sync --check\n" {
		t.Errorf("check mutated calls: %s", data)
	}
}

func TestProject_InstallUsesFrozenManagerContract(t *testing.T) {
	for _, tc := range []struct{ manager, lock, want string }{
		{"pnpm@10.0.0", "pnpm-lock.yaml", "install --frozen-lockfile"},
		{"npm@11.0.0", "package-lock.json", "ci"},
		{"", "pnpm-lock.yaml", "install --frozen-lockfile"},
	} {
		t.Run(tc.manager+tc.lock, func(t *testing.T) {
			dir := t.TempDir()
			mustWriteFile(t, filepath.Join(dir, tc.lock), "fixture")
			command, err := projectInstallCommand(dir, projectPackage{PackageManager: tc.manager, DevDependencies: map[string]string{"agnostic-ai": "0.82.0"}})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.Join(command.Args, " "), tc.want) {
				t.Errorf("install command: %v", command.Args)
			}
		})
	}
	if _, err := projectInstallCommand(t.TempDir(), projectPackage{DevDependencies: map[string]string{"agnostic-ai": "0.82.0"}}); err == nil {
		t.Error("bootstrap without lockfile accepted")
	}
}

func TestProjectMemory_UsesNativeHostDirectoryForVersionContract(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX local launcher fixture")
	}
	for target, variable := range hookProjectDirEnv {
		t.Run(target, func(t *testing.T) {
			project := budgetProject(t, "requires: '0.83.0'\ntargets: ["+target+"]\n")
			local := filepath.Join(project, "node_modules", ".bin", "agnostic-ai")
			mustWriteFile(t, local, "#!/bin/sh\necho 'agnostic-ai version 0.82.0'\n")
			if err := os.Chmod(local, 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv(variable, project)
			t.Setenv(adapters.HookTargetEnv, "")
			t.Setenv("PATH", t.TempDir())
			testutil.Chdir(t, t.TempDir())
			_, err := runCLI(t, "project", "--", "hook", "memory", "--target="+target)
			if err == nil || !strings.Contains(err.Error(), "0.82.0") || !strings.Contains(err.Error(), "0.83.0") {
				t.Fatalf("host project contract was not checked: %v", err)
			}
		})
	}
}

func TestProjectMemory_UsesConfiglessGitRootFromNestedDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX local launcher fixture")
	}
	project := setupGitRepo(t)
	local := filepath.Join(project, "node_modules", ".bin", "agnostic-ai")
	mustWriteFile(t, local, "#!/bin/sh\nif [ \"$1\" = --version ]; then echo 'agnostic-ai version 0.82.0'; else printf '%s\\n' \"$*\"; fi\n")
	if err := os.Chmod(local, 0o755); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(project, "src")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, child)
	silence(t)
	for _, variable := range hookProjectDirEnv {
		t.Setenv(variable, "")
	}
	got, err := runCLI(t, "project", "--", "hook", "memory", "-t", "codex")
	if err != nil || !strings.Contains(got, "hook memory -t codex") {
		t.Fatalf("configless local binary was not used: output=%q error=%v", got, err)
	}
}

func TestProjectCheck_IsRecognizedAsProjectGate(t *testing.T) {
	if !hasProjectCheck(projectHook.text()) {
		t.Error("new project gate was not recognized")
	}
}
