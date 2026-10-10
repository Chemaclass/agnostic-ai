package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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

func TestProjectCheck_IsRecognizedAsProjectGate(t *testing.T) {
	if !hasProjectCheck(projectHook.text()) {
		t.Error("new project gate was not recognized")
	}
}
