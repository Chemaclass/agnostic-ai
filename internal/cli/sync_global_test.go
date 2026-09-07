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

func TestSyncGlobal_WorksOutsideProjectAndPreservesNativeText(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AGNOSTIC_AI_HOME", filepath.Join(home, "source"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	source := filepath.Join(home, "source")
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
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	mustWriteGlobalTest(t, filepath.Join(home, "source", "AGNOSTIC_AI.md"), "new\n")
	mustWriteGlobalTest(t, filepath.Join(home, "source", "skills", "review", "SKILL.md"), "---\nname: review\n---\nmanaged\n")
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
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	mustWriteGlobalTest(t, filepath.Join(home, "source", "rules", "backend", "safe.md"), "---\nname: safe\n---\nSafe.\n")
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
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	sourceHook := filepath.Join(home, "source", "hooks", "notify.yaml")
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

func TestSyncGlobal_WritesEachTargetsOwnNativePaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AGNOSTIC_AI_HOME", filepath.Join(home, "source"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	source := filepath.Join(home, "source")
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Shared instructions\n")
	mustWriteGlobalTest(t, filepath.Join(source, "rules", "safe.md"), "---\nname: safe\n---\nBe safe.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "review", "SKILL.md"), "---\nname: review\ndescription: Review code\n---\nReview it.\n")

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--global"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync --global: %v", err)
	}

	// One representative of every shape in the table: a home-rooted
	// instructions file, an XDG-rooted one, a non-AGENTS.md filename,
	// the shared cross-tool skills tree, a target-private skills tree,
	// and the rules directory used where a vendor documents no
	// user-level instructions file.
	instructions := map[string]string{
		"codex": filepath.Join(home, ".codex", "AGENTS.md"),
		"zed":   filepath.Join(home, ".config", "zed", "AGENTS.md"),
		"goose": filepath.Join(home, ".config", "goose", ".goosehints"),
		"kiro":  filepath.Join(home, ".kiro", "steering", "AGENTS.md"),
	}
	for target, path := range instructions {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: %v", target, err)
			continue
		}
		if !strings.Contains(string(data), "Shared instructions") || !strings.Contains(string(data), "Be safe") {
			t.Errorf("%s: %s missing managed instructions:\n%s", target, path, data)
		}
	}
	for _, path := range []string{
		filepath.Join(home, ".agents", "skills", "review", "SKILL.md"),
		filepath.Join(home, ".cline", "skills", "review", "SKILL.md"),
		filepath.Join(home, ".config", "opencode", "skills", "review", "SKILL.md"),
		filepath.Join(home, ".augment", "rules", "safe.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Error(err)
		}
	}
	// Augment documents no user-level instructions file, so its rules
	// must not land as one.
	if _, err := os.Stat(filepath.Join(home, ".augment", "AGENTS.md")); !os.IsNotExist(err) {
		t.Error("wrote an instructions file for a target that documents none")
	}
	// A target with no documented user-level hooks surface gets no
	// hooks file invented for it.
	if _, err := os.Stat(filepath.Join(home, ".factory", "hooks.json")); !os.IsNotExist(err) {
		t.Error("invented a hooks file for a target with no documented user-level schema")
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"sync", "--global", "--check"})
	if err := root.Execute(); err != nil {
		t.Fatalf("check after full sync: %v", err)
	}
}

func TestSyncGlobal_SharedPathIsWrittenOnce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AGNOSTIC_AI_HOME", filepath.Join(home, "source"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	mustWriteGlobalTest(t, filepath.Join(home, "source", "AGNOSTIC_AI.md"), "Shared instructions\n")

	root := NewRootCmd("test")
	// cline and warp both read ~/.agents/AGENTS.md.
	root.SetArgs([]string{"sync", "--global", "--only", "cline,warp", "--dry-run"})
	var out bytes.Buffer
	root.SetOut(&out)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	shared := filepath.Join(home, ".agents", "AGENTS.md")
	if got := strings.Count(out.String(), "dry-run: write "+shared+"\n"); got != 1 {
		t.Errorf("shared instructions path written %d times, want 1:\n%s", got, out.String())
	}
}

func TestSyncGlobal_RejectsTargetWithNoUserLevelSurface(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AGNOSTIC_AI_HOME", filepath.Join(home, "source"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	mustWriteGlobalTest(t, filepath.Join(home, "source", "AGNOSTIC_AI.md"), "Shared instructions\n")

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--global", "-t", "jules"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unsupported target \"jules\"") {
		t.Fatalf("want unsupported-target error, got %v", err)
	}
}

func TestSyncGlobal_SweepsAnInstructionsFileAfterItsSourceIsGone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AGNOSTIC_AI_HOME", filepath.Join(home, "source"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	intro := filepath.Join(home, "source", "AGNOSTIC_AI.md")
	mustWriteGlobalTest(t, intro, "Shared instructions\n")

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--global", "--only", "codex"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "AGENTS.md")); err != nil {
		t.Fatalf("first sync wrote no instructions file: %v", err)
	}
	if err := os.Remove(intro); err != nil {
		t.Fatal(err)
	}
	root = NewRootCmd("test")
	root.SetArgs([]string{"sync", "--global", "--only", "codex"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("managed instructions file survived its source: %v", err)
	}
}

func TestSyncGlobal_KeepsTheTrailingNewlineOfPreservedUserText(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AGNOSTIC_AI_HOME", filepath.Join(home, "source"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	intro := filepath.Join(home, "source", "AGNOSTIC_AI.md")
	mustWriteGlobalTest(t, intro, "Shared instructions\n")
	mustWriteGlobalTest(t, filepath.Join(home, ".claude", "CLAUDE.md"), "Personal notes.\n")

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--global", "--only", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(intro); err != nil {
		t.Fatal(err)
	}
	root = NewRootCmd("test")
	root.SetArgs([]string{"sync", "--global", "--only", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".claude", "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "Personal notes.\n" {
		t.Errorf("preserved user text = %q, want %q", got, "Personal notes.\n")
	}
}
