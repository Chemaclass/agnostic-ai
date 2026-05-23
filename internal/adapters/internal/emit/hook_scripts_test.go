package emit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceToolFromHookCommand(t *testing.T) {
	cases := []struct {
		cmd, wantTool string
		wantOK        bool
	}{
		{".claude/hooks/x.sh", "claude", true},
		{".codex/hooks/x.sh", "codex", true},
		{".gemini/hooks/x.sh", "gemini", true},
		{"gofmt", "", false},
		{".claude/agents/y.md", "", false},
		{"./hooks/x.sh", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		tool, ok := SourceToolFromHookCommand(c.cmd)
		if tool != c.wantTool || ok != c.wantOK {
			t.Errorf("SourceToolFromHookCommand(%q) = (%q, %v), want (%q, %v)", c.cmd, tool, ok, c.wantTool, c.wantOK)
		}
	}
}

func TestMaterializeHookScript_CopiesSourceToolStashIntoTargetHooks(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.MkdirAll(".agnostic-ai/scripts/claude", 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte("#!/usr/bin/env bash\necho protect\n")
	if err := os.WriteFile(".agnostic-ai/scripts/claude/protect-files.sh", body, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := MaterializeHookScript(".codex/hooks/protect-files.sh", "codex", "claude", false); err != nil {
		t.Fatalf("materialize: %v", err)
	}

	out, err := os.ReadFile(filepath.Join(dir, ".codex/hooks/protect-files.sh"))
	if err != nil {
		t.Fatalf("expected emitted hook script under .codex/hooks/: %v", err)
	}
	if string(out) != string(body) {
		t.Errorf("body mismatch: %q vs %q", out, body)
	}
	info, err := os.Stat(filepath.Join(dir, ".codex/hooks/protect-files.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("expected executable bit preserved, got %v", info.Mode().Perm())
	}
}

func TestMaterializeHookScript_TargetVariantWinsOverSourceTool(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.MkdirAll(".agnostic-ai/scripts/codex", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(".agnostic-ai/scripts/claude", 0o755); err != nil {
		t.Fatal(err)
	}
	codexBody := []byte("codex-specific\n")
	claudeBody := []byte("claude-specific\n")
	if err := os.WriteFile(".agnostic-ai/scripts/codex/protect-files.sh", codexBody, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".agnostic-ai/scripts/claude/protect-files.sh", claudeBody, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := MaterializeHookScript(".codex/hooks/protect-files.sh", "codex", "claude", false); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(filepath.Join(dir, ".codex/hooks/protect-files.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(codexBody) {
		t.Errorf("expected target-specific variant to win, got %q", out)
	}
}

func TestMaterializeHookScript_FallsBackToUnifiedScript(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.MkdirAll(".agnostic-ai/scripts", 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte("unified\n")
	if err := os.WriteFile(".agnostic-ai/scripts/protect-files.sh", body, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := MaterializeHookScript(".codex/hooks/protect-files.sh", "codex", "claude", false); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(filepath.Join(dir, ".codex/hooks/protect-files.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(body) {
		t.Errorf("expected unified variant body, got %q", out)
	}
}

func TestMaterializeHookScript_NoStashIsNoOp(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := MaterializeHookScript(".codex/hooks/missing.sh", "codex", "claude", false); err != nil {
		t.Errorf("missing stash should be a no-op, got error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/hooks/missing.sh")); !os.IsNotExist(err) {
		t.Errorf("no body should be created when stash is empty, err=%v", err)
	}
}

func TestMaterializeHookScript_IgnoresNonHookCommand(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := MaterializeHookScript("gofmt", "codex", "", false); err != nil {
		t.Errorf("expected no-op for non-hook command, got %v", err)
	}
}
