package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSync_AugmentLocalHookKeepsInheritedBuiltinScript(t *testing.T) {
	setupMemoryProject(t, "augment")
	writeFile(t, ".agnostic-ai/local/hooks/memory-session-start.yaml", "timeout: 9\n")
	syncNotes(t)
	data, err := os.ReadFile(".augment/hooks/memory-augment-session-start.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "hook memory --target augment") {
		t.Errorf("inherited script does not load memory: %s", data)
	}
	syncNotes(t, "--check")
}

func TestSyncGlobal_AugmentLocalHookKeepsInheritedBuiltinScript(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "version: 1\ntargets: [augment]\nbuiltins: [memory]\n")
	mustWriteGlobalTest(t, filepath.Join(source, "local", "hooks", "memory-session-start.yaml"), "timeout: 9\n")
	if _, _, err := runGlobalAgentTest("--only", "augment"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".augment", "hooks", "memory-augment-session-start.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "hook memory --target augment") {
		t.Errorf("inherited global script does not load memory: %s", data)
	}
	if _, _, err := runGlobalAgentTest("--only", "augment", "--check"); err != nil {
		t.Fatal(err)
	}
}

func TestSync_AugmentLocalCommandUsesUserScript(t *testing.T) {
	setupMemoryProject(t, "augment")
	writeFile(t, ".agnostic-ai/local/hooks/memory-session-start.yaml", "x-augment:\n  command: .agnostic-ai/scripts/custom-memory.sh\n")
	const body = "#!/bin/sh\necho user-memory\n"
	writeFile(t, ".agnostic-ai/scripts/custom-memory.sh", body)
	syncNotes(t)
	data, err := os.ReadFile(".augment/hooks/custom-memory.sh")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != body {
		t.Errorf("custom command script = %q, want %q", data, body)
	}
	if _, err := os.Stat(".augment/hooks/memory-augment-session-start.sh"); !os.IsNotExist(err) {
		t.Errorf("custom command emitted an unused builtin script: %v", err)
	}
	syncNotes(t, "--check")
}
