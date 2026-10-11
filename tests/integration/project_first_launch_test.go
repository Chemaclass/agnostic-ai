package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestProjectLifecycle_FirstLaunchAfterFreshInstall(t *testing.T) {
	repository, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	name, installerName := "agnostic-ai", "pnpm"
	if runtime.GOOS == "windows" {
		name += ".exe"
		installerName += ".cmd"
	}
	binary := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-ldflags", "-X main.version=99.1.0 -X github.com/chemaclass/agnostic-ai/internal/cli.candidateVersion=99.1.0", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = repository
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	helper, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	installerDir := t.TempDir()
	launcher := "#!/bin/sh\nexec \"$AGNOSTIC_LIFECYCLE_TEST_EXECUTABLE\" -test.run=^TestProjectLifecycleInstallerProcess$ -- \"$@\"\n"
	if runtime.GOOS == "windows" {
		launcher = "@echo off\r\n\"%AGNOSTIC_LIFECYCLE_TEST_EXECUTABLE%\" -test.run=^TestProjectLifecycleInstallerProcess$ -- %*\r\nexit /b %errorlevel%\r\n"
	}
	if err := os.WriteFile(filepath.Join(installerDir, installerName), []byte(launcher), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", installerDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AGNOSTIC_LIFECYCLE_INSTALLER", "1")
	t.Setenv("AGNOSTIC_LIFECYCLE_TEST_EXECUTABLE", helper)
	t.Setenv("AGNOSTIC_LIFECYCLE_OLD_BINARY", binary)
	t.Setenv("AGNOSTIC_AI_PROJECT_BOOTSTRAP", "")
	t.Setenv("AGNOSTIC_AI_NO_UPDATE_CHECK", "1")
	t.Setenv("AGNOSTIC_AI_TARGET", "")
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())

	for i := 0; i < 8; i++ {
		dir := filepath.Join(t.TempDir(), fmt.Sprintf("fresh project %d", i))
		mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\nrequires: '99.1.0'\ntargets: [codex]\n")
		mustWrite(t, filepath.Join(dir, ".agnostic-ai/rules/example.md"), "---\nname: example\ndescription: Example rule.\n---\nKeep the public API stable.\n")
		mustWrite(t, filepath.Join(dir, "package.json"), "{\"private\":true,\"packageManager\":\"pnpm@10.0.0\",\"devDependencies\":{\"agnostic-ai\":\"99.1.0\"}}\n")
		mustWrite(t, filepath.Join(dir, "pnpm-lock.yaml"), "lockfileVersion: '9.0'\nfixtureVersion: 99.1.0\n")
		diagnostic := filepath.Join(t.TempDir(), "launcher-error.json")
		t.Setenv("AGNOSTIC_LIFECYCLE_LAUNCH_DIAGNOSTIC", diagnostic)
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		cmd := exec.CommandContext(ctx, binary, "project", "--bootstrap")
		cmd.Dir, cmd.WaitDelay = dir, 250*time.Millisecond
		var output lifecycleProbeOutput
		cmd.Stdout, cmd.Stderr = &output, &output
		err := cmd.Run()
		contextErr := ctx.Err()
		cancel()
		if err != nil {
			if receipt, readErr := os.ReadFile(diagnostic); readErr == nil {
				t.Logf("original launcher result:\n%s", receipt)
			} else if runtime.GOOS == "windows" {
				t.Logf("launcher result unavailable: %v", readErr)
			}
			t.Fatalf("fresh install %d: %v (context: %v)\n%s", i, err, contextErr, output.data)
		}
		assertContains(t, filepath.Join(dir, "AGENTS.md"), "Keep the public API stable.")
		installs, err := os.ReadFile(filepath.Join(dir, "node_modules/lifecycle-installs.log"))
		if err != nil || string(installs) != "99.1.0 install --frozen-lockfile marker=1\n" {
			t.Fatalf("fresh install %d must install once: %v\n%s", i, err, installs)
		}
	}
}
