package adapters

import (
	"strings"
	"testing"
)

func TestRegistry_HasAllExpectedTargets(t *testing.T) {
	expected := []string{
		"claude", "codex", "gemini", "cursor",
		"copilot", "aider", "cline", "windsurf", "continue",
		"amp", "zed", "warp", "opencode",
	}
	for _, name := range expected {
		a, ok := Get(name)
		if !ok {
			t.Errorf("missing adapter %q in registry", name)
			continue
		}
		if a.Name() != name {
			t.Errorf("adapter %q reports name %q", name, a.Name())
		}
	}
}

func TestGet_UnknownReturnsFalse(t *testing.T) {
	if _, ok := Get("nonexistent"); ok {
		t.Error("expected Get on unknown to return false")
	}
}

func TestNames_ReturnsAll(t *testing.T) {
	names := Names()
	if len(names) < 13 {
		t.Errorf("expected >= 13 names, got %d", len(names))
	}
}

func TestResolve_BuiltInWins(t *testing.T) {
	a, err := Resolve("claude")
	if err != nil {
		t.Fatalf("resolve claude: %v", err)
	}
	if a.Name() != "claude" {
		t.Errorf("Name()=%q, want claude", a.Name())
	}
}

func TestResolve_UnknownTargetMentionsPATH(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := Resolve("definitely-not-installed-target-xyz")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "unknown target") || !strings.Contains(msg, "PATH") {
		t.Errorf("err=%q, want mention of unknown target and PATH", msg)
	}
}
