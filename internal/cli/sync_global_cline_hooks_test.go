package cli

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

func TestSyncGlobal_ClineHookDirectory(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".cline", "hooks", "PreToolUse")
	manual := filepath.Join(home, ".cline", "hooks", "TaskStart")
	mustWriteGlobalTest(t, manual, "#!/bin/sh\necho manual\n")
	specPath := filepath.Join(source, "hooks", "guard.yaml")
	mustWriteGlobalTest(t, specPath, "name: guard\nevent: PreToolUse\ncommand: echo guard\n")
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "second.yaml"), "name: second\nevent: pretooluse\ncommand: echo second\n")
	out, _, err := runGlobalAgentTest("--only", "cline", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, path) {
		t.Errorf("preview missing hook: %s", out)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("preview wrote hook: %v", err)
	}
	if _, _, err := runGlobalAgentTest("--only", "cline"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"#!/usr/bin/env bash", "AGNOSTIC_AI_TARGET=cline", "echo guard", "echo second"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("hook lacks %q:\n%s", want, raw)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		t.Error("hook is not executable")
	}
	if _, _, err := runGlobalAgentTest("--only", "cline", "--check"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(specPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(source, "hooks", "second.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest("--only", "cline"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("removed source left hook: %v", err)
	}
	if raw, err := os.ReadFile(manual); err != nil || string(raw) != "#!/bin/sh\necho manual\n" {
		t.Errorf("manual hook changed: %v\n%s", err, raw)
	}
	if _, err := os.Stat(filepath.Join(home, "Documents", "Cline", "Hooks")); !os.IsNotExist(err) {
		t.Errorf("created unsupported Documents output: %v", err)
	}
}

func TestSyncGlobal_ClineHookHonorsRootAndNeutralScript(t *testing.T) {
	home, source := globalAgentTestHome(t)
	root := filepath.Join(home, "custom cline")
	t.Setenv("CLINE_DIR", root)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\nevent: PreToolUse\ncommand: .agnostic-ai/scripts/guard.sh\n")
	mustWriteGlobalTest(t, filepath.Join(source, "scripts", "guard.sh"), "#!/bin/sh\nprintf '%s\\n' 'blocked synthetic guard' >&2\nexit 2\n")
	if err := os.Chmod(filepath.Join(source, "scripts", "guard.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest("--only", "cline"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "hooks", "PreToolUse")
	if _, err := os.Stat(filepath.Join(root, "hook-scripts", "guard.sh")); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("bash", path)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		t.Fatalf("run global hook from spaced path: %v\nstdout: %s\nstderr: %s", err, output, stderr.String())
	}
	var reply struct {
		Cancel       bool   `json:"cancel"`
		ErrorMessage string `json:"errorMessage"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(string(output), "HOOK_CONTROL\t")), &reply); err != nil {
		t.Fatalf("read Cline control reply: %v\n%s", err, output)
	}
	if !reply.Cancel || reply.ErrorMessage != "blocked synthetic guard" {
		t.Errorf("Cline control reply = %+v", reply)
	}
	if _, _, err := runGlobalAgentTest("--only", "cline", "--check"); err != nil {
		t.Fatal(err)
	}
}

func TestSyncGlobal_ClineHookConflictKeepsNativeScript(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".cline", "hooks", "PreToolUse")
	manual := "#!/bin/sh\necho user\n"
	mustWriteGlobalTest(t, path, manual)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\nevent: PreToolUse\ncommand: echo guard\n")
	if _, _, err := runGlobalAgentTest("--only", "cline"); err == nil {
		t.Fatal("sync overwrote a conflicting native hook")
	}
	if raw, err := os.ReadFile(path); err != nil || string(raw) != manual {
		t.Errorf("native hook changed: %v\n%s", err, raw)
	}
}

func TestSyncGlobal_ClineHooksDoNotClaimGlobalImportSupport(t *testing.T) {
	globalAgentTestHome(t)
	if _, _, err := runImportGlobalTest("cline"); err == nil {
		t.Fatal("global hook directory falsely made Cline importable")
	}
}
