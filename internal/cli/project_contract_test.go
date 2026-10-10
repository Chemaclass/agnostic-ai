package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func projectContractBinary(t *testing.T, root, version string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX binary fixture; generated launchers have portable integration coverage")
	}
	local := filepath.Join(root, "node_modules", ".bin", "agnostic-ai")
	mustWriteFile(t, local, "#!/bin/sh\nif [ \"$1\" = --version ]; then echo 'agnostic-ai "+version+"'; else exit 0; fi\n")
	if err := os.Chmod(local, 0o755); err != nil {
		t.Fatal(err)
	}
	return local
}

func TestProjectContract_ExactPackagePin(t *testing.T) {
	for _, tc := range []struct{ name, requires, dependencies, devDependencies string }{
		{"dependency", "", "0.82.0", ""},
		{"development dependency", ">=0.80.0", "", "0.82.0"},
		{"contradictory declarations", "", "0.81.0", "0.82.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			projectContractBinary(t, root, "0.81.0")
			pkg := projectPackage{Dependencies: map[string]string{"agnostic-ai": tc.dependencies}, DevDependencies: map[string]string{"agnostic-ai": tc.devDependencies}}
			_, err := resolveProjectBinary(root, pkg, tc.requires)
			if err == nil || !strings.Contains(err.Error(), "package.json") {
				t.Errorf("exact package contract was ignored: %v", err)
			}
		})
	}
}

func TestProjectContract_PinOnlyChangeBootstraps(t *testing.T) {
	root := budgetProject(t, "requires: '>=0.80.0'\ntargets: [codex]\n")
	local := projectContractBinary(t, root, "0.81.0")
	mustWriteFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"agnostic-ai":"0.82.0"},"packageManager":"pnpm@10.0.0"}`)
	mustWriteFile(t, filepath.Join(root, "pnpm-lock.yaml"), "lockfileVersion: '9.0'\n")
	installer := filepath.Join(t.TempDir(), "pnpm")
	mustWriteFile(t, installer, "#!/bin/sh\necho installed > '"+filepath.Join(root, "install.log")+"'\nprintf '#!/bin/sh\\nif [ \"$1\" = --version ]; then echo agnostic-ai 0.82.0; else exit 0; fi\\n' > '"+local+"'\n")
	if err := os.Chmod(installer, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(installer)+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, err := runCLI(t, "project", "--check"); err == nil || !strings.Contains(err.Error(), "package.json") {
		t.Errorf("pin change check accepted old installed version: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "install.log")); !os.IsNotExist(err) {
		t.Error("check installed dependencies")
	}
	if _, err := runCLI(t, "project", "--bootstrap"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "install.log")); err != nil {
		t.Errorf("pin-only change did not install: %v", err)
	}
}

func TestProjectContract_AgainstUsesRequestedConfig(t *testing.T) {
	for _, ref := range []string{"index", "HEAD"} {
		for _, unstaged := range []string{"version: [broken", "version: 1\nrequires: '0.83.0'\ntargets: [codex]\n"} {
			t.Run(ref+unstaged, func(t *testing.T) {
				root := setupGitRepo(t)
				projectContractBinary(t, root, "0.82.0")
				mustWriteFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\nrequires: '0.82.0'\ntargets: [codex]\n")
				mustWriteFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"agnostic-ai":"0.82.0"}}`)
				git(t, root, "add", "agnostic-ai.yaml", "package.json")
				git(t, root, "commit", "-qm", "fixture")
				mustWriteFile(t, filepath.Join(root, "agnostic-ai.yaml"), unstaged)
				mustWriteFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"agnostic-ai":"0.83.0"}}`)
				testutil.Chdir(t, root)
				if _, err := runCLI(t, "project", "--check", "--against", ref); err != nil {
					t.Errorf("unstaged config blocked %s: %v", ref, err)
				}
			})
		}
	}
}

func TestProjectContract_ConflictingRequiresNeverInstalls(t *testing.T) {
	root := budgetProject(t, "requires: '0.83.0'\ntargets: [codex]\n")
	projectContractBinary(t, root, "0.82.0")
	mustWriteFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"agnostic-ai":"0.82.0"}}`)
	_, err := runCLI(t, "project", "--bootstrap")
	if err == nil || !strings.Contains(err.Error(), "conflicts") || !strings.Contains(err.Error(), "explicitly") {
		t.Errorf("incompatible declarations attempted bootstrap: %v", err)
	}
}

