package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSyncGlobal_WorksOutsideProjectAndPreservesNativeText(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AGNOSTIC_AI_HOME", filepath.Join(home, "source"))
	source := filepath.Join(home, "source", "global")
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Shared instructions\n")
	mustWriteGlobalTest(t, filepath.Join(source, "rules", "safe.md"), "---\nname: safe\n---\nBe safe.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "review", "SKILL.md"), "---\nname: review\ndescription: Review code\n---\nReview it.\n")
	mustWriteGlobalTest(t, filepath.Join(home, ".claude", "CLAUDE.md"), "Personal text.\n")

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--global"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync --global: %v", err)
	}
	for _, path := range []string{filepath.Join(home, ".claude", "CLAUDE.md"), filepath.Join(home, ".cursor", "AGENTS.md")} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		got := string(data)
		if !strings.Contains(got, "Shared instructions") || !strings.Contains(got, "Be safe") {
			t.Errorf("%s missing managed instructions:\n%s", path, got)
		}
	}
	claude, _ := os.ReadFile(filepath.Join(home, ".claude", "CLAUDE.md"))
	if !strings.Contains(string(claude), "Personal text.") {
		t.Error("sync replaced unrelated Claude instructions")
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "review", "SKILL.md")); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".cursor", "skills", "review", "SKILL.md")); err != nil {
		t.Error(err)
	}
	if runtime.GOOS != "windows" {
		bridge := filepath.Join(home, ".cursor", "hooks", "agnostic-ai-global-context.sh")
		out, err := exec.Command(bridge).Output()
		if err != nil {
			t.Fatalf("run Cursor bridge: %v", err)
		}
		var payload map[string]string
		if err := json.Unmarshal(out, &payload); err != nil || !strings.Contains(payload["additional_context"], "Shared instructions") {
			t.Fatalf("invalid Cursor bridge payload %q: %v", out, err)
		}
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"sync", "--global", "--check"})
	if err := root.Execute(); err != nil {
		t.Fatalf("second check: %v", err)
	}
}

func TestSyncGlobal_PreflightRejectsUnmanagedSkillBeforeWrites(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AGNOSTIC_AI_HOME", filepath.Join(home, "source"))
	mustWriteGlobalTest(t, filepath.Join(home, "source", "global", "AGNOSTIC_AI.md"), "new\n")
	mustWriteGlobalTest(t, filepath.Join(home, "source", "global", "skills", "review", "SKILL.md"), "---\nname: review\n---\nmanaged\n")
	mustWriteGlobalTest(t, filepath.Join(home, ".cursor", "skills", "review", "SKILL.md"), "unmanaged\n")

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--global"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unmanaged") {
		t.Fatalf("want unmanaged collision, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".claude", "CLAUDE.md")); !os.IsNotExist(statErr) {
		t.Error("preflight wrote Claude output before detecting Cursor collision")
	}
}

func TestSyncGlobal_RejectsScopedRuleAndUnsupportedFlags(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AGNOSTIC_AI_HOME", filepath.Join(home, "source"))
	mustWriteGlobalTest(t, filepath.Join(home, "source", "global", "rules", "backend", "safe.md"), "---\nname: safe\n---\nSafe.\n")
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--global"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "scoped") {
		t.Fatalf("want scoped rule error, got %v", err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"sync", "--global", "--watch"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "--watch") {
		t.Fatalf("want flag error, got %v", err)
	}
}

func TestSyncGlobal_PreservesAndRemovesOnlyManagedHooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AGNOSTIC_AI_HOME", filepath.Join(home, "source"))
	sourceHook := filepath.Join(home, "source", "global", "hooks", "notify.yaml")
	mustWriteGlobalTest(t, sourceHook, "name: notify\nevent: sessionStart\ncommand: managed-command\n")
	mustWriteGlobalTest(t, filepath.Join(home, ".cursor", "hooks.json"), "{\n  \"version\": 1,\n  \"theme\": \"dark\",\n  \"hooks\": {\"sessionStart\": [{\"command\": \"user-command\"}]}\n}\n")

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--global", "--only", "cursor"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(sourceHook); err != nil {
		t.Fatal(err)
	}
	root = NewRootCmd("test")
	root.SetArgs([]string{"sync", "--global", "--only", "cursor"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".cursor", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "user-command") || !strings.Contains(got, `"theme": "dark"`) {
		t.Fatalf("unrelated Cursor configuration was lost:\n%s", got)
	}
	if strings.Contains(got, "managed-command") {
		t.Fatalf("removed managed hook remains:\n%s", got)
	}
}

func mustWriteGlobalTest(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
