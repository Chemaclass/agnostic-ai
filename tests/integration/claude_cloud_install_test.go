package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/claudehooks"
)

const claudeCloudInstallPath = ".claude/hooks/agnostic-ai-cloud-install.sh"

func TestClaudeCloudInstall_SyncImportAndOptOut(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	handoffGit(t, dir, "init", "-q")
	run := func(args ...string) string {
		t.Helper()
		out, err := ellipsisCommand(t, binary, dir, args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return out
	}
	const config = "version: 1\ntargets: [claude]\ngitignore:\n  enabled: true\n"
	const guidance = "gitignore.commit: [claude:environments, claude:hooks]"
	writeProjectFile(t, dir, "agnostic-ai.yaml", config)
	writeProjectFile(t, dir, ".agnostic-ai/environments/dev.yaml", "install: npm ci\n")
	out := run("sync")
	if strings.Count(out, guidance) != 1 || strings.Contains(out, "Claude Code has no install step") {
		t.Errorf("unexpected sync output:\n%s", out)
	}
	run("sync", "--check")
	script := readProjectFile(t, dir, claudeCloudInstallPath)
	settings := readProjectFile(t, dir, ".claude/settings.json")
	var doc claudehooks.Settings
	if err := json.Unmarshal([]byte(settings), &doc); err != nil {
		t.Fatal(err)
	}
	groups := doc.Hooks["SessionStart"]
	if len(groups) != 1 || groups[0].Matcher != "startup|resume" || len(groups[0].Hooks) != 1 {
		t.Fatalf("unexpected SessionStart hooks: %+v", groups)
	}
	if hook := groups[0].Hooks[0]; hook.Shell != "bash" || hook.Timeout != 0 || !strings.Contains(hook.Command, "agnostic-ai-cloud-install.sh") {
		t.Errorf("unexpected hook: %+v", hook)
	}

	writeProjectFile(t, dir, "agnostic-ai.yaml", config+"  commit: [claude:environments]\n")
	if out := run("sync"); strings.Contains(out, guidance) {
		t.Errorf("sync advised committing an included output:\n%s", out)
	}
	writeProjectFile(t, dir, "agnostic-ai.yaml", config+"  allow: [/.claude/settings.json, /.claude/hooks/, /.claude/hooks/agnostic-ai-cloud-install.sh]\n")
	if out := run("sync", "--json"); strings.Contains(out, guidance) {
		t.Errorf("sync advised committing an allowlisted output:\n%s", out)
	}
	run("import", "claude")
	for source := range sourceSnapshot(t, filepath.Join(dir, ".agnostic-ai")) {
		if strings.HasPrefix(filepath.ToSlash(source), "hooks/") || strings.Contains(source, "agnostic-ai-cloud-install.sh") {
			t.Errorf("import copied generated output into %s", source)
		}
	}
	run("sync")
	run("sync", "--check")
	run("lint")
	if readProjectFile(t, dir, claudeCloudInstallPath) != script || readProjectFile(t, dir, ".claude/settings.json") != settings {
		t.Error("import and sync changed the generated script or settings")
	}

	writeProjectFile(t, dir, "agnostic-ai.yaml", config)
	writeProjectFile(t, dir, ".agnostic-ai/environments/dev.yaml", "install: npm ci\nx-claude:\n  install: false\n")
	if out := run("sync"); strings.Contains(out, guidance) {
		t.Errorf("sync gave guidance after opting out:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, claudeCloudInstallPath)); !os.IsNotExist(err) {
		t.Errorf("cloud install script remains after opting out: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json")); err == nil && strings.Contains(string(data), "agnostic-ai-cloud-install.sh") {
		t.Error("cloud install hook remains after opting out")
	}
}

func TestClaudeCloudInstall_RunsOncePerInstallAndRetriesFailures(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not on PATH")
	}
	if _, err := exec.LookPath("flock"); err != nil {
		if _, err := exec.LookPath("lockf"); err != nil {
			t.Skip("neither flock nor lockf on PATH")
		}
	}
	dir := t.TempDir()
	binary := builtBinary(t)
	handoffGit(t, dir, "init", "-q")
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	sync := func(install string) string {
		t.Helper()
		writeProjectFile(t, dir, ".agnostic-ai/environments/dev.yaml", "install: |\n  "+install+"\n")
		if out, err := ellipsisCommand(t, binary, dir, "sync"); err != nil {
			t.Fatalf("sync: %v\n%s", err, out)
		}
		var doc claudehooks.Settings
		if err := json.Unmarshal([]byte(readProjectFile(t, dir, ".claude/settings.json")), &doc); err != nil {
			t.Fatal(err)
		}
		return doc.Hooks["SessionStart"][0].Hooks[0].Command
	}
	run := func(command, remote, target string) (string, error) {
		t.Helper()
		cmd := exec.Command("sh", "-c", command)
		cmd.Dir = filepath.Join(dir, ".claude")
		cmd.Env = append(os.Environ(), "CLAUDE_CODE_REMOTE="+remote, "AGNOSTIC_AI_TARGET="+target, "CLAUDE_PROJECT_DIR="+dir)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		stdout, err := cmd.Output()
		if len(stdout) != 0 {
			t.Errorf("install wrote to stdout: %q", stdout)
		}
		return stderr.String(), err
	}
	command := sync("sleep 0.3; echo first; echo first >> installs")
	for _, env := range [][2]string{{"", "claude"}, {"true", "cursor"}} {
		if out, err := run(command, env[0], env[1]); err != nil || out != "" {
			t.Errorf("non-cloud Claude check %v: %v, stderr %q", env, err, out)
		}
	}
	results := make(chan string, 2)
	for range 2 {
		go func() {
			out, err := run(command, "true", "claude")
			if err != nil {
				t.Errorf("install: %v, stderr %q", err, out)
			}
			results <- out
		}()
	}
	if got := <-results + <-results; got != "first\n" {
		t.Errorf("concurrent starts ran install more or less than once: %q", got)
	}
	if out, err := run(command, "true", "claude"); err != nil || out != "" {
		t.Errorf("resume repeated install: %v, stderr %q", err, out)
	}
	command = sync("echo failed; false; echo unreachable")
	for range 2 {
		if out, err := run(command, "true", "claude"); err == nil || out != "failed\n" {
			t.Errorf("failed install: %v, stderr %q", err, out)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "agnostic-ai-cloud-install")); !os.IsNotExist(err) {
		t.Errorf("failed install left its success marker: %v", err)
	}
	command = sync("echo second; echo second >> installs")
	if out, err := run(command, "true", "claude"); err != nil || out != "second\n" {
		t.Errorf("changed install: %v, stderr %q", err, out)
	}
	if got := readProjectFile(t, dir, "installs"); got != "first\nsecond\n" {
		t.Errorf("install runs in project root: got %q", got)
	}
	command = sync("read -r answer || :\n  echo large # " + strings.Repeat("x", 300_000))
	if out, err := run(command, "true", "claude"); err != nil || out != "large\n" {
		t.Errorf("large install: %v, stderr %q", err, out)
	}
}