func TestProjectContract_UnsupportedLocalNeverInstalls(t *testing.T) {
	root := budgetProject(t, "requires: '0.82.0'\ntargets: [codex]\n")
	local := projectContractBinary(t, root, "0.82.0")
	mustWriteFile(t, local, "#!/bin/sh\nif [ \"$1\" = --version ]; then echo agnostic-ai 0.82.0; else exit 2; fi\n")
	mustWriteFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"agnostic-ai":"0.82.0"},"packageManager":"pnpm@10.0.0"}`)
	mustWriteFile(t, filepath.Join(root, "pnpm-lock.yaml"), "lockfileVersion: '9.0'\n")
	installer := filepath.Join(t.TempDir(), "pnpm")
	mustWriteFile(t, installer, "#!/bin/sh\necho installed > '"+filepath.Join(root, "install.log")+"'\n")
	if err := os.Chmod(installer, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(installer)+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, err := runCLI(t, "project", "--bootstrap")
	if err == nil || !strings.Contains(err.Error(), "upgrade") {
		t.Errorf("unsupported capability recovery: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "install.log")); !os.IsNotExist(err) {
		t.Errorf("unsupported capability reinstalled the same package: %v", err)
	}
}

func TestProjectContract_AgainstFindsRemovedNestedConfig(t *testing.T) {
	for _, ref := range []string{"index", "HEAD"} {
		t.Run(ref, func(t *testing.T) {
			outer := setupGitRepo(t)
			projectContractBinary(t, outer, "0.81.0")
			mustWriteFile(t, filepath.Join(outer, "agnostic-ai.yaml"), "version: 1\nrequires: '0.83.0'\ntargets: [codex]\n")
			root := filepath.Join(outer, "nested")
			projectContractBinary(t, root, "0.82.0")
			mustWriteFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\nrequires: '0.82.0'\ntargets: [codex]\n")
			mustWriteFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"agnostic-ai":"0.82.0"}}`)
			git(t, outer, "add", "agnostic-ai.yaml", "nested/agnostic-ai.yaml", "nested/package.json")
			git(t, outer, "commit", "-qm", "fixture")
			if err := os.Remove(filepath.Join(root, "agnostic-ai.yaml")); err != nil {
				t.Fatal(err)
			}
			testutil.Chdir(t, root)
			if _, err := runCLI(t, "project", "--check", "--against", ref); err != nil {
				t.Errorf("removed nested working config selected outer project: %v", err)
			}
		})
	}
}

func TestProjectContract_AgainstChecksNestedPackage(t *testing.T) {
	outer := setupGitRepo(t)
	mustWriteFile(t, filepath.Join(outer, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	root := filepath.Join(outer, "nested")
	projectContractBinary(t, root, "0.81.0")
	mustWriteFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\nrequires: '>=0.80.0'\ntargets: [codex]\n")
	mustWriteFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"agnostic-ai":"0.82.0"}}`)
	git(t, outer, "add", "agnostic-ai.yaml", "nested/agnostic-ai.yaml", "nested/package.json")
	git(t, outer, "commit", "-qm", "fixture")
	mustWriteFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"agnostic-ai":"0.81.0"}}`)
	testutil.Chdir(t, root)
	for _, ref := range []string{"index", "HEAD"} {
		_, err := runCLI(t, "project", "--check", "--against", ref)
		if err == nil || !strings.Contains(err.Error(), "package.json pins agnostic-ai to 0.82.0") {
			t.Errorf("nested %s package pin was not checked: %v", ref, err)
		}
	}
}
