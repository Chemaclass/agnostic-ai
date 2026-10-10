package integration

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProjectLifecycle_RealNpmBootstrap(t *testing.T) {
	for _, command := range []string{"node", "npm"} {
		if _, err := exec.LookPath(command); err != nil {
			if os.Getenv("CI") != "" {
				t.Fatalf("CI must provide %s for the real npm lifecycle: %v", command, err)
			}
			t.Skipf("real npm lifecycle requires %s: %v", command, err)
		}
	}
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(t.TempDir(), "npm lifecycle with spaces")
	project := filepath.Join(workspace, "project with spaces")
	packageSource := filepath.Join(workspace, "fake package")
	globalBin := filepath.Join(workspace, "old global")
	for _, dir := range []string{project, filepath.Join(project, ".agnostic-ai"), filepath.Join(packageSource, "bin"), globalBin} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, content string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	binary := filepath.Join(workspace, "candidate")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-ldflags", "-X main.version=0.82.0 -X github.com/chemaclass/agnostic-ai/internal/cli.candidateVersion=0.82.0", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = filepath.Join(packageDir, "..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	t.Setenv("PROJECT_NPM_TEST_BINARY", binary)
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	t.Setenv("AGNOSTIC_AI_PROJECT_BOOTSTRAP", "")
	t.Setenv("npm_config_offline", "true")
	t.Setenv("npm_config_audit", "false")
	t.Setenv("npm_config_fund", "false")
	t.Setenv("npm_config_update_notifier", "false")
	t.Setenv("npm_config_cache", filepath.Join(workspace, "npm cache"))
	for _, config := range []string{"userconfig", "globalconfig"} {
		path := filepath.Join(workspace, config)
		write(path, "", 0o644)
		t.Setenv("npm_config_"+config, path)
	}
	write(filepath.Join(packageSource, "package.json"), `{"name":"agnostic-ai","version":"0.82.0","bin":{"agnostic-ai":"bin/agnostic-ai.js"},"files":["bin"]}`, 0o644)
	write(filepath.Join(packageSource, "bin", "agnostic-ai.js"), `#!/usr/bin/env node
const { execFileSync } = require('node:child_process')
try {
  execFileSync(process.env.PROJECT_NPM_TEST_BINARY, process.argv.slice(2), { stdio: 'inherit' })
} catch (error) {
  process.exit(Number.isInteger(error.status) ? error.status : 1)
}
`, 0o755)
	npm := func(dir, command string, args ...string) []byte {
		t.Helper()
		cmd := exec.Command("npm", append([]string{command}, args...)...)
		if runtime.GOOS == "windows" {
			// All arguments are fixed literals; paths are passed through the working directory.
			cmd = exec.Command("cmd.exe", "/d", "/s", "/c", "npm "+strings.Join(append([]string{command}, args...), " "))
		}
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("npm %s: %v\n%s", command, err, out)
		}
		return out
	}
	packed := npm(packageSource, "pack", "--ignore-scripts", "--json", "--offline")
	var packages []struct {
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal(packed, &packages); err != nil || len(packages) != 1 || packages[0].Filename != "agnostic-ai-0.82.0.tgz" {
		t.Fatalf("local npm package: %v\n%s", err, packed)
	}
	write(filepath.Join(project, "package.json"), `{"name":"project-lifecycle-fixture","version":"1.0.0","private":true,"devDependencies":{"agnostic-ai":"file:../fake package/agnostic-ai-0.82.0.tgz"},"scripts":{"postinstall":"node postinstall.cjs"}}`, 0o644)
	write(filepath.Join(project, "postinstall.cjs"), `const fs = require('node:fs')
const path = require('node:path')
const { execFileSync } = require('node:child_process')
fs.appendFileSync('postinstall.jsonl', JSON.stringify({ marker: process.env.AGNOSTIC_AI_PROJECT_BOOTSTRAP, event: process.env.npm_lifecycle_event }) + '\n')
if (fs.readFileSync('postinstall.jsonl', 'utf8').trim().split('\n').length > 1) {
  throw new Error('recursive package install detected')
}
execFileSync(process.execPath, [path.join('node_modules', 'agnostic-ai', 'bin', 'agnostic-ai.js'), 'project', '--bootstrap'], { stdio: 'inherit' })
`, 0o644)
	write(filepath.Join(project, "agnostic-ai.yaml"), "version: 1\nrequires: '0.82.0'\ntargets: [codex]\n", 0o644)
	write(filepath.Join(project, ".agnostic-ai", "AGNOSTIC_AI.md"), "Real npm lifecycle guidance.\n", 0o644)
	npm(project, "install", "--package-lock-only", "--ignore-scripts", "--offline", "--no-audit", "--no-fund")
	originals := make(map[string][]byte)
	for _, name := range []string{"package.json", "package-lock.json"} {
		originals[name], err = os.ReadFile(filepath.Join(project, name))
		if err != nil {
			t.Fatal(err)
		}
	}
	oldGlobal := "#!/bin/sh\nif [ \"$1\" = --version ]; then echo 'agnostic-ai 0.81.0'; exit 0; fi\necho 'older global must not run' >&2\nexit 91\n"
	oldName := "agnostic-ai"
	if runtime.GOOS == "windows" {
		oldName += ".cmd"
		oldGlobal = "@echo off\r\nif \"%~1\"==\"--version\" (\r\n  echo agnostic-ai 0.81.0\r\n  exit /b 0\r\n)\r\necho older global must not run 1>&2\r\nexit /b 91\r\n"
	}
	write(filepath.Join(globalBin, oldName), oldGlobal, 0o755)
	t.Setenv("PATH", globalBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	run := func(args ...string) (string, error) {
		cmd := exec.Command(binary, args...)
		cmd.Dir = project
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := run("project", "--check"); err == nil || !strings.Contains(out, "missing") {
		t.Fatalf("fresh check: %v\n%s", err, out)
	}
	for _, name := range []string{"node_modules", "postinstall.jsonl"} {
		if _, err := os.Stat(filepath.Join(project, name)); !os.IsNotExist(err) {
			t.Fatalf("read-only check created %s: %v", name, err)
		}
	}
	for i := 0; i < 2; i++ {
		if out, err := run("project", "--bootstrap"); err != nil {
			t.Fatalf("bootstrap %d: %v\n%s", i+1, err, out)
		}
		log, err := os.ReadFile(filepath.Join(project, "postinstall.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		if string(log) != "{\"marker\":\"1\",\"event\":\"postinstall\"}\n" {
			t.Fatalf("postinstall must run once with the recursion marker: %s", log)
		}
	}
	if out, err := run("project", "--check"); err != nil {
		t.Fatalf("installed local package check: %v\n%s", err, out)
	}
	agents, err := os.ReadFile(filepath.Join(project, "AGENTS.md"))
	if err != nil || !strings.Contains(string(agents), "Real npm lifecycle guidance.") {
		t.Fatalf("postinstall did not sync project guidance: %v\n%s", err, agents)
	}
	for name, original := range originals {
		current, err := os.ReadFile(filepath.Join(project, name))
		if err != nil || !bytes.Equal(original, current) {
			t.Errorf("bootstrap changed %s: %v", name, err)
		}
	}
	if err := os.Remove(filepath.Join(project, "postinstall.jsonl")); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(project, "agnostic-ai.yaml"), "version: 1\nrequires: '0.83.0'\ntargets: [codex]\n", 0o644)
	out, bootstrapErr := run("project", "--bootstrap")
	log, err := os.ReadFile(filepath.Join(project, "postinstall.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(log) != "{\"marker\":\"1\",\"event\":\"postinstall\"}\n" {
		t.Fatalf("version mismatch recursively installed packages: %s\n%s", log, out)
	}
	if bootstrapErr == nil || !strings.Contains(out, "requires 0.83.0") || strings.Contains(out, "recursive package install detected") {
		t.Fatalf("locked version mismatch must fail after one normal postinstall: %v\n%s", bootstrapErr, out)
	}
}
